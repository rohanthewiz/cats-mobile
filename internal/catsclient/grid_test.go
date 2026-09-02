package catsclient

import (
	"testing"

	"github.com/rohanthewiz/cats/wire"
)

// Mirrors packages/catsproto/test/grid_test.dart.

func testFrame(w, h int, defFg, defBg uint32) *wire.PaneFrame {
	cells := make([]wire.Cell, w*h)
	for i := range cells {
		cells[i] = wire.Cell{S: string(rune('a' + i%26))}
	}
	return &wire.PaneFrame{
		T: wire.MsgPaneFrame, Pane: 1, W: uint16(w), H: uint16(h),
		Cur:   wire.Cursor{Vis: true},
		DefFg: defFg, DefBg: defBg,
		Cells:  cells,
		Scroll: &wire.Scroll{Off: 0, Max: 100, Rows: 3},
	}
}

func defaultFrame() *wire.PaneFrame { return testFrame(4, 3, 0x02c0c0c0, 0x02000000) }

func all[T comparable](xs []T, want T) bool {
	for _, x := range xs {
		if x != want {
			return false
		}
	}
	return true
}

func TestApplyFrameResolvesOmittedColoursAgainstTheFrameDefaults(t *testing.T) {
	g := NewPaneGrid(1)
	g.ApplyFrame(defaultFrame())
	// Every cell in the fixture omits both colours, which is the dominant
	// case on the wire; resolving here means the renderer reads a concrete
	// value instead of branching 3840 times a frame.
	if !all(g.Fg(), 0x02c0c0c0) {
		t.Errorf("fg = %v", g.Fg())
	}
	if !all(g.Bg(), 0x02000000) {
		t.Errorf("bg = %v", g.Bg())
	}
}

func TestApplyFrameKeepsAnExplicitColour(t *testing.T) {
	f := defaultFrame()
	f.Cells[0] = wire.Cell{S: "x", F: 0x02ff0000}
	g := NewPaneGrid(1)
	g.ApplyFrame(f)
	if g.Fg()[0] != 0x02ff0000 {
		t.Errorf("fg[0] = %#x", g.Fg()[0])
	}
	if g.Fg()[1] != f.DefFg {
		t.Errorf("fg[1] = %#x, want the default", g.Fg()[1])
	}
}

func TestApplyFrameMarksEveryRowDirty(t *testing.T) {
	g := NewPaneGrid(1)
	g.ApplyFrame(defaultFrame())
	if !all(g.DirtyRows(), true) {
		t.Errorf("dirty = %v", g.DirtyRows())
	}
	g.ClearDirty()
	if !all(g.DirtyRows(), false) {
		t.Errorf("dirty after clear = %v", g.DirtyRows())
	}
}

func TestApplyFrameAShortCellsArrayBlanksTheTail(t *testing.T) {
	// Instead of showing stale content.
	g := NewPaneGrid(1)
	g.ApplyFrame(defaultFrame())
	g.ApplyFrame(&wire.PaneFrame{
		Pane: 1, W: 4, H: 3, DefFg: 1, DefBg: 2,
		Cells: []wire.Cell{{S: "z"}},
	})
	if g.Glyph()[0] != "z" {
		t.Errorf("glyph[0] = %q", g.Glyph()[0])
	}
	if !all(g.Glyph()[1:], "") {
		t.Errorf("tail = %q", g.Glyph()[1:])
	}
	if g.Fg()[5] != 1 || g.Bg()[5] != 2 {
		t.Error("the blanked tail should carry the new defaults")
	}
}

func TestApplyFrameAnOverlongCellsArrayIsClamped(t *testing.T) {
	f := testFrame(2, 1, 1, 2)
	f.Cells = append(f.Cells, wire.Cell{S: "extra"}, wire.Cell{S: "extra"})
	g := NewPaneGrid(1)
	g.ApplyFrame(f)
	if len(g.Glyph()) != 2 {
		t.Errorf("grid has %d cells, want w*h = 2", len(g.Glyph()))
	}
}

func TestApplyDiffACellDirtiesItsRowAndNoOther(t *testing.T) {
	g := NewPaneGrid(1)
	g.ApplyFrame(testFrame(4, 3, 1, 2))
	g.ClearDirty()
	g.ApplyDiff(&wire.PaneDiff{Pane: 1, Cells: []wire.DiffCell{{I: 6, Cell: wire.Cell{S: "Z"}}}})
	if g.RowOf(6) != 1 {
		t.Errorf("rowOf(6) = %d", g.RowOf(6))
	}
	if d := g.DirtyRows(); d[0] || !d[1] || d[2] {
		t.Errorf("dirty = %v, want only row 1", d)
	}
	if g.Glyph()[6] != "Z" {
		t.Errorf("glyph[6] = %q", g.Glyph()[6])
	}
}

