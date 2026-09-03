package catsclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rohanthewiz/cats/wire"
)

// Socket is the transport a Conn drives.
//
// Abstracted so a test can replay a recorded transcript through the exact same
// fold the live client uses. Every screen test, every snapshot and demo mode
// run on the same code path as the real thing; the alternative is a mock that
// agrees with the client and disagrees with the server. Dial returns the real
// one.
type Socket interface {
	// Recv blocks until the next text frame arrives. It returns an error once
	// the socket is closed or has failed; Conn stops reading after that.
	Recv() (string, error)
	// Send writes one text frame. Safe to call from any goroutine.
	Send(text string) error
	// Close shuts the socket and unblocks a pending Recv.
	Close() error
}

// Pinger is the optional half of Socket a transport implements when it can
// ask the far side whether it is still there. The native WebSocket does; the
// browser one cannot (the page's network stack answers pings itself and
// exposes no way to send one). Conn.Probe uses it when present and reports
// nothing otherwise, so a caller never has to know which transport it has.
type Pinger interface {
	// Ping round-trips a control frame and returns once the peer answered,
	// or with an error when the socket is dead or ctx expires first.
	Ping(ctx context.Context) error
}

// CommandError is a command that came back ok: false.
type CommandError struct {
	Command string
	Message string
}

func (e *CommandError) Error() string {
	return fmt.Sprintf("cats command %s: %s", e.Command, e.Message)
}

// DisconnectedError is a command that was still outstanding at disconnect, or
// a send attempted after one. Distinct from a timeout so the UI can say
// "reconnecting" instead of "the server is slow". Cause is the socket's own
// error when there was one.
type DisconnectedError struct {
	Command string
	Cause   error
}

func (e *DisconnectedError) Error() string {
	return fmt.Sprintf("cats %s: the socket closed first", e.Command)
}

func (e *DisconnectedError) Unwrap() error { return e.Cause }

// TimeoutError is a command that did not answer within its limit. It matches
// errors.Is(err, context.DeadlineExceeded) so callers that already handle
// deadlines need no second branch.
type TimeoutError struct {
	Command string
	Limit   time.Duration
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("cats %s did not answer within %s", e.Command, e.Limit)
}

func (e *TimeoutError) Is(target error) bool { return target == context.DeadlineExceeded }

// ViewerModeViolation is returned by Conn.Send for a message a viewer must
// never emit, and by FollowWorkspace on a server where the command would move
// every window at the desk.
type ViewerModeViolation struct {
	What string
}

func (e *ViewerModeViolation) Error() string {
	return "viewer mode: a cats client never sends " + e.What +
		"; it would reshape the desktop it is looking at"
}

// VersionMismatchError is a welcome whose protocol version is not ours. The
// server normally refuses the handshake itself (a CommandError on "init"),
// but a server that answers with a different version and no error is still
// one this client cannot read, and the UI's word for it is "app update
// required".
type VersionMismatchError struct {
	Server int
	Client int
}

func (e *VersionMismatchError) Error() string {
	return fmt.Sprintf("cats protocol version %d, this app speaks %d", e.Server, e.Client)
}

// Options configures a Conn. The zero value is usable.
type Options struct {
	// Endpoint is recorded for the UI; the socket has already been dialled.
	Endpoint Endpoint
	// Workspace pins the connection to one desktop window's workspace from
	// the first message; "" follows the primary view. See PinnedWorkspace.
	Workspace string
	// DefaultTimeout bounds an ordinary command (30 s when zero). MaxTimeout
	// bounds pane.wait_for_output (10 min when zero): app.MaxWaitTimeout is ten
	// minutes, and a client deadline shorter than that would abandon a wait
	// the server is still faithfully serving.
	DefaultTimeout time.Duration
	MaxTimeout     time.Duration
	// OnMessage receives every decoded down-message, in arrival order, on the
	// reader goroutine. Unknown types never appear: wire.DecodeDown reports
	// them and the protocol's rule is to drop them. Welcome and CmdResult are
	// delivered too, after the connection has acted on them, so a debug view
	// can see them. Must not call Close.
	OnMessage func(msg any)
}

