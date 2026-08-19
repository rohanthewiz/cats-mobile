import 'dart:async';
import 'dart:convert';

import 'package:catsproto/catsproto.dart';
import 'package:test/test.dart';

/// Which window is this phone looking through, and what else could it look
/// through?
///
/// A connection is a view: each desktop window shows one workspace, and a
/// viewer with no pin follows the primary view — whichever window was touched
/// last. These tests cover both halves of that on the client: the fold of the
/// census into a pickable list of windows, and the pin that moves this
/// connection between them.
///
/// The pin is the part worth guarding. `workspace.focus` moves only the sender
/// on a server advertising [Caps.window] and moves EVERY window on one that
/// does not — the single command where a phone can rearrange somebody's desk.
class FakeSocket implements CatsSocket {
  final _controller = StreamController<String>();
  final List<Map<String, Object?>> sent = [];

  @override
  Stream<String> get messages => _controller.stream;

  @override
  void send(String text) => sent.add(jsonDecode(text) as Map<String, Object?>);

  @override
  Future<void> close() async {
    if (!_controller.isClosed) await _controller.close();
  }

  void deliver(Map<String, Object?> json) => _controller.add(jsonEncode(json));
}

const testEndpoint = Endpoint(id: 'test', host: '127.0.0.1', port: 8443);

/// A layout as the server builds it FOR THIS CONNECTION: the active flag is its
/// own view's workspace, not the session's.
Map<String, Object?> layoutShowing(String activeWs) => {
  't': MsgType.layout,
  'workspaces': [
    {'id': 'w1', 'name': 'cats', 'active': activeWs == 'w1'},
    {'id': 'w2', 'name': 'gonotes', 'active': activeWs == 'w2'},
  ],
  'tabs': <Object?>[],
  'panes': <Object?>[],
  'borders': <Object?>[],
};

/// A census with two desktop windows and this phone.
Map<String, Object?> censusOf(List<Map<String, Object?>> views) => {
  't': MsgType.clients,
  'total': views.length,
  'sizers': views.where((v) => v['viewer'] != true).length,
  'cols': 200,
  'rows': 60,
  'views': views,
};

const _windowOnW1 = {
  'workspace': 'w1',
  'cols': 200,
  'rows': 60,
  'focused': true,
  'primary': true,
};
const _windowOnW2 = {'workspace': 'w2', 'cols': 120, 'rows': 40};
const _thisPhone = {'workspace': 'w1', 'viewer': true};

