import 'package:catsproto/catsproto.dart';
import 'package:test/test.dart';

AgentItem agent(
  String pub,
  String state, {
  bool seen = true,
  int sinceMs = 0,
  int pane = 0,
}) => AgentItem(
  pane: pane,
  pub: pub,
  workspace: 'w1',
  tab: 1,
  agent: 'claude',
  state: state,
  seen: seen,
  sinceMs: sinceMs,
);

void main() {
  group('roster ordering', () {
    test('what needs me beats where it lives', () {
      final session = CatsSession()
        ..apply(
          Agents(
            items: [
              agent('w3:p1', AgentState.idle),
              agent('w1:p2', AgentState.working),
              agent('w2:p9', AgentState.blocked),
              agent('w1:p5', AgentState.idle, seen: false),
            ],
          ),
        );
      expect(
        session.roster.map((a) => a.pub),
        ['w2:p9', 'w1:p2', 'w1:p5', 'w3:p1'],
        reason:
            'blocked → working → done-unseen → idle, not grouped by workspace',
      );
    });

    test('within a group, the longest wait comes first', () {
      final session = CatsSession()
        ..apply(
          Agents(
            items: [
              agent('w1:p1', AgentState.blocked, sinceMs: 5000),
              agent('w1:p2', AgentState.blocked, sinceMs: 900000),
            ],
          ),
        );
      expect(session.roster.first.pub, 'w1:p2');
    });

    test('a blocked agent stays first even when unseen', () {
      // "Finished while you were away" is a nudge; "blocked" is a request. The
      // request wins.
      final session = CatsSession()
        ..apply(
          Agents(
            items: [
              agent('w1:p1', AgentState.idle, seen: false),
              agent('w1:p2', AgentState.blocked, seen: false),
            ],
          ),
        );
      expect(session.roster.first.pub, 'w1:p2');
    });
  });

  group('fold', () {
    test('pane_agent patches the rollup without waiting for the next one', () {
      final session = CatsSession()
        ..apply(Agents(items: [agent('w1:p1', AgentState.working, pane: 1)]))
        ..apply(
          const PaneAgent(
            pane: 1,
            agent: 'claude',
            state: AgentState.blocked,
            seen: true,
          ),
        );
      expect(session.roster.single.state, AgentState.blocked);
      expect(session.paneAgents[1]!.state, AgentState.blocked);
    });

    test('the patch carries the model, and clears it when absent', () {
      // The rows name the model rather than the agent, so a patch that dropped
      // it would blank the label for the round trip until the next rollup —
      // the exact lag the patch exists to avoid.
      final session = CatsSession()
        ..apply(Agents(items: [agent('w1:p1', AgentState.idle, pane: 1)]))
        ..apply(
          const PaneAgent(
            pane: 1,
            agent: 'copilot',
            state: AgentState.working,
            model: 'gpt-5-mini · medium',
            seen: true,
          ),
        );
      expect(session.roster.single.model, 'gpt-5-mini · medium');

      // An agent whose model stops resolving reports it absent; the old one
      // must not survive as a stale label.
      session.apply(
        const PaneAgent(
          pane: 1,
          agent: 'copilot',
          state: AgentState.idle,
          seen: true,
        ),
      );
      expect(session.roster.single.model, '');
    });

    test('chrome lands in its own maps, keyed by pane', () {
      final session = CatsSession()
        ..apply(const PaneTitle(pane: 4, title: 'vim'))
        ..apply(const PaneCwd(pane: 4, cwd: '/Users/ro/projs/go/cats'))
        ..apply(const PaneModes(pane: 4, mouse: true, altScreen: true))
        ..apply(const PaneExited(pane: 4, code: 130));
      expect(session.titles[4], 'vim');
      expect(session.cwds[4], '/Users/ro/projs/go/cats');
      expect(session.modes[4]!.altScreen, isTrue);
      expect(session.exitCodes[4], 130);
    });

    test('a respawned pane stops being an exited one', () {
      // The death is remembered, never re-derived: a client that only ever adds
      // to exitCodes would keep drawing "exited (130)" over a live shell for the
      // rest of the connection, and a reconnect is the only thing that clears it.
      final session = CatsSession()
        ..apply(const PaneExited(pane: 4, code: 130))
        ..apply(const PaneRespawned(pane: 4));
      expect(session.exitCodes.containsKey(4), isFalse);
    });

    test('the recorder and the runs in flight are server-authoritative', () {
      final session = CatsSession();
      // Null, not idle: a server too old to send `record` sends nothing, and an
      // unlit indicator would be a claim this client cannot make.
      expect(session.record, isNull);
      expect(session.runbookRuns, isEmpty);

      session
        ..apply(const RecordMsg(recording: true, steps: 3))
        ..apply(
          const RunbookRuns(
            runs: [
              RunbookRun(name: 'deploy', source: 'control', step: 2, steps: 5),
            ],
          ),
        );
      expect(session.record!.recording, isTrue);
      expect(session.record!.steps, 3);
      expect(session.runbookRuns.single.name, 'deploy');
      expect(session.runbookRuns.single.step, 2);

      // Replaced wholesale — a finished run is absent from the next push rather
      // than marked done in it, so folding must not merge with what came before.
      session.apply(const RunbookRuns(runs: []));
      expect(session.runbookRuns, isEmpty);
    });

    test('notifications accumulate for the inbox', () {
      final session = CatsSession()
        ..apply(
          const Notify(
            kind: 'attention',
            message: 'claude is blocked',
            pub: 'w1:p3',
          ),
        )
        ..apply(
          const Notify(kind: 'finished', message: 'run complete', pub: 'w2:p1'),
        );
      expect(session.notifications.length, 2);
      expect(session.notifications.last.pub, 'w2:p1');
    });

    test('clients stays null until the server pushes a census', () {
      // Null means "unknown", not "nobody" — a server without Caps.clients
      // never sends one, and rendering that as "you are alone" would be a lie.
      final session = CatsSession();
      expect(session.clients, isNull);
      session.apply(const Clients(total: 2, sizers: 1, cols: 200, rows: 60));
      expect(session.clients!.sizers, 1);
    });
  });

  group('resetForNewSocket', () {
    test('discards every grid', () {
      // Not tidiness: diff indices are relative to the last full frame's width
      // and def_fg/def_bg are per connection, so a carried-over grid is
      // corruption. registerConn resyncs every visible pane, so nothing is lost.
      final session = CatsSession()
        ..apply(
          PaneFrame(
            pane: 1,
            w: 2,
            h: 1,
            cur: const Cursor(x: 0, y: 0, vis: false, shape: 0),
            defFg: 1,
            defBg: 2,
            cells: const [
              Cell(s: 'a'),
              Cell(s: 'b'),
            ],
          ),
        )
        ..apply(const Layout(workspaces: [], tabs: [], panes: [], borders: []));
      expect(session.grids, isNotEmpty);
      expect(session.layout, isNotNull);

      session.resetForNewSocket();
      expect(session.grids, isEmpty);
      expect(session.layout, isNull);
      expect(session.clients, isNull);
    });

    test('keeps the roster, which is not connection-scoped', () {
      final session = CatsSession()
        ..apply(Agents(items: [agent('w1:p1', AgentState.blocked)]))
        ..resetForNewSocket();
      expect(
        session.agents,
        hasLength(1),
        reason:
            'the rollup spans every workspace and survives a reconnect '
            'until the server sends a fresh one',
      );
    });
  });
}
