import 'package:catsproto/catsproto.dart';
import 'package:test/test.dart';

PaneFrame frame({
  int w = 4,
  int h = 3,
  int defFg = 0x02c0c0c0,
  int defBg = 0x02000000,
}) => PaneFrame(
  pane: 1,
  w: w,
  h: h,
  cur: const Cursor(x: 0, y: 0, vis: true, shape: 0),
  defFg: defFg,
  defBg: defBg,
  cells: List<Cell>.generate(
    w * h,
    (i) => Cell(s: String.fromCharCode(97 + i % 26)),
  ),
  scroll: const Scroll(off: 0, max: 100, rows: 3),
);

void main() {
  group('PaneGrid.applyFrame', () {
    test('resolves omitted colours against the frame defaults', () {
      final grid = PaneGrid(1)..applyFrame(frame());
      // Every cell in the fixture omits both colours, which is the dominant
      // case on the wire — resolving here means the painter reads a concrete
      // value instead of branching 3840 times a frame.
      expect(grid.fg.every((c) => c == 0x02c0c0c0), isTrue);
      expect(grid.bg.every((c) => c == 0x02000000), isTrue);
    });

    test('keeps an explicit colour', () {
      final f = frame();
      final grid = PaneGrid(1)
        ..applyFrame(
          PaneFrame(
            pane: 1,
            w: f.w,
            h: f.h,
            cur: f.cur,
            defFg: f.defFg,
            defBg: f.defBg,
            cells: [
              const Cell(s: 'x', f: 0x02ff0000),
              ...f.cells.skip(1),
            ],
          ),
        );
      expect(grid.fg[0], 0x02ff0000);
      expect(grid.fg[1], f.defFg);
    });

    test('marks every row dirty', () {
      final grid = PaneGrid(1)..applyFrame(frame());
      expect(grid.dirtyRows.every((r) => r == 1), isTrue);
      grid.clearDirty();
      expect(grid.dirtyRows.every((r) => r == 0), isTrue);
    });

    test(
      'a short cells array blanks the tail instead of showing stale content',
      () {
        final grid = PaneGrid(1)..applyFrame(frame());
        grid.applyFrame(
          PaneFrame(
            pane: 1,
            w: 4,
            h: 3,
            cur: const Cursor(x: 0, y: 0, vis: false, shape: 0),
            defFg: 1,
            defBg: 2,
            cells: const [Cell(s: 'z')],
          ),
        );
        expect(grid.glyph[0], 'z');
        expect(grid.glyph.skip(1).every((g) => g.isEmpty), isTrue);
      },
    );
  });

  group('PaneGrid.applyDiff', () {
    test('a cell at index i dirties row i ~/ w and no other', () {
      final grid = PaneGrid(1)..applyFrame(frame(w: 4, h: 3));
      grid.clearDirty();
      grid.applyDiff(const PaneDiff(pane: 1, cells: [DiffCell(i: 6, s: 'Z')]));
      expect(grid.rowOf(6), 1);
      expect(grid.dirtyRows[0], 0);
      expect(grid.dirtyRows[1], 1);
      expect(grid.dirtyRows[2], 0);
      expect(grid.glyph[6], 'Z');
    });

    test('a diff before any full frame is dropped AND counted', () {
      // Counted rather than silently ignored: registerConn guarantees a full
      // frame per visible pane on connect, so a non-zero count means something
      // is wrong with the resync, not with the wire.
      final grid = PaneGrid(1);
      grid.applyDiff(
        const PaneDiff(
          pane: 1,
          cells: [
            DiffCell(i: 0, s: 'a'),
            DiffCell(i: 1, s: 'b'),
          ],
        ),
      );
      expect(grid.hasFullFrame, isFalse);
      expect(grid.droppedCells, 2);
    });

    test('an out-of-range index is dropped and counted', () {
      final grid = PaneGrid(1)..applyFrame(frame(w: 4, h: 3));
      grid.applyDiff(
        const PaneDiff(
          pane: 1,
          cells: [
            DiffCell(i: 999, s: 'x'),
            DiffCell(i: -1, s: 'y'),
          ],
        ),
      );
      expect(grid.droppedCells, 2);
    });

    test('a null scroll CLEARS rather than preserving the last one', () {
      // A stale `max` misplaces the absolute coordinates a later `read` is
      // computed from, which selects the wrong text without any error.
      final grid = PaneGrid(1)..applyFrame(frame());
      expect(grid.scroll?.max, 100);
      grid.applyDiff(const PaneDiff(pane: 1, cells: []));
      expect(grid.scroll, isNull);
    });

    test(
      'omitted diff colours resolve against the LAST FULL frame defaults',
      () {
        final grid = PaneGrid(1)
          ..applyFrame(frame(defFg: 0x02111111, defBg: 0x02222222));
        grid.applyDiff(
          const PaneDiff(pane: 1, cells: [DiffCell(i: 2, s: 'q')]),
        );
        expect(grid.fg[2], 0x02111111);
        expect(grid.bg[2], 0x02222222);
      },
    );

    test('a cursor is only replaced when the diff carries one', () {
      final grid = PaneGrid(1)..applyFrame(frame());
      grid.applyDiff(const PaneDiff(pane: 1, cells: []));
      expect(grid.cursor.vis, isTrue, reason: 'an absent cur means unchanged');
      grid.applyDiff(
        const PaneDiff(
          pane: 1,
          cur: Cursor(x: 3, y: 2, vis: false, shape: 1),
          cells: [],
        ),
      );
      expect(grid.cursor.x, 3);
      expect(grid.cursor.vis, isFalse);
    });
  });

  group('wide glyphs', () {
    test('the tail cell is empty, not a space, and contributes no text', () {
      // Verified against the server with:
      //   catctl probe --script 'type:printf "AB你好CD|\n"; dump:1'
      // The wire grid is always exactly w*h; a double-width glyph occupies its
      // own cell and leaves the next one as "" — which is what lets the glyph
      // spill into it at paint time instead of being clipped.
      final grid = PaneGrid(1)
        ..applyFrame(
          PaneFrame(
            pane: 1,
            w: 4,
            h: 1,
            cur: const Cursor(x: 0, y: 0, vis: false, shape: 0),
            defFg: 1,
            defBg: 2,
            cells: const [
              Cell(s: 'A'),
              Cell(s: '你'),
              Cell(s: ''),
              Cell(s: 'B'),
            ],
          ),
        );
      expect(grid.glyph[2], '');
      expect(grid.glyph[2], isNot(' '));
      expect(grid.rowText(0), 'A你B');
    });
  });
}
