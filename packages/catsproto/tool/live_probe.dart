// A live end-to-end check against a real catway. Not a unit test: it needs a
// server, so it is a tool rather than something `dart test` would try to run.
//
//   dart run tool/live_probe.dart '<cats://pair?...>'
//
// It walks the whole path a phone takes on its first launch — scan, pin,
// redeem, connect, query — and prints what it saw at each step, so a failure
// names the step rather than the symptom.
import 'dart:convert';
import 'dart:io';

import 'package:catsproto/catsproto.dart';

Future<int> main(List<String> args) async {
  if (args.isEmpty) {
    stderr.writeln('usage: dart run tool/live_probe.dart "<cats://pair?...>"');
    return 2;
  }

  final grant = Endpoint.parsePairUri(args.first);
  if (grant == null) {
    stderr.writeln('not a cats pairing URI: ${args.first}');
    return 1;
  }
  final endpoint = grant.endpoint;
  print('1. scanned    ${endpoint.host}:${endpoint.port} tls=${endpoint.tls}');
  print('   pin        ${endpoint.pinnedSha256}');

  // Redeem the grant for a session. The device never learns the password.
  String? seenFingerprint;
  final http = pinnedHttpClient(
    endpoint: endpoint,
    storedPin: null,
    onDecision: (d) {
      seenFingerprint = d.fingerprint;
      print('2. cert      ${d.verdict.name}  ${d.fingerprint}');
    },
  );
  final loginReq = await http.postUrl(endpoint.httpUri.replace(path: '/login'));
  loginReq.headers.set('content-type', 'application/x-www-form-urlencoded');
  loginReq.headers.set('accept', 'application/json');
  loginReq.write('password=${Uri.encodeQueryComponent(grant.token)}');
  final loginRes = await loginReq.close();
  final body = await loginRes.transform(utf8.decoder).join();
  if (loginRes.statusCode != 200) {
    stderr.writeln('3. redeem    FAILED ${loginRes.statusCode}: $body');
    return 1;
  }
  final token = (jsonDecode(body) as Map)['session'] as String;
  print('3. redeem    session ${token.substring(0, 12)}…');

  if (seenFingerprint != null &&
      !fingerprintsMatch(seenFingerprint!, endpoint.pinnedSha256 ?? '')) {
    stderr.writeln('   PIN MISMATCH — advertised ${endpoint.pinnedSha256}');
    return 1;
  }

  final socket = await WebSocketCatsSocket.connect(
    endpoint: endpoint,
    token: token,
    storedPin: endpoint.pinnedSha256,
    onCertDecision: (_) {},
  );
  final conn = CatsConnection(endpoint: endpoint, socket: socket);
  final session = CatsSession();
  conn.messages.listen(session.apply);

  final welcome = await conn.welcome;
  print('4. welcome   v${welcome.v} caps=${welcome.caps}');

  // The census is the direct evidence for viewer mode: `sizers` counts the
  // connections that declared a grid, and `cols`/`rows` is the grid they
  // settled on. A phone that had sized the session would show up in both.
  await Future<void>.delayed(const Duration(milliseconds: 200));
  final census = session.clients;
  if (census != null) {
    print(
      '   clients   total=${census.total} sizers=${census.sizers} '
      'grid=${census.cols}x${census.rows}',
    );
  }

  final panes = await conn.paneList();
  print('5. pane.list ${panes.panes.length} pane(s)');
  for (final p in panes.panes) {
    print(
      '             ${p.handle} focused=${p.focused} visible=${p.visible} '
      'agent=${p.agent.isEmpty ? "-" : p.agent} title=${p.title}',
    );
  }

  final before = await conn.sessionGet();
  print(
    '6. session   ws=${before.activeWorkspace} focus=${before.focusedPane} '
    'panes=${before.panes}',
  );

  final target = panes.panes.first.pane;
  await conn.paneSendInput(
    SendInputParams(pane: target, text: 'echo hello-from-dart', submit: true),
  );
  await Future<void>.delayed(const Duration(milliseconds: 700));
  final cap = await conn.capture(
    CaptureParams(pane: target, scope: 1, lines: 30, unwrap: true),
  );
  final hit = cap.text.contains('hello-from-dart');
  print('7. send+cap  ${hit ? "SAW the echo" : "did NOT see the echo"}');

  // The assertion that matters most: nothing the app did moved the desktop.
  final after = await conn.sessionGet();
  final unmoved =
      after.activeWorkspace == before.activeWorkspace &&
      after.focusedPane == before.focusedPane;
  print('8. viewport  ${unmoved ? "UNCHANGED" : "MOVED — a viewer must not"}');

  await conn.close();
  return (hit && unmoved) ? 0 : 1;
}
