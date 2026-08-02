import 'generated/wire.g.dart';
import 'grid.dart';

/// The client's fold of the down-message stream into state.
///
/// Every screen reads from here; nothing here knows about Flutter. Keeping the
/// fold in the protocol package is what lets a recorded transcript drive the
/// same state a live socket does, which is what makes widget tests and demo
/// mode honest rather than approximate.
///
/// # Roster ordering is a product decision, not a data one
///
/// The `agents` rollup is the one message that spans EVERY workspace — frames
/// stream only for visible panes, but agent chrome is global — so it, not the
/// layout, is the backbone of the home screen. It is grouped by state rather
/// than by workspace because on a phone *what needs me* beats *where it lives*.
class CatsSession {
  /// Every pane with an agent, across every workspace.
  List<AgentItem> agents = const [];

  /// The active workspace's structure. A viewer renders this; it never causes
  /// it — nothing in this class sends anything.
  Layout? layout;

  /// Per-pane grids, created lazily. Only panes the server actually streams
  /// frames for appear here.
  final Map<int, PaneGrid> grids = {};

  /// Per-pane chrome, keyed by pane id.
  final Map<int, String> titles = {};
  final Map<int, String> cwds = {};
  final Map<int, PaneAgent> paneAgents = {};
  final Map<int, PaneModes> modes = {};
  final Map<int, int> exitCodes = {};

  /// The connected-client census. Null until the server pushes one — which a
  /// server without [Caps.clients] never does, so null means "unknown", not
  /// "nobody".
  Clients? clients;

  Usage? usage;
  Theme? theme;
  String appTitle = '';

  /// Notification history, newest last. Backs the Inbox screen and the deep
  /// link an ntfy tap arrives on.
  final List<Notify> notifications = [];

  /// Non-fatal server errors, for a toast and a debug view.
  final List<ErrorMsg> errors = [];

  bool serverShutDown = false;
  UpdateReady? updateReady;

  /// Folds one decoded down-message. Unknown types never reach here — the
  /// generated [decodeDown] drops them, per the protocol's own rule.
  void apply(Object message) {
    switch (message) {
      case final Agents m:
        agents = m.items;
      case final Layout m:
        layout = m;
      case final PaneTitle m:
        titles[m.pane] = m.title;
      case final PaneCwd m:
        cwds[m.pane] = m.cwd;
      case final PaneAgent m:
        paneAgents[m.pane] = m;
        // Patch the rollup in place. The server re-sends the whole rollup on
        // any change too, but this arrives first and the roster should not lag
        // a round trip behind the pane it is describing.
        agents = [
          for (final a in agents)
            if (a.pane == m.pane)
              AgentItem(
                pane: a.pane,
                pub: a.pub,
                workspace: a.workspace,
                tab: a.tab,
                agent: m.agent,
                state: m.state,
                seen: m.seen,
                sinceMs: 0,
              )
            else
              a,
        ];
      case final PaneModes m:
        modes[m.pane] = m;
      case final PaneExited m:
        exitCodes[m.pane] = m.code;
      case final PaneFrame m:
        gridFor(m.pane).applyFrame(m);
      case final PaneDiff m:
        gridFor(m.pane).applyDiff(m);
      case final Clients m:
        clients = m;
      case final Usage m:
        usage = m;
      case final Theme m:
        theme = m;
      case final Title m:
        appTitle = m.title;
      case final Notify m:
        notifications.add(m);
      case final ErrorMsg m:
        errors.add(m);
      case Shutdown _:
        serverShutDown = true;
      case final UpdateReady m:
        updateReady = m;
      // Welcome and CmdResult are the connection's business, not the session's.
    }
  }

  PaneGrid gridFor(int pane) => grids.putIfAbsent(pane, () => PaneGrid(pane));

  /// Discards every grid. Call on a NEW socket, before the first message.
  ///
  /// This is not tidiness. Diff indices are relative to the last full frame's
  /// width, and `def_fg`/`def_bg` are per connection, so carrying a grid across
  /// a reconnect is corruption — cells patched at indices computed for another
  /// geometry, in colours resolved against another frame's defaults. catway's
  /// registerConn guarantees a full resync per visible pane on connect, so
  /// there is nothing to lose by dropping them.
  void resetForNewSocket() {
    grids.clear();
    layout = null;
    clients = null;
  }

  /// The roster, in the order the home screen shows it.
  ///
  /// Grouped by what needs attention, not by where it lives. Within a group,
  /// the longest-waiting first: an agent that has been blocked for twenty
  /// minutes outranks one that blocked ten seconds ago, and `since_ms` is
  /// exactly that number.
  List<AgentItem> get roster {
    const rank = {
      AgentState.blocked: 0,
      AgentState.working: 1,
      AgentState.idle: 3,
    };
    final sorted = [...agents];
    sorted.sort((a, b) {
      final ra = _groupRank(a, rank);
      final rb = _groupRank(b, rank);
      if (ra != rb) return ra.compareTo(rb);
      if (a.sinceMs != b.sinceMs) return b.sinceMs.compareTo(a.sinceMs);
      return a.pub.compareTo(b.pub);
    });
    return sorted;
  }

  static int _groupRank(AgentItem item, Map<String, int> rank) {
    // `seen: false` is "finished while you were away" and outranks idle — it is
    // the whole reason somebody picks up the phone. It matches how the web UI
    // renders the same flag as "Done".
    if (!item.seen && item.state != AgentState.blocked) return 2;
    return rank[item.state] ?? 4;
  }
}
