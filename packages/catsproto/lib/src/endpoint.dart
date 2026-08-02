import 'dart:io';

import 'sha256.dart';

/// How a client reaches one catway, and how it decides to trust it.
///
/// A phone learns its first endpoint from a `catctl pair` QR: a `cats://pair`
/// URI carrying the URL, a single-use grant, and the served certificate's
/// SHA-256 over the DER. That last field is the whole reason this file has a
/// trust model at all — the device pins out of band, at the moment it first
/// learns the address, instead of trusting whatever certificate a later
/// connection happens to present.

/// Where an endpoint sits, which decides how its certificate is validated.
enum EndpointKind {
  /// A LAN address or a Tailscale name: catway's own self-signed certificate,
  /// trusted by pin.
  direct,

  /// Reached through the relay. Still catway's own certificate — the relay is
  /// an SNI byte pipe that terminates nothing and holds no key — so this is
  /// also pinned. It is a separate kind because it is raced second and its
  /// latency budget is different.
  relay,
}

/// One reachable address for one catway.
class Endpoint {
  const Endpoint({
    required this.id,
    required this.host,
    required this.port,
    this.tls = true,
    this.kind = EndpointKind.direct,
    this.pinnedSha256,
  });

  /// Parses the `cats://pair?u=…&t=…&f=…` deep link `catctl pair` renders into
  /// a QR code.
  ///
  /// The keys are single letters because the URI has to fit a scannable symbol:
  /// a typical LAN pairing URI is 141 bytes, which is a version-8 QR, and cats
  /// has a test (`TestPairURIFitsAQRCode`) asserting it stays there. Spelling
  /// them out would spend that budget on nothing a human reads.
  ///
  /// The grant's expiry is deliberately NOT in the URI for the same reason. It
  /// is five minutes and single use; the honest signal that it has run out is
  /// the 401 from redeeming it, which the app has to handle anyway.
  ///
  /// Returns null rather than throwing on anything malformed. This arrives from
  /// a camera pointed at an arbitrary QR code in the world, so "not one of
  /// ours" is the common case, not an error.
  static PairGrant? parsePairUri(String raw) {
    final uri = Uri.tryParse(raw);
    if (uri == null || uri.scheme != 'cats' || uri.host != 'pair') return null;
    final url = uri.queryParameters['u'];
    final token = uri.queryParameters['t'];
    if (url == null || token == null || token.isEmpty) return null;
    final target = Uri.tryParse(url);
    if (target == null || !target.hasAuthority) return null;
    final tls = target.scheme == 'https' || target.scheme == 'wss';
    final port = target.hasPort ? target.port : (tls ? 443 : 80);
    return PairGrant(
      endpoint: Endpoint(
        id: '${target.host}:$port',
        host: target.host,
        port: port,
        tls: tls,
        // Empty when catway is serving plain HTTP, in which case there is
        // nothing to pin and nothing pinning would protect.
        pinnedSha256: uri.queryParameters['f'],
      ),
      token: token,
    );
  }

  /// Stable key for the token and pin stores. Not a display name.
  final String id;

  final String host;
  final int port;
  final bool tls;
  final EndpointKind kind;

  /// The certificate's SHA-256 over its DER, lowercase hex.
  ///
  /// DER, not the SPKI. catway mints a fresh ECDSA key on every certificate
  /// regeneration (`internal/gwtls`), so SPKI pinning would survive nothing DER
  /// pinning does not — and the DER hash is the value `openssl` and every
  /// browser's certificate viewer show, which makes it checkable by hand.
  final String? pinnedSha256;

  Uri get wsUri =>
      Uri(scheme: tls ? 'wss' : 'ws', host: host, port: port, path: '/ws');

  Uri get httpUri =>
      Uri(scheme: tls ? 'https' : 'http', host: host, port: port, path: '/');

  Endpoint withPin(String fingerprint) => Endpoint(
    id: id,
    host: host,
    port: port,
    tls: tls,
    kind: kind,
    pinnedSha256: fingerprint,
  );

  @override
  String toString() => 'Endpoint($id, ${kind.name}${tls ? ', tls' : ''})';
}

