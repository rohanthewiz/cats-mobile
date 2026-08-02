import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:math';

import 'endpoint.dart';
import 'generated/codec.g.dart';
import 'generated/commands.g.dart';
import 'generated/wire.g.dart';

/// The socket a [CatsConnection] drives.
///
/// Abstracted so a test can replay a recorded transcript through the exact same
/// fold the live client uses. Every widget test, every golden, and demo mode
/// run on the same code path as the real thing — the alternative is a mock that
/// agrees with the client and disagrees with the server.
abstract interface class CatsSocket {
  Stream<String> get messages;

  void send(String text);

  Future<void> close();
}

/// A real WebSocket, with the two hooks `package:web_socket_channel` cannot
/// give us: an `Authorization` header on the upgrade, and a certificate
/// callback. Both are load-bearing — the bearer token IS the credential, and
/// catway serves a self-signed certificate that only a pin can validate.
class WebSocketCatsSocket implements CatsSocket {
  WebSocketCatsSocket._(this._ws);

  static Future<WebSocketCatsSocket> connect({
    required Endpoint endpoint,
    required String token,
    String? storedPin,
    required void Function(CertDecision) onCertDecision,
    Duration pingInterval = const Duration(seconds: 20),
  }) async {
    final ws = await WebSocket.connect(
      endpoint.wsUri.toString(),
      // No Origin header is sent. gwauth.OriginOK returns true for an empty
      // Origin ("non-browser client… auth is still enforced"), so a native
      // client passes the check for free — the header exists to stop a hostile
      // PAGE, and there is no page here.
      headers: {'Authorization': 'Bearer $token'},
      customClient: endpoint.tls
          ? pinnedHttpClient(
              endpoint: endpoint,
              storedPin: storedPin,
              onDecision: onCertDecision,
            )
          : null,
    );
    // Dart replies to server pings in the platform layer; this is the other
    // direction, so a NAT that drops an idle flow gets traffic to keep. catway
    // pings at 30s and reads with a 90s deadline, so 20s here is comfortably
    // inside its window without being chatty on cellular.
    ws.pingInterval = pingInterval;
    return WebSocketCatsSocket._(ws);
  }

  final WebSocket _ws;

  @override
  Stream<String> get messages => _ws.map((e) => e is String ? e : '');

  @override
  void send(String text) => _ws.add(text);

  @override
  Future<void> close() => _ws.close();
}

/// Thrown when a command comes back `ok: false`.
class CatsCommandError implements Exception {
  CatsCommandError(this.command, this.message);

  final String command;
  final String message;

  @override
  String toString() => 'CatsCommandError($command): $message';
}

/// Thrown when a command is still outstanding at disconnect. Distinct from a
/// timeout so the UI can say "reconnecting" instead of "the server is slow".
class CatsDisconnectedError implements Exception {
  CatsDisconnectedError(this.command);

  final String command;

  @override
  String toString() =>
      'CatsDisconnectedError($command): the socket closed first';
}

/// Thrown by [CatsConnection.send] for a message a viewer must never emit.
class ViewerModeViolation implements Exception {
  ViewerModeViolation(this.what);

  final String what;

  @override
  String toString() =>
      'ViewerModeViolation: a cats client never sends $what — it would reshape '
      'the desktop it is looking at';
}

