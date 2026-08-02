import 'dart:async';
import 'dart:convert';
import 'dart:math';

import 'package:catsproto/catsproto.dart';
import 'package:test/test.dart';

/// A CatsSocket that records what was sent and lets a test push messages back.
///
/// This is the same seam TranscriptConnection will use to replay recorded
/// JSONL, so the code under test is the real fold, not a stand-in for it.
class FakeSocket implements CatsSocket {
  final _controller = StreamController<String>();
  final List<Map<String, Object?>> sent = [];
  bool closed = false;

  @override
  Stream<String> get messages => _controller.stream;

  @override
  void send(String text) => sent.add(jsonDecode(text) as Map<String, Object?>);

  @override
  Future<void> close() async {
    closed = true;
    if (!_controller.isClosed) await _controller.close();
  }

  void deliver(Map<String, Object?> json) => _controller.add(jsonEncode(json));

  void deliverRaw(String text) => _controller.add(text);

  Future<void> drop() => close();
}

const testEndpoint = Endpoint(id: 'test', host: '127.0.0.1', port: 8443);

void main() {
  group('handshake', () {
    test('declares nothing: the phone never sizes the desktop grid', () {
      final socket = FakeSocket();
      CatsConnection(endpoint: testEndpoint, socket: socket);

      final init = socket.sent.single;
      expect(init['t'], MsgType.init);
      expect(init['v'], kProtocolVersion);
      // catway's registerConn has TWO guards, both keyed on > 0: the session
      // grid, and the cell metrics that ride β create_pane/resize. Clearing one
      // and not the other is the easy mistake, so both are asserted.
      expect(init['cols'], 0);
      expect(init['rows'], 0);
      expect(init['cell_w_px'], 0);
      expect(init['cell_h_px'], 0);
      expect(init['viewer'], isTrue);
    });

    test('completes welcome and records caps', () async {
      final socket = FakeSocket();
      final conn = CatsConnection(endpoint: testEndpoint, socket: socket);
      socket.deliver({
        't': MsgType.welcome,
        'v': kProtocolVersion,
        'caps': [Caps.viewer, Caps.keyPane, Caps.clients],
      });
      final welcome = await conn.welcome;
      expect(welcome.v, kProtocolVersion);
      expect(conn.caps, contains(Caps.keyPane));
      await conn.close();
    });

    test('a rejected welcome fails the future rather than hanging', () async {
      final socket = FakeSocket();
      final conn = CatsConnection(endpoint: testEndpoint, socket: socket);
      socket.deliver({
        't': MsgType.welcome,
        'v': 2,
        'error': 'protocol version',
      });
      await expectLater(conn.welcome, throwsA(isA<CatsCommandError>()));
      await conn.close();
    });
  });

  group('viewer mode', () {
    test('send refuses Resize', () {
      final socket = FakeSocket();
      final conn = CatsConnection(endpoint: testEndpoint, socket: socket);
      // Constructing one is already an analyzer error in this package (the
      // generator marks it @Deprecated and analysis_options promotes that to an
      // error); this asserts the runtime layer under it. The ignore has to sit
      // on the construction itself, so it gets its own statement — the
      // formatter is free to move an expression onto a later line, and a
      // silenced lint that drifts off its target silently stops silencing.
      // ignore: deprecated_member_use_from_same_package
      const resize = Resize(cols: 40, rows: 20);
      expect(() => conn.send(resize), throwsA(isA<ViewerModeViolation>()));
      expect(socket.sent.length, 1, reason: 'only the handshake');
    });

    test('send refuses a second init', () {
      final socket = FakeSocket();
      final conn = CatsConnection(endpoint: testEndpoint, socket: socket);
      expect(
        () => conn.send(
          const Init(
            v: kProtocolVersion,
            cols: 200,
            rows: 60,
            dpr: 3,
            cellWPx: 8,
            cellHPx: 16,
          ),
        ),
        throwsA(isA<ViewerModeViolation>()),
      );
    });

    test('ordinary up-messages go through', () {
      final socket = FakeSocket();
      CatsConnection(endpoint: testEndpoint, socket: socket).send(
        const Key(
          pane: 3,
          code: 'Escape',
          key: 'Escape',
          mods: 0,
          kind: KeyKind.down,
        ),
      );
      expect(socket.sent.last['t'], MsgType.key);
      expect(socket.sent.last['pane'], 3);
    });
  });

  group('command correlation', () {
    test('a reply resolves the matching call', () async {
      final socket = FakeSocket();
      final conn = CatsConnection(endpoint: testEndpoint, socket: socket);

      final future = conn.capture(
        const CaptureParams(pane: 1, scope: 1, lines: 200, unwrap: true),
      );
      final sent = socket.sent.last;
      expect(sent['t'], MsgType.cmd);
      expect(sent['name'], CmdName.capture);
      expect(
        sent['id'],
        isNotNull,
        reason: 'capture is reply-gated: no id, no run',
      );
      expect((sent['params']! as Map)['unwrap'], isTrue);

      socket.deliver({
        't': MsgType.cmdResult,
        'id': sent['id'],
        'ok': true,
        'data': {'text': 'hello from the pane'},
      });
      expect((await future).text, 'hello from the pane');
      await conn.close();
    });

    test('an ok:false reply throws with the command name attached', () async {
      final socket = FakeSocket();
      final conn = CatsConnection(endpoint: testEndpoint, socket: socket);
      final future = conn.paneSendInput(
        const SendInputParams(pane: 1, text: 'hi', submit: true),
      );
      socket.deliver({
        't': MsgType.cmdResult,
        'id': socket.sent.last['id'],
        'ok': false,
        'error': 'workspace is locked',
      });
      await expectLater(
        future,
        throwsA(
          isA<CatsCommandError>()
              .having((e) => e.command, 'command', CmdName.paneSendInput)
              .having((e) => e.message, 'message', 'workspace is locked'),
        ),
      );
      await conn.close();
    });

    test('concurrent calls do not cross their replies', () async {
      final socket = FakeSocket();
      final conn = CatsConnection(endpoint: testEndpoint, socket: socket);
      final a = conn.capture(const CaptureParams(pane: 1));
      final b = conn.capture(const CaptureParams(pane: 2));
      final idA = socket.sent[socket.sent.length - 2]['id'];
      final idB = socket.sent.last['id'];
      expect(idA, isNot(idB));

      // Answer out of order — which is the normal case, since the two panes'
      // captures resolve independently server-side.
      socket.deliver({
        't': MsgType.cmdResult,
        'id': idB,
        'ok': true,
        'data': {'text': 'B'},
      });
      socket.deliver({
        't': MsgType.cmdResult,
        'id': idA,
        'ok': true,
        'data': {'text': 'A'},
      });
      expect((await a).text, 'A');
      expect((await b).text, 'B');
      await conn.close();
    });

    test('every pending call fails on disconnect', () async {
      // The property that matters: a leaked completer is a spinner that never
      // stops, and the user has no way to make it stop short of killing the app.
      final socket = FakeSocket();
      final conn = CatsConnection(endpoint: testEndpoint, socket: socket);
      final a = conn.capture(const CaptureParams(pane: 1));
      final b = conn.paneList();
      await socket.drop();
      await expectLater(a, throwsA(isA<CatsDisconnectedError>()));
      await expectLater(b, throwsA(isA<CatsDisconnectedError>()));
    });

    test('a call times out rather than waiting forever', () async {
      final socket = FakeSocket();
      final conn = CatsConnection(
        endpoint: testEndpoint,
        socket: socket,
        defaultTimeout: const Duration(milliseconds: 30),
      );
      await expectLater(
        conn.capture(const CaptureParams(pane: 1)),
        throwsA(isA<TimeoutException>()),
      );
      await conn.close();
    });

    test('a late reply to a timed-out call does not blow up', () async {
      final socket = FakeSocket();
      final conn = CatsConnection(
        endpoint: testEndpoint,
        socket: socket,
        defaultTimeout: const Duration(milliseconds: 20),
      );
      final id = () {
        final f = conn.capture(const CaptureParams(pane: 1));
        unawaited(f.catchError((_) => const CaptureResult(text: '')));
        return socket.sent.last['id'];
      }();
      await Future<void>.delayed(const Duration(milliseconds: 50));
      socket.deliver({
        't': MsgType.cmdResult,
        'id': id,
        'ok': true,
        'data': {'text': 'late'},
      });
      await Future<void>.delayed(const Duration(milliseconds: 10));
      await conn.close();
    });
  });

  group('message stream', () {
    test('publishes decoded messages and drops unknown ones', () async {
      final socket = FakeSocket();
      final conn = CatsConnection(endpoint: testEndpoint, socket: socket);
      final seen = <Object>[];
      conn.messages.listen(seen.add);

      socket.deliver({'t': MsgType.title, 'title': 'cats'});
      socket.deliver({'t': 'from_a_newer_server'});
      socket.deliverRaw('{not json at all');
      socket.deliver({'t': MsgType.paneExited, 'pane': 2, 'code': 130});
      await Future<void>.delayed(Duration.zero);

      expect(seen.map((m) => m.runtimeType), [Title, PaneExited]);
      await conn.close();
    });
  });

  group('Backoff', () {
    test('climbs to the ceiling and resets', () {
      final b = Backoff(random: Random(1));
      final first = b.next();
      expect(first.inMilliseconds, inInclusiveRange(400, 600));
      for (var i = 0; i < 10; i++) {
        b.next();
      }
      expect(b.next().inMilliseconds, inInclusiveRange(24000, 36000));
      b.reset();
      expect(b.next().inMilliseconds, inInclusiveRange(400, 600));
    });

    test(
      'jitters, so a network that came back does not produce a lockstep herd',
      () {
        final b = Backoff(random: Random(7));
        final samples = <int>{};
        for (var i = 0; i < 20; i++) {
          b.reset();
          samples.add(b.next().inMilliseconds);
        }
        expect(samples.length, greaterThan(5));
      },
    );
  });
}
