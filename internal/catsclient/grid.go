package catsclient

import (
	"strings"

	"github.com/rohanthewiz/cats/wire"
)

// PaneGrid is one pane's cell grid, as a column store over flat slices.
//
// # Why parallel slices and not a slice of Cell
//
// A 120×32 pane is 3840 cells. As one struct per cell that is a copy of every
// field on every full frame and a wide stride for a renderer that reads one
// attribute at a time; as five flat slices it is contiguous, cache-friendly
// memory that a row renderer walks in order. The glyphs are strings and cannot
// be unboxed, so they cost a header each, but still one allocation for the
// slice rather than 3840 objects.
//
// # Colours resolve at APPLY time, not paint time
//
// The wire omits a cell's fg/bg when it equals the frame's def_fg/def_bg, which
// is the dominant case. Resolving that here means paint reads a concrete
// colour; resolving it in the renderer would repeat the branch 3840 times per
// frame instead of once per changed cell. Apply is per message; paint is per
// frame, and there are many frames per message.
//
// This type holds no UI types. The TextGrid rows, the zoom transform and the
// per-row cache all live in the app; what lives here is the fold from the
// wire, which is the part with tests. Not safe for concurrent use: the Session
// that owns it is locked by the app around Apply and around every render.
type PaneGrid struct {
	Pane uint32

	w, h    int
	hasFull bool

	fg    []uint32
	bg    []uint32
	attr  []uint16
	link  []uint32
	glyph []string
	dirty []bool

	defFg, defBg uint32

	// Links is the frame's OSC 8 URI table. Link-bearing frames are always
	// full (a β rule), so a diff never changes this.
	Links []string

	Cursor wire.Cursor

	// Scroll is the pane's scrollback position, or nil when the server sent
	// none.
	//
	// A nil scroll on a diff must CLEAR this, never preserve the last value.
	// A stale Max misplaces the coordinates a later read is computed from,
	// which silently selects the wrong text.
	Scroll *wire.Scroll

	// DroppedCells counts diff cells that arrived before any full frame, or
	// whose index fell outside the grid. Both are dropped: a diff index is
	// relative to the last full frame's width, so applying one against the
	// wrong width is corruption, not approximation. Counted rather than
	// silently ignored so a test can assert it happened and a debug screen can
	// show it.
	DroppedCells int
}

func NewPaneGrid(pane uint32) *PaneGrid { return &PaneGrid{Pane: pane} }

func (g *PaneGrid) Width() int  { return g.w }
func (g *PaneGrid) Height() int { return g.h }

// HasFullFrame is true once a full PaneFrame has been applied. Until then the
// grid has no width, so a diff's row-major index means nothing.
func (g *PaneGrid) HasFullFrame() bool { return g.hasFull }

// The column accessors return the live backing slices, not copies: the
// renderer reads them per paint and a copy per paint would undo the reason the
// store is flat. Callers must not write to them.

// Fg and Bg are packed 0x02RRGGBB colours, already resolved against the
// frame defaults.
func (g *PaneGrid) Fg() []uint32 { return g.fg }
func (g *PaneGrid) Bg() []uint32 { return g.bg }

// Attr is the ratatui modifier bitmask per cell.
func (g *PaneGrid) Attr() []uint16 { return g.attr }

// Link is a 1-based index into Links per cell; 0 means no hyperlink.
func (g *PaneGrid) Link() []uint32 { return g.link }

// Glyph is one entry per cell. The tail cell of a double-width glyph is the
// empty string, NOT a space. It contributes only its background and draws
// nothing, which is what lets the preceding wide glyph spill into it.
func (g *PaneGrid) Glyph() []string { return g.glyph }

// DirtyRows marks rows touched since ClearDirty. The server already did the
// diffing, so this falls straight out of the protocol rather than being
// recomputed: a one-line prompt edit dirties 3 rows of 32.
func (g *PaneGrid) DirtyRows() []bool { return g.dirty }

