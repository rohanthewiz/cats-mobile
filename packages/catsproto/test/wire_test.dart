import 'dart:convert';

import 'package:catsproto/catsproto.dart';
import 'package:test/test.dart';

/// Round-tripping through jsonEncode rather than == is deliberate: the
/// generated classes carry no value equality (deep equality over their List and
/// Map fields would need package:collection, and this package takes no runtime
/// dependencies), and the encoded form is the thing that actually has to match
/// the server anyway.
String rt(
  Object? Function(Map<String, Object?>) from,
  Map<String, Object?> json,
) {
  final decoded = from(json);
  return jsonEncode((decoded as dynamic).toJson());
}

void main() {
  group('decodeDown', () {
    test('routes every down type it knows', () {
      final cases = <String, Type>{
        MsgType.welcome: Welcome,
        MsgType.layout: Layout,
        MsgType.agents: Agents,
        MsgType.paneTitle: PaneTitle,
        MsgType.paneCwd: PaneCwd,
        MsgType.paneAgent: PaneAgent,
        MsgType.paneModes: PaneModes,
        MsgType.paneExited: PaneExited,
        MsgType.paneFrame: PaneFrame,
        MsgType.paneDiff: PaneDiff,
        MsgType.clipboard: Clipboard,
        MsgType.notify: Notify,
        MsgType.title: Title,
        MsgType.error: ErrorMsg,
        MsgType.shutdown: Shutdown,
        MsgType.updateReady: UpdateReady,
        MsgType.theme: Theme,
        MsgType.usage: Usage,
        MsgType.clients: Clients,
        MsgType.cmdResult: CmdResult,
      };
      for (final entry in cases.entries) {
        final decoded = decodeDown({'t': entry.key});
        expect(decoded.runtimeType, entry.value, reason: 'for t=${entry.key}');
      }
    });

    test('drops an unknown type rather than throwing', () {
      // The rule that lets catway add a message type without a version bump.
      // A client that threw here would drop the socket and every pane with it.
      expect(decodeDown({'t': 'a_type_from_the_future'}), isNull);
      expect(decodeDown(const {}), isNull);
    });

    test('tolerates a message missing every field', () {
      final frame = decodeDown({'t': MsgType.paneFrame})! as PaneFrame;
      expect(frame.w, 0);
      expect(frame.cells, isEmpty);
      expect(frame.scroll, isNull);
    });

    test('tolerates fields of the wrong type', () {
      // Not hypothetical robustness theatre: a proxy that rewrites JSON, or a
      // future server sending a richer shape under an old key, both land here.
      final title = decodeDown({'t': MsgType.title, 'title': 42})! as Title;
      expect(title.title, '');
    });
  });

  group('decodeFrame', () {
    test('returns null for malformed JSON instead of throwing', () {
      expect(decodeFrame('{not json'), isNull);
      expect(decodeFrame('[1,2,3]'), isNull); // valid JSON, wrong shape
      expect(decodeFrame('{"t":"title"}'), isNotNull);
    });
  });

  group('toJson', () {
    test('omits omitempty fields at their zero, never sends null', () {
      const key = Key(code: 'KeyA', key: 'a', mods: 0, kind: KeyKind.down);
      final json = key.toJson();
      expect(
        json.containsKey('pane'),
        isFalse,
        reason: 'pane 0 means "the focused pane" and must be ABSENT, not 0',
      );
      expect(json['t'], MsgType.key);
      expect(json.values, isNot(contains(null)));
    });

    test('emits a non-omitempty field even at its zero', () {
      // mods has no omitempty in Go, so the server always sees the key.
      const key = Key(code: 'KeyA', key: 'a', mods: 0, kind: KeyKind.down);
      expect(key.toJson().containsKey('mods'), isTrue);
    });

    test('always carries the discriminator', () {
      expect(const Paste(data: 'hi').toJson()['t'], MsgType.paste);
      expect(
        const Notify(kind: 'attention', message: 'm').toJson()['t'],
        MsgType.notify,
      );
    });
  });

  group('round trips', () {
    test('PaneFrame with cells, links and scroll', () {
      final json = {
        't': MsgType.paneFrame,
        'pane': 3,
        'w': 2,
        'h': 2,
        'cur': {'x': 1, 'y': 0, 'vis': true, 'shape': 2},
        'def_fg': 0x02c0c0c0,
        'def_bg': 0x02000000,
        'links': ['https://example.com'],
        'cells': [
          {'s': 'a'},
          {'s': 'b', 'f': 0x02ff0000, 'm': 1, 'h': 1},
          {'s': ''},
          {'s': 'd', 'b': 0x0200ff00},
        ],
        'scroll': {'off': 0, 'max': 120, 'rows': 32},
      };
      expect(rt(PaneFrame.fromJson, json), jsonEncode(json));
    });

    test('DiffCell flattens the embedded Cell onto the wire', () {
      final diff = PaneDiff.fromJson({
        't': MsgType.paneDiff,
        'pane': 1,
        'cells': [
          {'i': 7, 's': 'x', 'f': 0x02ffffff},
        ],
      });
      final cell = diff.cells.single;
      expect(cell.i, 7);
      expect(cell.s, 'x');
      // …and regroups it, so a caller can pass the styling around as a unit.
      expect(cell.cell.s, 'x');
      expect(cell.cell.f, 0x02ffffff);
      expect(
        cell.toJson().containsKey('cell'),
        isFalse,
        reason: 'the wire is flat; a nested object would not decode in Go',
      );
    });

    test('Rect survives as an array', () {
      final layout = Layout.fromJson({
        't': MsgType.layout,
        'workspaces': <Object?>[],
        'tabs': <Object?>[],
        'panes': [
          {
            'pane': 1,
            'pub': 'w1:p1',
            'rect': [0, 0, 80, 24],
            'inner': [1, 1, 78, 22],
            'focused': true,
          },
        ],
        'borders': <Object?>[],
      });
      final pane = layout.panes.single;
      expect(pane.rect, const Rect(0, 0, 80, 24));
      expect(pane.rect.w, 80);
      expect(pane.scrollbar, isNull);
      expect(pane.toJson()['rect'], [0, 0, 80, 24]);
    });

    test('a short or absent Rect yields zeros rather than throwing', () {
      expect(Rect.fromJson(null), const Rect(0, 0, 0, 0));
      expect(Rect.fromJson([5]), const Rect(5, 0, 0, 0));
      expect(Rect.fromJson('nonsense'), const Rect(0, 0, 0, 0));
    });

    test('[]byte is base64 in both directions', () {
      final clip = Clipboard.fromJson({'t': MsgType.clipboard, 'data': 'aGk='});
      expect(clip.data, [104, 105]);
      expect(clip.toJson()['data'], 'aGk=');
      // Malformed base64 is empty, not an exception: a corrupt clipboard write
      // is not worth dropping the socket for.
      expect(asBytes('!!!not base64!!!'), isEmpty);
    });

    test('json.RawMessage passes through untouched', () {
      final result = CmdResult.fromJson({
        't': MsgType.cmdResult,
        'id': 'c1',
        'ok': true,
        'data': {'text': 'hello'},
      });
      expect(result.data, {'text': 'hello'});
      expect(CaptureResult.fromJson(asObj(result.data)).text, 'hello');
    });

    test('Usage keeps -1 distinct from 0', () {
      final usage = Usage.fromJson({
        't': MsgType.usage,
        'source': 'local',
        'five_hour': {'pct': kUsagePctUnknown, 'detail': '1.2M tokens'},
        'weekly': {'pct': 43.5},
        'weekly_model': {'pct': kUsagePctUnknown},
      });
      expect(usage.fiveHour.pct, kUsagePctUnknown);
      expect(usage.weekly.pct, 43.5);
      expect(usage.weeklyModelName, '');
    });

    test('a nullable pointer field stays null', () {
      final params = SplitParams.fromJson({'direction': SplitDir.h});
      expect(params.pane, isNull, reason: 'nil pane means "the focused pane"');
      expect(params.toJson().containsKey('pane'), isFalse);
    });
  });

  group('command vocabulary', () {
    test('every spec has a name constant', () {
      expect(kCommandSpecs, isNotEmpty);
      for (final spec in kCommandSpecs) {
        expect(spec.name, isNotEmpty);
      }
    });

    test('the reply-gated commands are the ones that only return data', () {
      final gated = kCommandSpecs
          .where((s) => s.replyRequired)
          .map((s) => s.name);
      expect(
        gated,
        containsAll(<String>[
          CmdName.read,
          CmdName.capture,
          CmdName.paneWaitForOutput,
          CmdName.configGet,
          CmdName.themeList,
          CmdName.pluginList,
          CmdName.pathList,
          CmdName.worktreeList,
        ]),
      );
      // pane.send_input has an effect worth performing whether or not anyone is
      // listening, so it must NOT be gated.
      expect(gated, isNot(contains(CmdName.paneSendInput)));
    });
  });

  group('key table', () {
    test('maps the keys a phone actually sends', () {
      // The 0x7 nibble is the HID usage page; PhysicalKeyboardKey.usbHidUsage
      // carries it, so the table must too.
      expect(w3cCodeForUsbHid(0x70004), 'KeyA');
      expect(w3cCodeForUsbHid(0x70028), 'Enter');
      expect(w3cCodeForUsbHid(0x70029), 'Escape');
      expect(w3cCodeForUsbHid(0x70052), 'ArrowUp');
      expect(w3cCodeForUsbHid(0x700E0), 'ControlLeft');
    });

    test('an unknown key is named, not guessed', () {
      expect(w3cCodeForUsbHid(0xDEADBEEF), kUnidentifiedKeyCode);
    });
  });

  group('cell attributes', () {
    test('the bits are distinct powers of two', () {
      const bits = [
        CellAttr.bold,
        CellAttr.dim,
        CellAttr.italic,
        CellAttr.underlined,
        CellAttr.reversed,
        CellAttr.crossedOut,
      ];
      for (final bit in bits) {
        expect(bit & (bit - 1), 0, reason: '$bit is not a single bit');
      }
      expect(bits.toSet().length, bits.length);
    });
  });
}