// Conn is one live session with one catway.
//
// # The phone never resizes the desktop
//
// catway's registerConn takes the session grid from the FIRST init that
// declares one, and every connection shares that grid. A phone honestly
// reporting 40×20 reflows every pane for everybody. Four independent layers
// stop that (see the package doc), and this type is two of them:
//
//  1. handshake builds the Init itself, with hardcoded zeros and Viewer set.
//     App code has no way to supply one, except the workspace it wants to look
//     through, which says nothing about size.
//  2. Send refuses Resize outright.
//
// FollowWorkspace adds a third of the same kind for the same reason: it
// refuses to send workspace.focus unless the server advertises wire.CapWindow.
// With that capability the command moves only this connection's view; without
// it, it is the old session-wide switch and would drag every window at the
// desk along.
//
// # Goroutines
//
// One reader goroutine, started by New, owns the socket's receive side. It
// decodes each frame, settles the pending command it answers (if any), and
// hands the message to OnMessage. Close stops it and waits for it to exit, so
// Close must not be called from OnMessage.
type Conn struct {
	endpoint       Endpoint
	socket         Socket
	defaultTimeout time.Duration
	maxTimeout     time.Duration
	onMessage      func(any)

	mu              sync.Mutex
	pending         map[string]*pending
	nextID          int
	closed          bool
	closeErr        error
	caps            map[string]bool
	pinnedWorkspace string
	welcome         *wire.Welcome
	welcomeErr      error
	welcomeDone     bool

	// welcomeCh is closed exactly once, when the welcome is decided either
	// way. Waiters select on it rather than polling.
	welcomeCh  chan struct{}
	readerDone chan struct{}
}

// pending is one command awaiting its reply. Exactly one of onResult, expire
// and fail delivers to ch, because each removes the entry from the map under
// the lock before sending; ch is buffered so that send never blocks.
type pending struct {
	name  string
	ch    chan result
	timer *time.Timer
}

type result struct {
	data json.RawMessage
	err  error
}

// New takes ownership of socket, sends the handshake and starts reading.
func New(socket Socket, opts Options) *Conn {
	c := &Conn{
		endpoint:        opts.Endpoint,
		socket:          socket,
		defaultTimeout:  opts.DefaultTimeout,
		maxTimeout:      opts.MaxTimeout,
		onMessage:       opts.OnMessage,
		pending:         map[string]*pending{},
		caps:            map[string]bool{},
		pinnedWorkspace: opts.Workspace,
		welcomeCh:       make(chan struct{}),
		readerDone:      make(chan struct{}),
	}
	if c.defaultTimeout == 0 {
		c.defaultTimeout = 30 * time.Second
	}
	if c.maxTimeout == 0 {
		c.maxTimeout = 10 * time.Minute
	}
	c.handshake()
	go c.read()
	return c
}

// Endpoint is the address this connection was dialled to.
func (c *Conn) Endpoint() Endpoint { return c.endpoint }

// handshake sends the one Init this connection will ever send.
//
// The zeros are the point. catway's registerConn skips the area assignment
// when cols/rows are 0, and skips the cell metrics when those are 0: two
// separate guards, both of which a viewer must clear. Sending Viewer as well is
// belt and braces against a server that honours one and forgets the other.
//
// Workspace is the one field the app gets to fill in, and it is not a hole in
// the rule above: a workspace is which window this connection looks THROUGH,
// not a claim about its size. Empty is "follow the primary view", which is
// what a phone that has not picked a window wants. It rides the handshake
// rather than a command afterwards so a reconnect comes back on the same
// window instead of flicking to the desk's for a frame; FollowWorkspace is
// the live half.
func (c *Conn) handshake() {
	// No T: wire.Marshal stamps "t" from the Go type.
	init := wire.Init{
		V:         wire.ProtocolVersion,
		Cols:      0,
		Rows:      0,
		DPR:       0,
		CellWPx:   0,
		CellHPx:   0,
		Viewer:    true,
		Workspace: c.pinnedWorkspace,
	}
	raw, err := wire.Marshal(init)
	if err != nil {
		// A fixed struct of scalars cannot fail to marshal; if it somehow does,
		// the session is unusable and every caller should learn that at once.
		c.fail(&DisconnectedError{Command: "init", Cause: err})
		return
	}
	if err := c.socket.Send(string(raw)); err != nil {
		c.fail(&DisconnectedError{Command: "init", Cause: err})
	}
}

// read is the reader goroutine.
func (c *Conn) read() {
	defer close(c.readerDone)
	for {
		text, err := c.socket.Recv()
		if err != nil {
			c.fail(&DisconnectedError{Command: "<socket closed>", Cause: err})
			return
		}
		c.onText(text)
	}
}