void main() {
  group('the windows a phone can look through', () {
    test('folds the census into desktop windows, viewers left out', () {
      final s = CatsSession();
      s.apply(decodeDown(layoutShowing('w1'))!);
      s.apply(
        decodeDown(censusOf([_windowOnW1, _windowOnW2, _thisPhone]))!,
      );

      final windows = s.desktopWindows;
      expect(windows, hasLength(2), reason: 'the phone is not a window');
      expect(windows.map((w) => w.workspaceId), ['w1', 'w2']);
      // Joined to the layout's names — the census carries ids only.
      expect(windows.map((w) => w.label), ['cats', 'gonotes']);
      expect(windows.first.cols, 200);
      expect(s.viewerCount, 1);
    });

    test('viewWorkspace is the server\'s answer, not a local guess', () {
      final s = CatsSession();
      expect(s.viewWorkspace, '', reason: 'nothing has arrived yet');
      s.apply(decodeDown(layoutShowing('w2'))!);
      expect(s.viewWorkspace, 'w2');
    });

    test('followed marks the window whose workspace this view shows', () {
      final s = CatsSession();
      s.apply(decodeDown(layoutShowing('w2'))!);
      s.apply(decodeDown(censusOf([_windowOnW1, _windowOnW2]))!);

      expect(s.followedWindow?.workspaceId, 'w2');
      expect(s.followedWindow?.label, 'gonotes');
      // The primary is a different window: this phone pinned the other one.
      expect(s.primaryWindow?.workspaceId, 'w1');
      expect(s.primaryWindow?.focused, isTrue);
    });

    test('an unpinned viewer follows the primary, so both agree', () {
      final s = CatsSession();
      s.apply(decodeDown(layoutShowing('w1'))!);
      s.apply(decodeDown(censusOf([_windowOnW1, _windowOnW2]))!);

      expect(s.followedWindow?.workspaceId, s.primaryWindow?.workspaceId);
    });

    test('a census before the first layout still lists windows', () {
      final s = CatsSession();
      s.apply(decodeDown(censusOf([_windowOnW1]))!);

      // No names and nothing followed yet — a UI shows ids rather than treating
      // the ordering of two server messages as an error.
      expect(s.desktopWindows.single.workspaceName, '');
      expect(s.desktopWindows.single.label, 'w1');
      expect(s.followedWindow, isNull);
    });

    test('resetForNewSocket drops the windows with the rest of the fold', () {
      final s = CatsSession();
      s.apply(decodeDown(layoutShowing('w1'))!);
      s.apply(decodeDown(censusOf([_windowOnW1]))!);
      s.resetForNewSocket();

      expect(s.desktopWindows, isEmpty);
      expect(s.viewWorkspace, '');
    });
  });

  group('picking a window', () {
    /// A connection with the welcome already delivered, since every follow call
    /// waits for the capability set.
    CatsConnection connected(FakeSocket socket, {List<String> caps = const []}) {
      final conn = CatsConnection(endpoint: testEndpoint, socket: socket);
      socket.deliver({
        't': MsgType.welcome,
        'v': kProtocolVersion,
        'caps': caps,
      });
      return conn;
    }

    test('the handshake carries the pin, and omits it when there is none', () {
      final unpinned = FakeSocket();
      CatsConnection(endpoint: testEndpoint, socket: unpinned);
      expect(unpinned.sent.single.containsKey('workspace'), isFalse,
          reason: 'absent means "follow the primary view"');

      final pinned = FakeSocket();
      final conn = CatsConnection(
        endpoint: testEndpoint,
        socket: pinned,
        workspace: 'w2',
      );
      // A reconnect lands back on the same window instead of flicking to the
      // desk's for a frame.
      expect(pinned.sent.single['workspace'], 'w2');
      expect(conn.pinnedWorkspace, 'w2');
      expect(conn.followsPrimaryView, isFalse);
      // Still declares no geometry: a pin is not a size.
      expect(pinned.sent.single['cols'], 0);
      expect(pinned.sent.single['rows'], 0);
      expect(pinned.sent.single['viewer'], isTrue);
    });

    test('followWorkspace sends workspace.focus and holds the pin', () async {
      final socket = FakeSocket();
      final conn = connected(socket, caps: [Caps.window]);

      final done = conn.followWorkspace('w2');
      await Future<void>.delayed(Duration.zero);
      final cmd = socket.sent.last;
      expect(cmd['t'], MsgType.cmd);
      expect(cmd['name'], CmdName.workspaceFocus);
      expect((cmd['params'] as Map)['id'], 'w2');

      socket.deliver({'t': MsgType.cmdResult, 'id': cmd['id'], 'ok': true});
      await done;
      expect(conn.pinnedWorkspace, 'w2');
      await conn.close();
    });

    test('followPrimaryView releases it with an empty id', () async {
      final socket = FakeSocket();
      final conn = connected(socket, caps: [Caps.window]);

      final done = conn.followPrimaryView();
      await Future<void>.delayed(Duration.zero);
      final cmd = socket.sent.last;
      expect((cmd['params'] as Map)['id'], '',
          reason: 'empty is the wire form of "follow the primary"');

      socket.deliver({'t': MsgType.cmdResult, 'id': cmd['id'], 'ok': true});
      await done;
      expect(conn.followsPrimaryView, isTrue);
      await conn.close();
    });

    test('a rejected follow leaves the pin alone', () async {
      final socket = FakeSocket();
      final conn = CatsConnection(
        endpoint: testEndpoint,
        socket: socket,
        workspace: 'w1',
      );
      socket.deliver({
        't': MsgType.welcome,
        'v': kProtocolVersion,
        'caps': [Caps.window],
      });

      final done = conn.followWorkspace('w9');
      await Future<void>.delayed(Duration.zero);
      final cmd = socket.sent.last;
      socket.deliver({
        't': MsgType.cmdResult,
        'id': cmd['id'],
        'ok': false,
        'error': 'unknown workspace w9',
      });

      await expectLater(done, throwsA(isA<CatsCommandError>()));
      // A pin the server refused must not survive into the next handshake,
      // where it would be silently fallen back a second time.
      expect(conn.pinnedWorkspace, 'w1');
      await conn.close();
    });

    test('refuses to send it to a server without the window capability',
        () async {
      final socket = FakeSocket();
      final conn = connected(socket, caps: [Caps.viewer, Caps.clients]);
      final beforeSend = socket.sent.length;

      await expectLater(
        conn.followWorkspace('w2'),
        throwsA(isA<ViewerModeViolation>()),
      );
      // Nothing went on the wire: there, workspace.focus is the old
      // session-wide switch and would move every window at the desk.
      expect(socket.sent.length, beforeSend);
      expect(conn.followsPrimaryView, isTrue);
      await conn.close();
    });
  });
}
