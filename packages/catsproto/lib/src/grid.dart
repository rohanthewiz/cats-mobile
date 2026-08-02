import 'dart:typed_data';

import 'generated/wire.g.dart';

/// One pane's cell grid, as a column store over typed arrays.
///
/// # Why typed arrays and not a List of Cell
///
/// A 120×32 pane is 3840 cells. As boxed objects that is ~300 KB of allocation
/// re-done on every full frame, all of it visible to the GC; as four typed
/// arrays it is 46 KB of contiguous memory the collector never walks. The
/// renderer reads it per paint, so the difference is not academic.
///
/// The glyphs are the exception — they are Strings and cannot be unboxed — so
/// they live in a plain List, which is still one allocation rather than 3840.
///
/// # Colours resolve at APPLY time, not paint time
///
/// The wire omits a cell's fg/bg when it equals the frame's `def_fg`/`def_bg`,
/// which is the dominant case. Resolving that here means paint reads a
/// concrete colour; resolving it in the painter would repeat the branch 3840
/// times per frame instead of once per changed cell. Apply is per message;
/// paint is per frame, and there are many frames per message.
///
/// This class holds NO Flutter types. The painter, the pinch-zoom transform and
/// the row picture cache all live in the app; what lives here is the fold from
/// the wire, which is the part with tests.
class PaneGrid {
  PaneGrid(this.pane);

  final int pane;

  int _w = 0;
  int _h = 0;

  int get width => _w;
  int get height => _h;

  /// True once a full [PaneFrame] has been applied. Until then the grid has no
  /// width, so a diff's row-major index means nothing.
  bool get hasFullFrame => _hasFull;
  bool _hasFull = false;

  Uint32List _fg = Uint32List(0);
  Uint32List _bg = Uint32List(0);
  Uint16List _attr = Uint16List(0);
  Uint32List _link = Uint32List(0);
  List<String> _glyph = const [];
  Uint8List _dirtyRows = Uint8List(0);

  Uint32List get fg => _fg;
  Uint32List get bg => _bg;
  Uint16List get attr => _attr;

  /// 1-based index into [links]; 0 means no hyperlink.
  Uint32List get link => _link;

  /// One entry per cell. The tail cell of a double-width glyph is the empty
  /// string — NOT a space. It contributes only its background and draws
  /// nothing, which is what lets the preceding wide glyph spill into it.
  List<String> get glyph => _glyph;

  /// The frame's OSC 8 URI table. Link-bearing frames are always full (a β
  /// rule), so a diff never changes this.
  List<String> links = const [];

  Cursor cursor = const Cursor(x: 0, y: 0, vis: false, shape: 0);

  /// The pane's scrollback position, or null when the server sent none.
  ///
  /// `scroll: null` must CLEAR this, never preserve the last value. A stale
  /// `max` misplaces the coordinates a later `read` is computed from, which
  /// silently selects the wrong text.
  Scroll? scroll;

  /// Rows touched since [clearDirty]. The server already did the diffing, so
  /// this falls straight out of the protocol rather than being recomputed: a
  /// one-line prompt edit dirties 3 rows of 32.
  Uint8List get dirtyRows => _dirtyRows;

  /// Diffs that arrived before any full frame, or whose index fell outside the
  /// grid. Both are dropped — a diff index is relative to the last full frame's
  /// width, so applying one against the wrong width is corruption, not
  /// approximation. Counted rather than silently ignored so a test can assert
  /// it happened and a debug screen can show it.
  int droppedCells = 0;

  void applyFrame(PaneFrame frame) {
    final size = frame.w * frame.h;
    if (_w != frame.w || _h != frame.h || _fg.length != size) {
      _w = frame.w;
      _h = frame.h;
      _fg = Uint32List(size);
      _bg = Uint32List(size);
      _attr = Uint16List(size);
      _link = Uint32List(size);
      _glyph = List<String>.filled(size, '', growable: false);
      _dirtyRows = Uint8List(frame.h);
    }
    links = frame.links;
    cursor = frame.cur;
    scroll = frame.scroll;

    final cells = frame.cells;
    final n = cells.length < size ? cells.length : size;
    for (var i = 0; i < n; i++) {
      _set(i, cells[i], frame.defFg, frame.defBg);
    }
    // A short cells array is a malformed frame; blank the tail rather than
    // leaving the previous frame's content showing through, which would read as
    // a rendering bug forever after.
    for (var i = n; i < size; i++) {
      _fg[i] = frame.defFg;
      _bg[i] = frame.defBg;
      _attr[i] = 0;
      _link[i] = 0;
      _glyph[i] = '';
    }
    _defFg = frame.defFg;
    _defBg = frame.defBg;
    _hasFull = true;
    _dirtyRows.fillRange(0, _dirtyRows.length, 1);
  }

  void applyDiff(PaneDiff diff) {
    if (!_hasFull) {
      // Nothing to patch onto. registerConn guarantees a full frame per visible
      // pane on every new socket, so this is a transient at connect, not a
      // state to recover from.
      droppedCells += diff.cells.length;
      return;
    }
    if (diff.cur != null) cursor = diff.cur!;
    scroll = diff.scroll; // null CLEARS; see the field doc

    for (final cell in diff.cells) {
      final i = cell.i;
      if (i < 0 || i >= _fg.length) {
        droppedCells++;
        continue;
      }
      _set(i, cell.cell, _defFg, _defBg);
    }
  }

  int _defFg = 0;
  int _defBg = 0;

  /// The frame defaults the omitted colours resolve against. Per connection and
  /// per full frame — two clients of the same pane can be told different ones.
  int get defFg => _defFg;
  int get defBg => _defBg;

  void _set(int i, Cell cell, int defFg, int defBg) {
    // 0 is never a real packed colour (a real one is 0x02RRGGBB), so "omitted"
    // and "black" are distinguishable on the wire and resolve unambiguously.
    _fg[i] = cell.f == 0 ? defFg : cell.f;
    _bg[i] = cell.b == 0 ? defBg : cell.b;
    _attr[i] = cell.m;
    _link[i] = cell.h;
    _glyph[i] = cell.s;
    if (_w > 0) _dirtyRows[i ~/ _w] = 1;
  }

  void clearDirty() => _dirtyRows.fillRange(0, _dirtyRows.length, 0);

  /// The row a cell index lands on. Exposed because it is the one piece of
  /// index arithmetic the app repeats, and getting it wrong repaints the wrong
  /// row rather than crashing.
  int rowOf(int cellIndex) => _w == 0 ? 0 : cellIndex ~/ _w;

  /// The plain text of one row, for the "copy this line" affordance and for
  /// tests. Wide-glyph tail cells are empty strings and contribute nothing,
  /// which is what makes the result a correct string rather than one padded
  /// with phantom spaces.
  String rowText(int row) {
    if (row < 0 || row >= _h) return '';
    final sb = StringBuffer();
    for (var x = 0; x < _w; x++) {
      sb.write(_glyph[row * _w + x]);
    }
    return sb.toString();
  }
}
