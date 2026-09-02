package catsclient

import (
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"
)

// fakeSocket records what was sent and lets a test push messages back.
//
// This is the same seam a transcript replayer will use to feed recorded JSONL,
// so the code under test is the real fold, not a stand-in for it.
type fakeSocket struct {
	in   chan string
	once sync.Once

	mu   sync.Mutex
	sent []map[string]any
}

func newFakeSocket() *fakeSocket {
	return &fakeSocket{in: make(chan string, 64)}
}

func (s *fakeSocket) Recv() (string, error) {
	text, ok := <-s.in
	if !ok {
		return "", io.EOF
	}
	return text, nil
}

func (s *fakeSocket) Send(text string) error {
	var m map[string]any
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		return err
	}
	s.mu.Lock()
	s.sent = append(s.sent, m)
	s.mu.Unlock()
	return nil
}

// Close unblocks Recv with io.EOF. Idempotent, because Conn.Close closes the
// socket and a test may also drop() it.
func (s *fakeSocket) Close() error {
	s.once.Do(func() { close(s.in) })
	return nil
}

// drop simulates the network going away under the connection.
func (s *fakeSocket) drop() { _ = s.Close() }

func (s *fakeSocket) deliver(m map[string]any) {
	raw, _ := json.Marshal(m)
	s.in <- string(raw)
}

func (s *fakeSocket) deliverRaw(text string) { s.in <- text }

func (s *fakeSocket) sentCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sent)
}

func (s *fakeSocket) sentAt(i int) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sent[i]
}

// last is the most recent message sent.
func (s *fakeSocket) last() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sent[len(s.sent)-1]
}

// waitSent blocks until at least n messages have been sent. Invoke sends on
// the caller's goroutine before blocking on the reply, so a test that calls it
// on a goroutine needs to wait for the send to land before reading it.
func (s *fakeSocket) waitSent(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for s.sentCount() < n {
		if time.Now().After(deadline) {
			t.Fatalf("waited for %d sent messages, have %d", n, s.sentCount())
		}
		time.Sleep(time.Millisecond)
	}
}

var testEndpoint = Endpoint{ID: "test", Host: "127.0.0.1", Port: 8443}

// welcomeWith is a welcome message advertising the given caps.
func welcomeWith(caps ...string) map[string]any {
	c := make([]any, len(caps))
	for i, s := range caps {
		c[i] = s
	}
	return map[string]any{"t": "welcome", "v": 1, "caps": c}
}
