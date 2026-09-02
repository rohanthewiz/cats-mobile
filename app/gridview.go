package catsapp

import (
	"fmt"
	"strings"

	"github.com/rohanthewiz/cats-mobile/internal/catsclient"
	"github.com/rohanthewiz/grmob/core"
)

// The bridge from a PaneGrid's column store to TextGrid rows.
//
// # Cell attribute bits
//
// These are the ratatui Modifier bits cats packs into Cell.M. cats keeps them
// unexported (internal/orchestration/protocol.go), which is why the Dart
// generator scraped them from source, and why they are retyped here with the
// same warning the Dart copy carried: a misplaced bit is not a crash, it is
// italics where there should be underline, forever. gridview_test.go pins
// every one of them against a hand-built frame.
const (
	attrBold      uint16 = 0x1
	attrDim       uint16 = 0x2
	attrItalic    uint16 = 0x4
	attrUnderline uint16 = 0x8
	attrReversed  uint16 = 0x40
	attrStrike    uint16 = 0x100
)

// gridRows converts a pane grid into one GridRow per terminal row.
//
// # Runs
//
// Adjacent cells with the same fg, bg and attributes coalesce into one run,
// which is what makes a row a handful of runs rather than 120 of them. A
// prompt line is typically three to six runs; a full-colour TUI row more.
// The plan's measurement (one changed row is a ~260-byte patch) assumed
// six-run rows, and this is where that assumption is made true.
//
// # Reverse video
//
// grmob has no reverse attribute, so a reversed cell swaps its fg and bg
// here, AFTER the frame defaults have been resolved (PaneGrid did that at
// apply time). Swapping before resolution would swap two "inherit" markers
// and the cell would vanish.
//
// # Inheritance
//
// A run whose colour equals the frame default is emitted with "" so it
// inherits the grid's TextColor/Background. That is the common case, and it
// keeps the wire form of an ordinary row to its text.
//
// # Wide glyphs
//
// The tail cell of a double-width glyph is the empty string; it contributes
// only its background (it keeps its run's colours) and no text, which is
// what lets the preceding glyph spill into it on a monospace renderer.
func gridRows(g *catsclient.PaneGrid) []core.GridRow {
	if g == nil || !g.HasFullFrame() {
		return nil
	}
	width, height := g.Width(), g.Height()
	fg, bg, attr, glyph := g.Fg(), g.Bg(), g.Attr(), g.Glyph()
	defFg, defBg := g.DefFg(), g.DefBg()

	rows := make([]core.GridRow, height)
	var text strings.Builder
	for y := 0; y < height; y++ {
		var row core.GridRow
		var cur core.GridRun
		open := false
		flush := func() {
			if open {
				cur.Text = text.String()
				row = append(row, cur)
				text.Reset()
			}
			open = false
		}
		for x := 0; x < width; x++ {
			i := y*width + x
			cellFg, cellBg, cellAttr := fg[i], bg[i], attr[i]
			if cellAttr&attrReversed != 0 {
				cellFg, cellBg = cellBg, cellFg
			}
			run := core.GridRun{
				Fg:   colorOrInherit(cellFg, defFg),
				Bg:   colorOrInherit(cellBg, defBg),
				Attr: gridAttr(cellAttr),
			}
			if !open || run.Fg != cur.Fg || run.Bg != cur.Bg || run.Attr != cur.Attr {
				flush()
				cur = run
				open = true
			}
			text.WriteString(glyph[i])
		}
		flush()
		rows[y] = trimTrailingBlank(row)
	}
	return rows
}

// trimTrailingBlank drops runs at the end of a row that are only spaces in
// the inherited colours with no attribute that draws on a space (underline,
// strike). A terminal row is padded to full width, and 60 trailing spaces
// on 24 rows is most of an idle pane's wire form. The renderer does not
// care: rows are laid out left to right and an absent tail is a blank tail.
func trimTrailingBlank(row core.GridRow) core.GridRow {
	for len(row) > 0 {
		last := row[len(row)-1]
		if last.Bg != "" || last.Attr&(core.GridUnderline|core.GridStrike) != 0 {
			return row
		}
		if strings.TrimSpace(last.Text) != "" {
			// Trim the run's own trailing spaces but keep the run.
			row[len(row)-1].Text = strings.TrimRight(last.Text, " ")
			return row
		}
		row = row[:len(row)-1]
	}
	return row
}

// colorOrInherit is the CSS form of a packed colour, or "" when it is the
// frame default (the grid's own colour applies).
func colorOrInherit(packed, def uint32) string {
	if packed == def {
		return ""
	}
	return cssColor(packed)
}

// cssColor renders a packed 0x02RRGGBB colour as "#rrggbb". The top byte is
// the encoding tag (0x02 for RGB) and is dropped.
func cssColor(packed uint32) string {
	return fmt.Sprintf("#%06x", packed&0xFFFFFF)
}

// gridAttr maps ratatui modifier bits onto grmob's Grid* bits. Reverse is
// handled by the caller (a swap, not an attribute); anything else ratatui
// can express (blink, hidden) has no grmob spelling and is dropped.
func gridAttr(m uint16) int {
	a := 0
	if m&attrBold != 0 {
		a |= core.GridBold
	}
	if m&attrDim != 0 {
		a |= core.GridDim
	}
	if m&attrItalic != 0 {
		a |= core.GridItalic
	}
	if m&attrUnderline != 0 {
		a |= core.GridUnderline
	}
	if m&attrStrike != 0 {
		a |= core.GridStrike
	}
	return a
}
