import 'dart:convert';

import 'package:catsproto/catsproto.dart';
import 'package:test/test.dart';

void main() {
  group('sha256', () {
    test('matches the published vectors', () {
      // FIPS 180-4 / RFC 6234. A hand-rolled hash with no test vectors is a
      // hash that is wrong in a way nobody notices until it matters.
      expect(
        sha256Hex(utf8.encode('')),
        'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
      );
      expect(
        sha256Hex(utf8.encode('abc')),
        'ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad',
      );
      expect(
        sha256Hex(
          utf8.encode(
            'abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq',
          ),
        ),
        '248d6a61d20638b8e5c026930c3e6039a33ce45964ff2167f6ecedd419db06c1',
      );
    });

    test('handles inputs that straddle a block boundary', () {
      // 55, 56 and 64 bytes: the padding edge cases, where a wrong length field
      // or an off-by-one in the block count shows up and nowhere else.
      for (final n in [54, 55, 56, 57, 63, 64, 65, 119, 120]) {
        final digest = sha256Hex(List<int>.filled(n, 0x61));
        expect(digest.length, 64, reason: 'length $n');
      }
      expect(
        sha256Hex(List<int>.filled(56, 0x61)),
        'b35439a4ac6f0948b6d6f9e3c6af0f5f590ce20f1bde7090ef7970686ec6738a',
      );
    });

    test('fingerprints compare across formatting', () {
      expect(fingerprintsMatch('AB:CD:EF', 'abcdef'), isTrue);
      expect(fingerprintsMatch('ab cd ef', 'ABCDEF'), isTrue);
      expect(fingerprintsMatch('abcdef', 'abcdee'), isFalse);
    });
  });

  group('pair URI', () {
    test('parses what catctl pair mints', () {
      // Byte-for-byte the shape catctl's pairURI builds: single-letter keys,
      // no expiry. Verified against a live `catctl pair --json` run.
      final grant = Endpoint.parsePairUri(
        'cats://pair?f=aa11bb22'
        '&t=abc123'
        '&u=https%3A%2F%2F192.168.2.52%3A8443',
      );
      expect(grant, isNotNull);
      expect(grant!.token, 'abc123');
      expect(grant.endpoint.host, '192.168.2.52');
      expect(grant.endpoint.port, 8443);
      expect(grant.endpoint.tls, isTrue);
      expect(grant.endpoint.pinnedSha256, 'aa11bb22');
    });

    test('plain HTTP pairs with no pin, because there is nothing to pin', () {
      final grant = Endpoint.parsePairUri(
        'cats://pair?t=tok&u=http%3A%2F%2F10.0.0.5%3A8421',
      )!;
      expect(grant.endpoint.tls, isFalse);
      expect(grant.endpoint.pinnedSha256, isNull);
      expect(grant.endpoint.wsUri.toString(), 'ws://10.0.0.5:8421/ws');
    });

    test(
      'returns null for anything else, because a camera sees everything',
      () {
        expect(Endpoint.parsePairUri('https://example.com'), isNull);
        expect(Endpoint.parsePairUri('cats://pair'), isNull);
        expect(
          Endpoint.parsePairUri('cats://pair?url=nonsense&token=t'),
          isNull,
        );
        expect(Endpoint.parsePairUri('WIFI:S:home;T:WPA;P:hunter2;;'), isNull);
      },
    );

    test('builds a /ws URL from the paired address', () {
      final grant = Endpoint.parsePairUri(
        'cats://pair?u=https%3A%2F%2Fhost%3A9000&t=t',
      )!;
      expect(grant.endpoint.wsUri.toString(), 'wss://host:9000/ws');
    });
  });

  group('certificate decision', () {
    final der = utf8.encode('a certificate, pretend');
    final fingerprint = sha256Hex(der);

    test('a matching pin is accepted', () {
      final d = decideCert(der: der, storedPin: fingerprint, pairedPin: null);
      expect(d.verdict, CertVerdict.pinned);
      expect(d.accepted, isTrue);
    });

    test('the pin from pairing counts before anything is stored', () {
      final d = decideCert(der: der, storedPin: null, pairedPin: fingerprint);
      expect(d.verdict, CertVerdict.pinned);
    });

    test('a CHANGED pin is refused, and says so distinctly', () {
      // The two rejections need different words. "We have not met" asks the
      // user to pair; "this is not the certificate we pinned" is the one that
      // deserves alarm, and conflating them trains people to tap through both.
      final d = decideCert(der: der, storedPin: 'ff' * 32, pairedPin: null);
      expect(d.verdict, CertVerdict.mismatch);
      expect(d.accepted, isFalse);
      expect(
        d.fingerprint,
        fingerprint,
        reason: 'so the UI can show what it saw',
      );
    });

    test('an unpinned endpoint is trust-on-first-use', () {
      final d = decideCert(der: der, storedPin: null, pairedPin: null);
      expect(d.verdict, CertVerdict.firstUse);
      expect(d.accepted, isTrue);
    });
  });

  group('MemoryTrustStore', () {
    test('keeps tokens and pins apart', () async {
      final store = MemoryTrustStore();
      await store.writeToken('e1', 'sess');
      await store.writePin('e1', 'aa');
      expect(await store.readToken('e1'), 'sess');
      expect(await store.readPin('e1'), 'aa');
      await store.clearToken('e1');
      expect(await store.readToken('e1'), isNull);
      expect(
        await store.readPin('e1'),
        'aa',
        reason: 'signing out must not forget which server this is',
      );
    });
  });
}
