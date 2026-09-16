#!/usr/bin/env python3
"""Flip a pane's agent state on a walk desk, so the phone has news to react to.

The app's attention path (app/attention.go) turns an agent that *became* blocked
into a buzz and a notification, and an agent that stopped being blocked into a
cancelled one. Nothing in catctl drives that: an agent's state is reported by
the agent's own hooks, so a walk needs something standing in for one. This is
that stand-in -- one JSON line on catway's --hook-socket, method
pane.report_agent.

Tracked rather than written into a scratchpad per walk, for the same reason as
ios/uitests: a harness that is re-derived every session is one whose findings
cannot be re-checked.

# Two ways this fails silently, both learned the hard way

The source is minted fresh on every run. catway suppresses a (source, agent)
pair for good once pane.release_agent has seen it with no official session ref,
and a suppressed report still answers `ok` -- so reusing a source produces a
run that looks entirely successful and changes nothing.

The agent is "walkbot" and deliberately not "claude". States carrying the
reserved cats:claude source are ignored, and on a walk desk the pane usually
*is* a real claude, so borrowing the name means competing with it.

usage:
    scripts/hook.py <state> [pane_handle]

    scripts/hook.py blocked w1:p1     an agent that needs someone
    scripts/hook.py idle w1:p1        ... and stopped needing them

env:
    CM_HOOK_SOCKET   catway's --hook-socket (default /tmp/cm-hooks.sock)
"""
import json
import os
import socket
import sys
import time

state = sys.argv[1] if len(sys.argv) > 1 else "blocked"
pane = sys.argv[2] if len(sys.argv) > 2 else "w1:p1"
sock_path = os.environ.get("CM_HOOK_SOCKET", "/tmp/cm-hooks.sock")

now_ms = int(time.time() * 1000)
req = {
    "id": "walk-%d" % now_ms,
    "method": "pane.report_agent",
    "params": {
        "pane_id": pane,
        "source": "cats:walk-%d" % int(time.time()),
        "agent": "walkbot",
        "state": state,
        # Monotonic enough for a walk: catway keeps the highest seq it has seen
        # per source, and a fresh source starts the comparison over anyway.
        "seq": now_ms % 100000,
    },
}

if not os.path.exists(sock_path):
    sys.exit("no hook socket at %s -- is catway running with --hook-socket?" % sock_path)

s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
s.connect(sock_path)
s.sendall((json.dumps(req) + "\n").encode())
s.shutdown(socket.SHUT_WR)
print("sent   :", json.dumps(req["params"]))
print("reply  :", s.recv(4096).decode().strip())
s.close()