func (c *Conn) onText(text string) {
	msg, err := wire.DecodeDown([]byte(text))
	if err != nil {
		// Malformed JSON: drop the message, keep the socket. Unknown "t": the
		// protocol says ignore it. Both come back as errors from DecodeDown and
		// neither is a reason to drop a session and every pane with it.
		return
	}
	switch m := msg.(type) {
	case *wire.Welcome:
		c.onWelcome(m)
	case *wire.CmdResult:
		c.onResult(m)
	}
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if !closed && c.onMessage != nil {
		c.onMessage(msg)
	}
}

func (c *Conn) onWelcome(m *wire.Welcome) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.caps = make(map[string]bool, len(m.Caps))
	for _, cap := range m.Caps {
		c.caps[cap] = true
	}
	switch {
	case m.Error != "":
		c.decideWelcomeLocked(nil, &CommandError{Command: "init", Message: m.Error})
	case m.V != wire.ProtocolVersion:
		c.decideWelcomeLocked(nil, &VersionMismatchError{Server: m.V, Client: wire.ProtocolVersion})
	default:
		c.decideWelcomeLocked(m, nil)
	}
}

// decideWelcomeLocked settles the welcome once; later calls are ignored.
func (c *Conn) decideWelcomeLocked(w *wire.Welcome, err error) {
	if c.welcomeDone {
		return
	}
	c.welcomeDone = true
	c.welcome, c.welcomeErr = w, err
	close(c.welcomeCh)
}

func (c *Conn) onResult(m *wire.CmdResult) {
	c.mu.Lock()
	p := c.pending[m.ID]
	delete(c.pending, m.ID)
	c.mu.Unlock()
	if p == nil {
		// Not an error: it is the reply to a call that already timed out.
		// It still reaches OnMessage, where a debug view can see it.
		return
	}
	p.timer.Stop()
	if m.Ok {
		p.ch <- result{data: m.Data}
	} else {
		p.ch <- result{err: &CommandError{Command: p.name, Message: m.Error}}
	}
}

// Welcome blocks until the server's welcome arrives, the server rejects the
// handshake (a protocol-version mismatch), the socket dies first, or ctx ends.
func (c *Conn) Welcome(ctx context.Context) (*wire.Welcome, error) {
	select {
	case <-c.welcomeCh:
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.welcome, c.welcomeErr
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// HasCap reports whether the server advertised a capability. False before the
// welcome, and false on a server old enough to send no caps at all, which is a
// real answer rather than "unknown": such a server honours none of them.
func (c *Conn) HasCap(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.caps[name]
}

// Caps is a copy of the advertised capability set.
func (c *Conn) Caps() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.caps))
	for name := range c.caps {
		out = append(out, name)
	}
	return out
}

// IsClosed reports whether the socket has failed or been closed.
func (c *Conn) IsClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

// --- which window this connection looks through ------------------------------
//
// A connection is a view. A desktop window pins its workspace with ?ws=; this
// client pins it in the handshake and can move it afterwards. The state lives
// here rather than in Session because it is a fact about the SOCKET: it dies
// with it, and the app hands it to the next Conn so a reconnect lands on the
// same window.
//
// What the server is showing is a different question, and the session answers
// that one (Session.ViewWorkspace) from the layout it actually received. A pin
// can be stale (the workspace it named can be closed from the desk) and the
// server falls back silently when it is, so a UI that reports the pin as "what
// you are watching" would be lying at exactly the wrong moment.

// PinnedWorkspace is the workspace this connection asked to be pinned to, or
// "" when it is following the primary view (whichever desktop window was
// touched last).
func (c *Conn) PinnedWorkspace() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pinnedWorkspace
}

// FollowsPrimaryView reports whether this connection is following the primary
// view rather than holding one window.
func (c *Conn) FollowsPrimaryView() bool { return c.PinnedWorkspace() == "" }