/// One live session with one catway.
///
/// # The phone never resizes the desktop
///
/// catway's `registerConn` takes the session grid from the FIRST `init` that
/// declares one, and every connection shares that grid. A phone honestly
/// reporting 40×20 reflows every pane for everybody. Four independent layers
/// stop that, and this class is two of them:
///
///   1. [_handshake] builds the `Init` itself, with hardcoded zeros and
///      `viewer: true`. App code has no way to supply one.
///   2. [send] refuses `Resize` outright.
///   3. the generator marks `Resize` `@Deprecated`, and analysis_options.yaml
///      promotes `deprecated_member_use` to an error.
///   4. a source test fails on any `Resize(` or non-zero `cols:` under lib/.
///
/// Belt and braces is warranted here because it is the one failure mode where a
/// bug in the phone breaks the user's desktop.
class CatsConnection with CatsCommands implements CatsCommandTransport {
  CatsConnection({
    required this.endpoint,
    required CatsSocket socket,
    this.defaultTimeout = const Duration(seconds: 30),
    this.maxTimeout = const Duration(minutes: 10),
  }) : _socket = socket {
    _subscription = _socket.messages.listen(
      _onText,
      onError: (Object e) => _fail(e),
      onDone: () => _fail(CatsDisconnectedError('<socket closed>')),
      cancelOnError: false,
    );
    // A caller that never awaits [welcome] — the common case once the session
    // is up — must not turn a disconnect into an unhandled async error that
    // takes down the zone. ignore() registers a discarding listener; a real
    // await still receives the error, because a Future can have several.
    _welcome.future.ignore();
    _handshake();
  }

  final Endpoint endpoint;

  /// Bounds an ordinary command. `pane.wait_for_output` overrides it and can
  /// legitimately run to [maxTimeout] — app.MaxWaitTimeout is ten minutes, and
  /// a client deadline shorter than that would abandon a wait the server is
  /// still faithfully serving.
  final Duration defaultTimeout;
  final Duration maxTimeout;

  final CatsSocket _socket;
  late final StreamSubscription<String> _subscription;

  final _incoming = StreamController<Object>.broadcast();
  final Map<String, _Pending> _pending = {};
  final _welcome = Completer<Welcome>();
  int _nextId = 0;
  bool _closed = false;

  /// Every decoded down-message, in arrival order. Unknown types never appear —
  /// [decodeDown] drops them, which is the protocol's rule.
  Stream<Object> get messages => _incoming.stream;

  /// Completes with the server's `welcome`, or with an error when the server
  /// rejected the handshake (a protocol-version mismatch) or the socket died
  /// first.
  Future<Welcome> get welcome => _welcome.future;

  /// The capabilities the server advertised, once [welcome] has arrived. Empty
  /// before that — and an empty set is a real answer, not "unknown": a server
  /// old enough to send no caps honours none of them.
  Set<String> get caps => _caps;
  Set<String> _caps = const {};

  bool get isClosed => _closed;

  void _handshake() {
    // The zeros are the point. catway's registerConn skips the o.area
    // assignment when cols/rows are 0, and skips the cell metrics when those
    // are 0 — two separate guards, both of which a viewer must clear. Sending
    // `viewer: true` as well is belt and braces against a server that honours
    // one and forgets the other.
    const init = Init(
      v: kProtocolVersion,
      cols: 0,
      rows: 0,
      dpr: 0,
      cellWPx: 0,
      cellHPx: 0,
      viewer: true,
    );
    _socket.send(jsonEncode(init.toJson()));
  }

  /// Sends one up-message.
  ///
  /// Refuses `Resize` regardless of what the caller intended: see the class doc.
  void send(Object message) {
    if (_closed) throw CatsDisconnectedError('<send>');
    // The type test is the ONLY use of Resize in this package, and it exists to
    // refuse it. Naming a deprecated type in order to reject it is exactly the
    // case the lint cannot distinguish, so it is silenced here and nowhere else.
    // ignore: deprecated_member_use_from_same_package
    if (message is Resize) throw ViewerModeViolation('resize');
    if (message is Init) {
      throw ViewerModeViolation(
        'a second init (the handshake is ours to build)',
      );
    }
    final json = switch (message) {
      final Key m => m.toJson(),
      final Mouse m => m.toJson(),
      final Paste m => m.toJson(),
      final Image m => m.toJson(),
      final Cmd m => m.toJson(),
      _ => throw ArgumentError('not an up-message: ${message.runtimeType}'),
    };
    _socket.send(jsonEncode(json));
  }

