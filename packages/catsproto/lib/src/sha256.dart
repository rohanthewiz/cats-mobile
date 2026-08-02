import 'dart:typed_data';

/// SHA-256, FIPS 180-4.
///
/// Why this is here rather than `package:crypto`: it is needed for exactly one
/// thing — hashing a server certificate's DER so a pin learned at pairing can
/// be checked on every later connect — and `catsproto` is deliberately a
/// zero-runtime-dependency package. Ninety lines of a fully specified,
/// test-vectored algorithm is a smaller liability than a dependency in the
/// package that has to keep working when everything else is stale.
///
/// Not a general crypto library. No streaming API, no HMAC, no constant-time
/// anything — it hashes a certificate, and the input is public.

const List<int> _k = <int>[
  0x428a2f98,
  0x71374491,
  0xb5c0fbcf,
  0xe9b5dba5,
  0x3956c25b,
  0x59f111f1,
  0x923f82a4,
  0xab1c5ed5,
  0xd807aa98,
  0x12835b01,
  0x243185be,
  0x550c7dc3,
  0x72be5d74,
  0x80deb1fe,
  0x9bdc06a7,
  0xc19bf174,
  0xe49b69c1,
  0xefbe4786,
  0x0fc19dc6,
  0x240ca1cc,
  0x2de92c6f,
  0x4a7484aa,
  0x5cb0a9dc,
  0x76f988da,
  0x983e5152,
  0xa831c66d,
  0xb00327c8,
  0xbf597fc7,
  0xc6e00bf3,
  0xd5a79147,
  0x06ca6351,
  0x14292967,
  0x27b70a85,
  0x2e1b2138,
  0x4d2c6dfc,
  0x53380d13,
  0x650a7354,
  0x766a0abb,
  0x81c2c92e,
  0x92722c85,
  0xa2bfe8a1,
  0xa81a664b,
  0xc24b8b70,
  0xc76c51a3,
  0xd192e819,
  0xd6990624,
  0xf40e3585,
  0x106aa070,
  0x19a4c116,
  0x1e376c08,
  0x2748774c,
  0x34b0bcb5,
  0x391c0cb3,
  0x4ed8aa4a,
  0x5b9cca4f,
  0x682e6ff3,
  0x748f82ee,
  0x78a5636f,
  0x84c87814,
  0x8cc70208,
  0x90befffa,
  0xa4506ceb,
  0xbef9a3f7,
  0xc67178f2,
];

int _rotr(int x, int n) => ((x >> n) | (x << (32 - n))) & 0xffffffff;

/// The 32-byte digest of [input].
Uint8List sha256(List<int> input) {
  final h = Uint32List.fromList(<int>[
    0x6a09e667,
    0xbb67ae85,
    0x3c6ef372,
    0xa54ff53a,
    0x510e527f,
    0x9b05688c,
    0x1f83d9ab,
    0x5be0cd19,
  ]);

  // Pad: 0x80, then zeros to 56 mod 64, then the length in BITS, big-endian.
  final bitLen = input.length * 8;
  final padded = Uint8List(((input.length + 9 + 63) ~/ 64) * 64);
  padded.setRange(0, input.length, input);
  padded[input.length] = 0x80;
  final lenView = ByteData.sublistView(padded, padded.length - 8);
  // A certificate is never 2^32 bits, but write the full 64-bit length anyway:
  // truncating here would be a silent wrong answer rather than a failure.
  lenView.setUint32(0, bitLen >> 32, Endian.big);
  lenView.setUint32(4, bitLen & 0xffffffff, Endian.big);

  final w = Uint32List(64);
  final view = ByteData.sublistView(padded);
  for (var chunk = 0; chunk < padded.length; chunk += 64) {
    for (var i = 0; i < 16; i++) {
      w[i] = view.getUint32(chunk + i * 4, Endian.big);
    }
    for (var i = 16; i < 64; i++) {
      final s0 = _rotr(w[i - 15], 7) ^ _rotr(w[i - 15], 18) ^ (w[i - 15] >> 3);
      final s1 = _rotr(w[i - 2], 17) ^ _rotr(w[i - 2], 19) ^ (w[i - 2] >> 10);
      w[i] = (w[i - 16] + s0 + w[i - 7] + s1) & 0xffffffff;
    }

    var a = h[0], b = h[1], c = h[2], d = h[3];
    var e = h[4], f = h[5], g = h[6], hh = h[7];

    for (var i = 0; i < 64; i++) {
      final s1 = _rotr(e, 6) ^ _rotr(e, 11) ^ _rotr(e, 25);
      final ch = (e & f) ^ (~e & g);
      final t1 = (hh + s1 + ch + _k[i] + w[i]) & 0xffffffff;
      final s0 = _rotr(a, 2) ^ _rotr(a, 13) ^ _rotr(a, 22);
      final maj = (a & b) ^ (a & c) ^ (b & c);
      final t2 = (s0 + maj) & 0xffffffff;

      hh = g;
      g = f;
      f = e;
      e = (d + t1) & 0xffffffff;
      d = c;
      c = b;
      b = a;
      a = (t1 + t2) & 0xffffffff;
    }

    h[0] = (h[0] + a) & 0xffffffff;
    h[1] = (h[1] + b) & 0xffffffff;
    h[2] = (h[2] + c) & 0xffffffff;
    h[3] = (h[3] + d) & 0xffffffff;
    h[4] = (h[4] + e) & 0xffffffff;
    h[5] = (h[5] + f) & 0xffffffff;
    h[6] = (h[6] + g) & 0xffffffff;
    h[7] = (h[7] + hh) & 0xffffffff;
  }

  final out = Uint8List(32);
  final outView = ByteData.sublistView(out);
  for (var i = 0; i < 8; i++) {
    outView.setUint32(i * 4, h[i], Endian.big);
  }
  return out;
}

/// Lowercase hex, the form `catctl pair` advertises and `openssl` prints.
String sha256Hex(List<int> input) {
  final digest = sha256(input);
  final sb = StringBuffer();
  for (final byte in digest) {
    sb.write(byte.toRadixString(16).padLeft(2, '0'));
  }
  return sb.toString();
}

/// Compares two hex fingerprints, tolerating case and the colon-separated form
/// some tools print. Not constant-time, and it does not need to be: both sides
/// are public values, and an attacker who can supply the certificate already
/// knows its hash.
bool fingerprintsMatch(String a, String b) {
  String norm(String s) =>
      s.replaceAll(':', '').replaceAll(' ', '').toLowerCase();
  return norm(a) == norm(b);
}