// FollowWorkspace pins this connection to one desktop window's workspace.
//
// This is a viewer-safe command, and the capability check is what makes it
// one. On a server advertising wire.CapWindow, workspace.focus moves ONLY the
// connection that sent it: nothing at the desk changes, which is why picking a
// window to watch does not need the "explicit confirmed gesture" that
// agent.focus or pane.zoom does. On a server WITHOUT that capability the same
// command is a session mutation and would switch every window at the desk, so
// it is refused here rather than sent and hoped about.
//
// Waits for the welcome first: the capability set is not known before it, and
// an empty set is a real answer ("honours none") that must not be read as a
// refusal on a server that simply has not replied yet.
func (c *Conn) FollowWorkspace(ctx context.Context, workspaceID string) error {
	if err := c.requireWindowCap(ctx, "workspace.focus"); err != nil {
		return err
	}
	if _, err := c.Invoke(ctx, wire.CmdWorkspaceFocus, wire.WorkspaceParams{ID: workspaceID}); err != nil {
		return err
	}
	// Only after the server said ok: a pin the server rejected (an id closed
	// between the census and the tap) must not survive into the next
	// handshake, where it would be silently fallen back a second time.
	c.mu.Lock()
	c.pinnedWorkspace = workspaceID
	c.mu.Unlock()
	return nil
}

// FollowPrimaryView releases the pin: this connection follows the primary
// view again. The return leg of FollowWorkspace: an empty id is "follow the
// primary", the state Init.Workspace leaves a connection in by being absent. A
// server too old to know it answers ok: false and the pin stands, which is a
// detectable no rather than a silent one.
func (c *Conn) FollowPrimaryView(ctx context.Context) error {
	return c.FollowWorkspace(ctx, "")
}

func (c *Conn) requireWindowCap(ctx context.Context, what string) error {
	if _, err := c.Welcome(ctx); err != nil {
		return err
	}
	if !c.HasCap(wire.CapWindow) {
		// Phrased to complete ViewerModeViolation's own sentence, which ends
		// "; it would reshape the desktop it is looking at".
		return &ViewerModeViolation{What: fmt.Sprintf(
			"%s to a server that does not advertise %q, where it is a session-wide switch",
			what, wire.CapWindow)}
	}
	return nil
}

// Send sends one up-message: a wire.Key, Mouse, Paste, Image or Cmd, by value
// or by pointer. The "t" discriminator is wire.Marshal's job (it stamps from
// the Go type), so a caller cannot send a message the server would drop as
// untyped, and this method has nothing to fill in.
//
// Refuses Resize regardless of what the caller intended (see the type doc),
// and a second Init, because the handshake is this type's to build. Focus and
// Raw are refused as well, for a quieter reason: the server folds every
// connection's focus report into one "is anyone looking" bit that decides
// whether a TUI at the desk parks its caret, and a phone in the foreground has
// no business un-parking it; Raw is the deprecated pre-encoded escape hatch,
// and a viewer never pre-encodes.
func (c *Conn) Send(msg any) error {
	if c.IsClosed() {
		return &DisconnectedError{Command: "<send>"}
	}
	if err := allowUp(msg); err != nil {
		return err
	}
	raw, err := wire.Marshal(msg)
	if err != nil {
		return err
	}
	return c.socket.Send(string(raw))
}

// allowUp is the viewer allowlist: nil for a message a viewer may send, a
// ViewerModeViolation for the ones the type doc forbids, and a plain error
// for anything that is not an up-message at all.
func allowUp(msg any) error {
	switch msg.(type) {
	case *wire.Resize, wire.Resize:
		// Naming the type here is the ONLY use of Resize in this module, and it
		// exists to refuse it. viewer_mode_test.go looks for composite
		// literals, which this is not.
		return &ViewerModeViolation{What: "resize"}
	case *wire.Init, wire.Init:
		return &ViewerModeViolation{What: "a second init (the handshake is ours to build)"}
	case *wire.Key, wire.Key,
		*wire.Mouse, wire.Mouse,
		*wire.Paste, wire.Paste,
		*wire.Image, wire.Image,
		*wire.Cmd, wire.Cmd:
		return nil
	}
	return fmt.Errorf("catsclient: not an up-message a viewer sends: %T", msg)
}

