package catsclient

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rohanthewiz/cats/wire"
)

// Typed command helpers over Conn.Invoke.
//
// The Dart package generated one method per command from cats's command table
// (catgen-dart). cats does not yet emit the Go equivalent (a catgen-go is
// planned in the move-to-grmob plan, phase 1 step 3), so until it does these
// are written by hand and limited to what the phone actually calls. Each is
// one line over Call, so adding a command as a screen needs it is cheap, and
// the names follow the Dart methods so the two are recognisable side by side.

// Call sends a command and decodes its reply into R. A reply with no data
// leaves R at its zero value, which is what a command with no result type
// (workspace.focus, say) answers with.
func Call[R any](ctx context.Context, c *Conn, name string, params any) (R, error) {
	var out R
	raw, err := c.Invoke(ctx, name, params)
	if err != nil {
		return out, err
	}
	if len(raw) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("catsclient: %s: decode reply: %w", name, err)
	}
	return out, nil
}

// Capture extracts a pane's buffer text.
func (c *Conn) Capture(ctx context.Context, p wire.CaptureParams) (wire.CaptureResult, error) {
	return Call[wire.CaptureResult](ctx, c, wire.CmdCapture, p)
}

// PaneSendInput injects text into a pane as though typed, optionally followed
// by Enter. The composer's "reply" verb.
func (c *Conn) PaneSendInput(ctx context.Context, p wire.SendInputParams) error {
	_, err := c.Invoke(ctx, wire.CmdPaneSendInput, p)
	return err
}

// PaneWaitForOutput blocks until a pane's output matches, or the server's
// timeout elapses. Bounded by Options.MaxTimeout rather than DefaultTimeout.
func (c *Conn) PaneWaitForOutput(ctx context.Context, p wire.WaitForOutputParams) (wire.WaitForOutputResult, error) {
	return Call[wire.WaitForOutputResult](ctx, c, wire.CmdWaitForOutput, p)
}

// PaneList enumerates every pane in the session.
func (c *Conn) PaneList(ctx context.Context) (wire.PaneListResult, error) {
	return Call[wire.PaneListResult](ctx, c, wire.CmdPaneList, nil)
}

// WorkspaceFocus is the raw command. Prefer FollowWorkspace, which checks the
// capability that makes it viewer-safe and keeps the pin in step.
func (c *Conn) WorkspaceFocus(ctx context.Context, p wire.WorkspaceParams) error {
	_, err := c.Invoke(ctx, wire.CmdWorkspaceFocus, p)
	return err
}