// DefFg and DefBg are the frame defaults the omitted colours resolve against.
// Per connection and per full frame: two clients of the same pane can be told
// different ones.
func (g *PaneGrid) DefFg() uint32 { return g.defFg }
func (g *PaneGrid) DefBg() uint32 { return g.defBg }

// ApplyFrame replaces the whole grid.
func (g *PaneGrid) ApplyFrame(frame *wire.PaneFrame) {
	w, h := int(frame.W), int(frame.H)
	size := w * h
	if g.w != w || g.h != h || len(g.fg) != size {
		g.w, g.h = w, h
		g.fg = make([]uint32, size)
		g.bg = make([]uint32, size)
		g.attr = make([]uint16, size)
		g.link = make([]uint32, size)
		g.glyph = make([]string, size)
		g.dirty = make([]bool, h)
	}
	g.Links = frame.Links
	g.Cursor = frame.Cur
	g.Scroll = frame.Scroll

	n := min(len(frame.Cells), size)
	for i := range n {
		g.set(i, frame.Cells[i], frame.DefFg, frame.DefBg)
	}
	// A short cells array is a malformed frame; blank the tail rather than
	// leaving the previous frame's content showing through, which would read
	// as a rendering bug forever after.
	for i := n; i < size; i++ {
		g.fg[i] = frame.DefFg
		g.bg[i] = frame.DefBg
		g.attr[i] = 0
		g.link[i] = 0
		g.glyph[i] = ""
	}
	g.defFg, g.defBg = frame.DefFg, frame.DefBg
	g.hasFull = true
	for i := range g.dirty {
		g.dirty[i] = true
	}
}

// ApplyDiff patches changed cells onto the last full frame.
func (g *PaneGrid) ApplyDiff(diff *wire.PaneDiff) {
	if !g.hasFull {
		// Nothing to patch onto. registerConn guarantees a full frame per
		// visible pane on every new socket, so this is a transient at
		// connect, not a state to recover from.
		g.DroppedCells += len(diff.Cells)
		return
	}
	if diff.Cur != nil {
		g.Cursor = *diff.Cur
	}
	g.Scroll = diff.Scroll // nil CLEARS; see the field doc

	for _, cell := range diff.Cells {
		if cell.I < 0 || cell.I >= len(g.fg) {
			g.DroppedCells++
			continue
		}
		g.set(cell.I, cell.Cell, g.defFg, g.defBg)
	}
}

func (g *PaneGrid) set(i int, cell wire.Cell, defFg, defBg uint32) {
	// 0 is never a real packed colour (a real one is 0x02RRGGBB), so "omitted"
	// and "black" are distinguishable on the wire and resolve unambiguously.
	g.fg[i] = cell.F
	if cell.F == 0 {
		g.fg[i] = defFg
	}
	g.bg[i] = cell.B
	if cell.B == 0 {
		g.bg[i] = defBg
	}
	g.attr[i] = cell.M
	g.link[i] = cell.H
	g.glyph[i] = cell.S
	if g.w > 0 {
		g.dirty[i/g.w] = true
	}
}

// ClearDirty is called by the renderer after it has painted the dirty rows.
func (g *PaneGrid) ClearDirty() {
	for i := range g.dirty {
		g.dirty[i] = false
	}
}

// RowOf is the row a cell index lands on. Exposed because it is the one piece
// of index arithmetic the app repeats, and getting it wrong repaints the wrong
// row rather than crashing.
func (g *PaneGrid) RowOf(cellIndex int) int {
	if g.w == 0 {
		return 0
	}
	return cellIndex / g.w
}

// RowText is the plain text of one row, for the "copy this line" affordance
// and for tests. Wide-glyph tail cells are empty strings and contribute
// nothing, which is what makes the result a correct string rather than one
// padded with phantom spaces.
func (g *PaneGrid) RowText(row int) string {
	if row < 0 || row >= g.h {
		return ""
	}
	var sb strings.Builder
	for x := 0; x < g.w; x++ {
		sb.WriteString(g.glyph[row*g.w+x])
	}
	return sb.String()
}