// Invoke sends one command and blocks for its reply, returning the reply's
// raw data. Call is the typed wrapper.
//
// Every call is bounded. A wait that resolves only on a server event is the
// one case where a long deadline is correct rather than lazy, but even that
// one has a deadline, because a leaked waiter is a spinner that never stops.
// ctx ending first abandons the call; a reply that then arrives is dropped.
func (c *Conn) Invoke(ctx context.Context, name string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, &DisconnectedError{Command: name}
	}
	id := fmt.Sprintf("c%d", c.nextID)
	c.nextID++
	p := &pending{name: name, ch: make(chan result, 1)}
	c.pending[id] = p
	limit := c.defaultTimeout
	if name == wire.CmdWaitForOutput {
		limit = c.maxTimeout
	}
	p.timer = time.AfterFunc(limit, func() { c.expire(id, p, limit) })
	c.mu.Unlock()

	cmd, err := wire.NewCmd(id, name, params)
	if err == nil {
		var raw []byte
		if raw, err = wire.Marshal(cmd); err == nil {
			err = c.socket.Send(string(raw))
		}
	}
	if err != nil {
		c.forget(id, p)
		return nil, err
	}

	select {
	case r := <-p.ch:
		return r.data, r.err
	case <-ctx.Done():
		c.forget(id, p)
		return nil, ctx.Err()
	}
}

// forget withdraws a call that will never be answered by the socket. It is a
// no-op if the reply already landed, in which case the buffered result is
// simply never read.
func (c *Conn) forget(id string, p *pending) {
	c.mu.Lock()
	if c.pending[id] == p {
		delete(c.pending, id)
	}
	c.mu.Unlock()
	p.timer.Stop()
}

func (c *Conn) expire(id string, p *pending, limit time.Duration) {
	c.mu.Lock()
	live := c.pending[id] == p
	if live {
		delete(c.pending, id)
	}
	c.mu.Unlock()
	if live {
		p.ch <- result{err: &TimeoutError{Command: p.name, Limit: limit}}
	}
}

// fail closes the connection and fails every outstanding call. Called on
// socket error and on Close; idempotent.
//
// Every pending call must fail, without exception. The alternative is a call
// nobody ever answers, which on a phone is a spinner that spins until the app
// is killed.
func (c *Conn) fail(err error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.closeErr = err
	pend := c.pending
	c.pending = map[string]*pending{}
	c.decideWelcomeLocked(nil, err)
	c.mu.Unlock()

	var disc *DisconnectedError
	isDisconnect := errors.As(err, &disc)
	for _, p := range pend {
		p.timer.Stop()
		e := err
		if isDisconnect {
			// Name the command that was abandoned, not the socket event.
			e = &DisconnectedError{Command: p.name, Cause: disc.Cause}
		}
		p.ch <- result{err: e}
	}
}

// Done is closed when the reader goroutine has exited: the socket failed,
// or Close was called. It is the signal a reconnect loop waits on, and it is
// a channel rather than a callback so the loop can select on it against its
// own cancellation. Err says why.
func (c *Conn) Done() <-chan struct{} { return c.readerDone }

// Err is why the connection closed, or nil while it is open. A socket that
// died underneath is a DisconnectedError whose Cause is the transport's
// error; a deliberate Close is a DisconnectedError with no cause.
func (c *Conn) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeErr
}

// Probe asks whether the socket is still alive and, when it is not, fails
// the connection so Done fires and a reconnect loop redials at once.
//
// It exists for the foreground transition. A phone that spent an hour in
// the background usually comes back with a socket the OS quietly severed:
// nothing has failed yet from this side, the keep-alive will notice within
// its interval, and the first user action would notice sooner — as a
// spinner. Probing on resume moves that discovery to the moment the screen
// lights up. The ping is bounded by ctx; a socket that cannot answer within
// it is treated as dead, which is the right call for a probe whose whole
// point is speed (a healthy socket answers a ping in one round trip).
//
// A transport without Pinger — the browser's — reports nil, because the
// page's own stack keeps the socket honest and there is nothing to add. A
// connection already closed reports its Err.
func (c *Conn) Probe(ctx context.Context) error {
	c.mu.Lock()
	closed, closeErr := c.closed, c.closeErr
	c.mu.Unlock()
	if closed {
		return closeErr
	}
	p, ok := c.socket.(Pinger)
	if !ok {
		return nil
	}
	if err := p.Ping(ctx); err != nil {
		// Close rather than fail alone: the reader is blocked in Recv on a
		// socket that will never deliver, and closing the socket is what
		// unblocks it. Close is idempotent with a reader that has since
		// exited on its own.
		_ = c.Close()
		return &DisconnectedError{Command: "<probe>", Cause: err}
	}
	return nil
}

// Close fails every pending call, closes the socket and waits for the reader
// to exit. Must not be called from OnMessage, which runs on that reader.
func (c *Conn) Close() error {
	c.fail(&DisconnectedError{Command: "<close>"})
	err := c.socket.Close()
	<-c.readerDone
	return err
}