func TestApplyDiffBeforeAnyFullFrameIsDroppedAndCounted(t *testing.T) {
	// Counted rather than silently ignored: registerConn guarantees a full
	// frame per visible pane on connect, so a non-zero count means something
	// is wrong with the resync, not with the wire.
	g := NewPaneGrid(1)
	g.ApplyDiff(&wire.PaneDiff{Pane: 1, Cells: []wire.DiffCell{
		{I: 0, Cell: wire.Cell{S: "a"}}, {I: 1, Cell: wire.Cell{S: "b"}},
	}})
	if g.HasFullFrame() {
		t.Error("a diff is not a full frame")
	}
	if g.DroppedCells != 2 {
		t.Errorf("dropped = %d", g.DroppedCells)
	}
}

func TestApplyDiffOutOfRangeIndexIsDroppedAndCounted(t *testing.T) {
	g := NewPaneGrid(1)
	g.ApplyFrame(testFrame(4, 3, 1, 2))
	g.ApplyDiff(&wire.PaneDiff{Pane: 1, Cells: []wire.DiffCell{
		{I: 999, Cell: wire.Cell{S: "x"}}, {I: -1, Cell: wire.Cell{S: "y"}},
	}})
	if g.DroppedCells != 2 {
		t.Errorf("dropped = %d", g.DroppedCells)
	}
}

func TestApplyDiffANilScrollClearsRatherThanPreserving(t *testing.T) {
	// A stale max misplaces the absolute coordinates a later read is computed
	// from, which selects the wrong text without any error.
	g := NewPaneGrid(1)
	g.ApplyFrame(defaultFrame())
	if g.Scroll == nil || g.Scroll.Max != 100 {
		t.Fatalf("scroll = %+v", g.Scroll)
	}
	g.ApplyDiff(&wire.PaneDiff{Pane: 1})
	if g.Scroll != nil {
		t.Errorf("scroll = %+v, want nil", g.Scroll)
	}
}

func TestApplyDiffOmittedColoursResolveAgainstTheLastFullFrame(t *testing.T) {
	g := NewPaneGrid(1)
	g.ApplyFrame(testFrame(4, 3, 0x02111111, 0x02222222))
	g.ApplyDiff(&wire.PaneDiff{Pane: 1, Cells: []wire.DiffCell{{I: 2, Cell: wire.Cell{S: "q"}}}})
	if g.Fg()[2] != 0x02111111 || g.Bg()[2] != 0x02222222 {
		t.Errorf("fg,bg = %#x,%#x", g.Fg()[2], g.Bg()[2])
	}
}

func TestApplyDiffACursorIsOnlyReplacedWhenTheDiffCarriesOne(t *testing.T) {
	g := NewPaneGrid(1)
	g.ApplyFrame(defaultFrame())
	g.ApplyDiff(&wire.PaneDiff{Pane: 1})
	if !g.Cursor.Vis {
		t.Error("an absent cur means unchanged")
	}
	g.ApplyDiff(&wire.PaneDiff{Pane: 1, Cur: &wire.Cursor{X: 3, Y: 2, Vis: false, Shape: 1}})
	if g.Cursor.X != 3 || g.Cursor.Vis {
		t.Errorf("cursor = %+v", g.Cursor)
	}
}

func TestWideGlyphTailCellIsEmptyNotASpace(t *testing.T) {
	// Verified against the server with:
	//   catctl probe --script 'type:printf "AB你好CD|\n"; dump:1'
	// The wire grid is always exactly w*h; a double-width glyph occupies its
	// own cell and leaves the next one as "", which is what lets the glyph
	// spill into it at paint time instead of being clipped.
	g := NewPaneGrid(1)
	g.ApplyFrame(&wire.PaneFrame{
		Pane: 1, W: 4, H: 1, DefFg: 1, DefBg: 2,
		Cells: []wire.Cell{{S: "A"}, {S: "你"}, {S: ""}, {S: "B"}},
	})
	if g.Glyph()[2] != "" {
		t.Errorf("tail = %q, want empty", g.Glyph()[2])
	}
	if got := g.RowText(0); got != "A你B" {
		t.Errorf("rowText = %q", got)
	}
	if g.RowText(5) != "" || g.RowText(-1) != "" {
		t.Error("an out-of-range row is empty, not a panic")
	}
}
