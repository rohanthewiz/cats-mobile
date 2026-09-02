package catsapp

import (
	"reflect"
	"testing"

	"github.com/rohanthewiz/cats-mobile/internal/catsclient"
	"github.com/rohanthewiz/cats/wire"
	"github.com/rohanthewiz/grmob/core"
)

// The grid conversion, pinned cell by cell: run coalescing, the attribute
// bit mapping, reverse video as a swap after default resolution, inherit
// markers for default colours, wide-glyph tails, and trailing-blank trimming.

const (
	white = 0x02ffffff
	black = 0x02000000
	red   = 0x02ff0000
	blue  = 0x020000ff
)

// frameOf builds a full frame from rows of cells; every row must be the
// same width.
func frameOf(defFg, defBg uint32, rows ...[]wire.Cell) *wire.PaneFrame {
	w := len(rows[0])
	f := &wire.PaneFrame{Pane: 1, W: uint16(w), H: uint16(len(rows)), DefFg: defFg, DefBg: defBg}
	for _, r := range rows {
		f.Cells = append(f.Cells, r...)
	}
	return f
}

func cells(s string) []wire.Cell {
	out := make([]wire.Cell, 0, len(s))
	for _, r := range s {
		out = append(out, wire.Cell{S: string(r)})
	}
	return out
}

func gridFrom(f *wire.PaneFrame) *catsclient.PaneGrid {
	g := catsclient.NewPaneGrid(f.Pane)
	g.ApplyFrame(f)
	return g
}

func TestAdjacentEqualCellsCoalesceIntoOneRun(t *testing.T) {
	row := cells("ab")
	row = append(row, wire.Cell{S: "c", F: red}, wire.Cell{S: "d", F: red}, wire.Cell{S: "e"})
	rows := gridRows(gridFrom(frameOf(white, black, row)))
	want := []core.GridRow{{
		{Text: "ab"},
		{Text: "cd", Fg: "#ff0000"},
		{Text: "e"},
	}}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("rows = %#v\nwant   %#v", rows, want)
	}
}

func TestDefaultColoursBecomeInheritMarkers(t *testing.T) {
	// The frame default fg is red; a cell that says red explicitly is the
	// same as one that omits it, and both must inherit from the grid.
	row := []wire.Cell{{S: "x", F: red}, {S: "y"}, {S: "z", F: blue, B: white}}
	rows := gridRows(gridFrom(frameOf(red, black, row)))
	want := []core.GridRow{{
		{Text: "xy"},
		{Text: "z", Fg: "#0000ff", Bg: "#ffffff"},
	}}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("rows = %#v\nwant   %#v", rows, want)
	}
}

func TestEveryAttributeBitMaps(t *testing.T) {
	cases := []struct {
		m    uint16
		want int
	}{
		{attrBold, core.GridBold},
		{attrDim, core.GridDim},
		{attrItalic, core.GridItalic},
		{attrUnderline, core.GridUnderline},
		{attrStrike, core.GridStrike},
		{attrBold | attrUnderline, core.GridBold | core.GridUnderline},
		{0x10, 0}, // slow blink: no grmob spelling, dropped
	}
	for _, c := range cases {
		if got := gridAttr(c.m); got != c.want {
			t.Errorf("gridAttr(%#x) = %d, want %d", c.m, got, c.want)
		}
	}
}

func TestReverseVideoSwapsAfterDefaultsResolve(t *testing.T) {
	// A reversed cell with omitted colours must come out as fg=defBg,
	// bg=defFg, not as two inherit markers (which would make it vanish).
	row := []wire.Cell{{S: "r", M: attrReversed}, {S: "n"}}
	rows := gridRows(gridFrom(frameOf(white, black, row)))
	want := []core.GridRow{{
		{Text: "r", Fg: "#000000", Bg: "#ffffff"},
		{Text: "n"},
	}}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("rows = %#v\nwant   %#v", rows, want)
	}
}

func TestWideGlyphTailContributesNoText(t *testing.T) {
	row := []wire.Cell{{S: "漢"}, {S: ""}, {S: "a"}}
	rows := gridRows(gridFrom(frameOf(white, black, row)))
	if len(rows) != 1 || len(rows[0]) != 1 || rows[0][0].Text != "漢a" {
		t.Errorf("rows = %#v", rows)
	}
}

func TestTrailingBlankRunsAreTrimmedButStyledOnesKept(t *testing.T) {
	plain := cells("hi      ")
	rows := gridRows(gridFrom(frameOf(white, black, plain)))
	if want := (core.GridRow{{Text: "hi"}}); !reflect.DeepEqual(rows[0], want) {
		t.Errorf("plain padding not trimmed: %#v", rows[0])
	}

	// A trailing run with a background is content (a status bar), and one
	// with underline draws on its spaces.
	styled := cells("hi")
	styled = append(styled, wire.Cell{S: " ", B: blue}, wire.Cell{S: " ", B: blue})
	rows = gridRows(gridFrom(frameOf(white, black, styled)))
	if len(rows[0]) != 2 || rows[0][1].Bg != "#0000ff" || rows[0][1].Text != "  " {
		t.Errorf("styled padding was trimmed: %#v", rows[0])
	}

	// An all-blank row becomes an empty row, which TextGrid keeps as a
	// blank line.
	rows = gridRows(gridFrom(frameOf(white, black, cells("    "))))
	if len(rows) != 1 || len(rows[0]) != 0 {
		t.Errorf("blank row = %#v, want an empty row", rows)
	}
}

func TestNoFrameYieldsNoRows(t *testing.T) {
	if rows := gridRows(catsclient.NewPaneGrid(1)); rows != nil {
		t.Errorf("rows before a frame = %#v", rows)
	}
	if rows := gridRows(nil); rows != nil {
		t.Errorf("rows of nil = %#v", rows)
	}
}

func TestCSSColorDropsTheEncodingTag(t *testing.T) {
	if got := cssColor(0x02a1b2c3); got != "#a1b2c3" {
		t.Errorf("cssColor = %q", got)
	}
}