  @override
  Future<Object?> invoke(String name, Object? params) {
    if (_closed) return Future.error(CatsDisconnectedError(name));
    final id = 'c${_nextId++}';
    final pending = _Pending(name, Completer<Object?>());
    _pending[id] = pending;

    _socket.send(jsonEncode(Cmd(id: id, name: name, params: params).toJson()));

    // Every call is bounded. A wait that resolves only on a server event is the
    // one case where a long deadline is correct rather than lazy — but even
    // that one has a deadline, because a leaked completer is a spinner that
    // never stops.
    final limit = name == CmdName.paneWaitForOutput
        ? maxTimeout
        : defaultTimeout;
    pending.timer = Timer(limit, () {
      if (_pending.remove(id) != null) {
        pending.completer.completeError(
          TimeoutException('$name did not answer within $limit'),
        );
      }
    });
    return pending.completer.future;
  }

  void _onText(String text) {
    final json = decodeFrame(text);
    if (json == null) return; // malformed: drop the message, keep the socket

    final msg = decodeDown(json);
    if (msg == null) return; // unknown "t": the protocol says ignore it

    if (msg is Welcome) {
      _caps = Set.unmodifiable(msg.caps);
      if (!_welcome.isCompleted) {
        if (msg.error.isNotEmpty) {
          _welcome.completeError(CatsCommandError('init', msg.error));
        } else {
          _welcome.complete(msg);
        }
      }
    }
    if (msg is CmdResult) {
      final pending = _pending.remove(msg.id);
      if (pending != null) {
        pending.timer?.cancel();
        if (msg.ok) {
          pending.completer.complete(msg.data);
        } else {
          pending.completer.completeError(
            CatsCommandError(pending.name, msg.error),
          );
        }
      }
      // A CmdResult with no pending entry is not an error: it is the reply to a
      // call that already timed out. Falling through publishes it on [messages]
      // anyway, where a debug view can see it.
    }
    if (!_incoming.isClosed) _incoming.add(msg);
  }

  /// Fails every outstanding call. Called on socket error and on close.
  ///
  /// Every pending completer must fail, without exception. The alternative is
  /// a Future nobody ever completes, which in Flutter is a spinner that spins
  /// until the app is killed.
  void _fail(Object error) {
    if (_closed) return;
    _closed = true;
    for (final entry in _pending.entries) {
      entry.value.timer?.cancel();
      if (!entry.value.completer.isCompleted) {
        entry.value.completer.completeError(
          error is CatsDisconnectedError
              ? CatsDisconnectedError(entry.value.name)
              : error,
        );
      }
    }
    _pending.clear();
    if (!_welcome.isCompleted) _welcome.completeError(error);
    if (!_incoming.isClosed) _incoming.close();
  }

  Future<void> close() async {
    _fail(CatsDisconnectedError('<close>'));
    await _subscription.cancel();
    await _socket.close();
  }
}

class _Pending {
  _Pending(this.name, this.completer);

  final String name;
  final Completer<Object?> completer;
  Timer? timer;
}

/// Reconnect backoff: 500 ms doubling to 30 s, with ±20% jitter.
///
/// The jitter is not decoration. Every phone on a network that dropped comes
/// back at the same moment, and an unjittered ladder has them all knocking in
/// lockstep — which is how a home server that just restarted gets a thundering
/// herd instead of a queue.
class Backoff {
  Backoff({
    this.initial = const Duration(milliseconds: 500),
    this.ceiling = const Duration(seconds: 30),
    Random? random,
  }) : _random = random ?? Random();

  final Duration initial;
  final Duration ceiling;
  final Random _random;

  int _attempt = 0;

  /// Call on a successful connect, on resume, on a network change, and on
  /// pull-to-refresh. A user who pulled to refresh is telling you they think it
  /// should work now; making them wait out a 30 s ladder is answering an
  /// explicit request with a shrug.
  void reset() => _attempt = 0;

  Duration next() {
    final scaled = initial.inMilliseconds * (1 << _attempt.clamp(0, 20));
    final capped = min(scaled, ceiling.inMilliseconds);
    _attempt++;
    final jitter = 1.0 + (_random.nextDouble() * 0.4 - 0.2);
    return Duration(milliseconds: (capped * jitter).round());
  }
}
