package catsclient

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"strings"
)

// CertVerdict is why a certificate was accepted or refused. Surfaced to the UI,
// because the two rejections need very different words: an unknown endpoint
// asks the user to pair, and a CHANGED pin on a known endpoint is the one that
// deserves alarm. Conflating them trains people to tap through both.
type CertVerdict int

const (
	// VerdictPinned: the certificate matches the stored or paired pin.
	VerdictPinned CertVerdict = iota
	// VerdictFirstUse: no pin exists at all; accepted on trust-on-first-use.
	VerdictFirstUse
	// VerdictMismatch: a pin exists and this certificate is not it. Refused.
	VerdictMismatch
)

func (v CertVerdict) String() string {
	switch v {
	case VerdictPinned:
		return "pinned"
	case VerdictFirstUse:
		return "first-use"
	default:
		return "mismatch"
	}
}

// CertDecision is a verdict plus the fingerprint it was reached about, so the
// UI can show the user what it saw.
type CertDecision struct {
	Verdict     CertVerdict
	Fingerprint string
}

// Accepted reports whether the connection may proceed.
func (d CertDecision) Accepted() bool { return d.Verdict != VerdictMismatch }

// ErrCertMismatch is what the TLS handshake fails with when DecideCert refuses
// the certificate; errors.Is on a dial error tells the app to show the
// "this is not the server we pinned" screen rather than "reconnecting".
var ErrCertMismatch = errors.New("catsclient: certificate does not match the pinned fingerprint")

// FingerprintSHA256 is the lowercase hex SHA-256 of a certificate's DER, the
// same value catway prints and `openssl x509 -fingerprint -sha256` shows.
func FingerprintSHA256(der []byte) string {
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:])
}

// FingerprintsMatch compares two fingerprints regardless of formatting: colons,
// spaces and case are all things a human or a tool might have added.
func FingerprintsMatch(a, b string) bool {
	return normalizeFingerprint(a) == normalizeFingerprint(b)
}

func normalizeFingerprint(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, ":", "")
	s = strings.ReplaceAll(s, " ", "")
	return s
}

// DecideCert decides whether to accept der for an endpoint whose stored pin is
// storedPin and whose pairing carried pairedPin. Either may be "" for none.
//
// Trust on first use ONLY when there is no pin and none was carried in from
// pairing. A pairing QR always carries one, so the honest path never reaches
// VerdictFirstUse: it exists for an endpoint typed in by hand, where the
// alternative is refusing to connect at all.
func DecideCert(der []byte, storedPin, pairedPin string) CertDecision {
	fingerprint := FingerprintSHA256(der)
	expected := storedPin
	if expected == "" {
		expected = pairedPin
	}
	if expected == "" {
		return CertDecision{VerdictFirstUse, fingerprint}
	}
	if FingerprintsMatch(expected, fingerprint) {
		return CertDecision{VerdictPinned, fingerprint}
	}
	return CertDecision{VerdictMismatch, fingerprint}
}

// PinnedTLSConfig builds the tls.Config a dial to endpoint should use.
//
// The platform's own validation runs first. A certificate that validates
// publicly is accepted without consulting the pin and without a decision
// callback, which is correct: it means the relay hostname got a real
// certificate. Only when that fails (which for catway's self-signed certificate
// is always) does the pin decide, which makes VerifyPeerCertificate the pin
// check's natural home rather than a replacement for real validation.
//
// InsecureSkipVerify is set so the handshake reaches our callback at all; the
// callback then does the verification the flag switched off, so the name is
// misleading about what the resulting config actually checks.
func PinnedTLSConfig(endpoint Endpoint, storedPin string, onDecision func(CertDecision)) *tls.Config {
	return &tls.Config{
		ServerName:         endpoint.Host,
		InsecureSkipVerify: true, //nolint:gosec // re-verified in VerifyPeerCertificate
		MinVersion:         tls.VersionTLS12,
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return errors.New("catsclient: server presented no certificate")
			}
			if publiclyValid(rawCerts, endpoint.Host) {
				return nil
			}
			decision := DecideCert(rawCerts[0], storedPin, endpoint.PinnedSHA256)
			if onDecision != nil {
				onDecision(decision)
			}
			if !decision.Accepted() {
				return ErrCertMismatch
			}
			return nil
		},
	}
}

// publiclyValid runs the chain through the system roots for host. Any failure
// (parse error, no roots, untrusted issuer, wrong name) means "not publicly
// valid" and hands the decision to the pin; it is never an error in itself.
func publiclyValid(rawCerts [][]byte, host string) bool {
	leaf, err := x509.ParseCertificate(rawCerts[0])
	if err != nil {
		return false
	}
	intermediates := x509.NewCertPool()
	for _, raw := range rawCerts[1:] {
		if c, err := x509.ParseCertificate(raw); err == nil {
			intermediates.AddCert(c)
		}
	}
	_, err = leaf.Verify(x509.VerifyOptions{DNSName: host, Intermediates: intermediates})
	return err == nil
}