/// What a scanned pairing QR yields: where to connect, with what certificate,
/// and the single-use grant to redeem for a session.
class PairGrant {
  const PairGrant({required this.endpoint, required this.token});

  final Endpoint endpoint;

  /// Worth minutes and exactly one use. Redeem it at `POST /login` with
  /// `password=<token>`; what comes back is an ordinary session, which is the
  /// credential the device actually keeps.
  ///
  /// There is no expiry field, because the URI carries none. A grant that ran
  /// out answers with a 401, which is a path the app has to handle regardless —
  /// a second source of truth for the same fact would only be a way to disagree
  /// with the server about whether the code still works.
  final String token;
}

/// Remembers per-endpoint secrets. The app backs this with
/// flutter_secure_storage; tests use [MemoryTrustStore].
///
/// The session token IS the security model — there is no second factor and no
/// refresh flow — so it never touches shared_preferences.
abstract interface class TrustStore {
  Future<String?> readToken(String endpointId);

  Future<void> writeToken(String endpointId, String token);

  Future<void> clearToken(String endpointId);

  /// The pinned certificate fingerprint, or null when this endpoint has never
  /// been seen.
  Future<String?> readPin(String endpointId);

  Future<void> writePin(String endpointId, String sha256Hex);
}

/// A TrustStore for tests and for a `--demo` build with no server.
class MemoryTrustStore implements TrustStore {
  final Map<String, String> _tokens = {};
  final Map<String, String> _pins = {};

  @override
  Future<String?> readToken(String endpointId) async => _tokens[endpointId];

  @override
  Future<void> writeToken(String endpointId, String token) async {
    _tokens[endpointId] = token;
  }

  @override
  Future<void> clearToken(String endpointId) async {
    _tokens.remove(endpointId);
  }

  @override
  Future<String?> readPin(String endpointId) async => _pins[endpointId];

  @override
  Future<void> writePin(String endpointId, String sha256Hex) async {
    _pins[endpointId] = sha256Hex;
  }
}

/// Why a certificate was refused. Surfaced to the UI, because the two cases
/// need very different words: an unknown endpoint asks the user to pair, and a
/// CHANGED pin on a known endpoint is the one that deserves alarm.
enum CertVerdict { pinned, firstUse, mismatch }

class CertDecision {
  const CertDecision(this.verdict, this.fingerprint);

  final CertVerdict verdict;
  final String fingerprint;

  bool get accepted => verdict != CertVerdict.mismatch;
}

/// Decides whether to accept [der] for an endpoint whose stored pin is
/// [storedPin].
///
/// Trust on first use ONLY when there is no pin and none was carried in from
/// pairing. A pairing QR always carries one, so the honest path never reaches
/// [CertVerdict.firstUse] — it exists for an endpoint typed in by hand, where
/// the alternative is refusing to connect at all.
CertDecision decideCert({
  required List<int> der,
  required String? storedPin,
  required String? pairedPin,
}) {
  final fingerprint = sha256Hex(der);
  final expected = storedPin ?? pairedPin;
  if (expected == null) {
    return CertDecision(CertVerdict.firstUse, fingerprint);
  }
  if (fingerprintsMatch(expected, fingerprint)) {
    return CertDecision(CertVerdict.pinned, fingerprint);
  }
  return CertDecision(CertVerdict.mismatch, fingerprint);
}

/// Builds the HttpClient a WebSocket connect should use for [endpoint].
///
/// `badCertificateCallback` fires only when the platform's own validation has
/// already failed, which for catway's self-signed certificate is always. That
/// makes it the pin check's natural home — a certificate that somehow DID
/// validate publicly never reaches here, and is accepted, which is correct: it
/// means the relay hostname got a real certificate.
HttpClient pinnedHttpClient({
  required Endpoint endpoint,
  required String? storedPin,
  required void Function(CertDecision) onDecision,
}) {
  final client = HttpClient();
  client.badCertificateCallback = (cert, host, port) {
    final decision = decideCert(
      der: cert.der,
      storedPin: storedPin,
      pairedPin: endpoint.pinnedSha256,
    );
    onDecision(decision);
    return decision.accepted;
  };
  return client;
}
