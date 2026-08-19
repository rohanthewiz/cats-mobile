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
                // Taken from the message, not carried over from the old item:
                // an agent whose model stopped resolving reports it absent, and
                // a stale model must not outlive the report it came from.
                model: m.model,
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

  // --- windows -----------------------------------------------------------
  //
  // A connection is a VIEW, not a mirror: each desktop window shows one
  // workspace, and a viewer follows the primary view — whichever desktop
  // window the user touched last. Two questions follow from that, and the
  // phone is the client that most needs both answered on screen:
  //
  //   "whose window am I looking through?"  -> [viewWorkspace], [followedWindow]
  //   "what else could I look through?"     -> [desktopWindows]
  //
  // Both are folds of what the server already sends. The `clients` census
  // carries one entry per connection with the workspace it RESOLVED to, and
  // the `layout` this connection receives is built for its own view — so the
  // active flag in it is the server's own answer to "what am I showing",
  // rather than something reconstructed here from a pin that may have gone
  // stale. Deriving it twice is how a client ends up disagreeing with the
  // server about what is on its own screen.

  /// The workspace this connection is being shown, per the server.
  ///
  /// Read off the layout's active flag rather than from any local pin: a pin
  /// naming a workspace that has since been closed falls back server-side, and
  /// a viewer with no pin at all resolves to a workspace it never named.
  /// Empty until the first layout arrives.
  String get viewWorkspace {
    for (final w in layout?.workspaces ?? const <WorkspaceInfo>[]) {
      if (w.active) return w.id;
    }
    return '';
  }

  /// The desktop windows currently connected, in census order.
  ///
  /// Viewers are left out: another phone is not something this one can look
  /// through. A window's [DesktopWindow.followed] is workspace equality, not
  /// connection identity — the census gives connections no ids, and two windows
  /// on one workspace mirror anyway, so "following that window" honestly means
  /// "showing what it shows".
  List<DesktopWindow> get desktopWindows {
    final names = {
      for (final w in layout?.workspaces ?? const <WorkspaceInfo>[])
        w.id: w.name,
    };
    final showing = viewWorkspace;
    return [
      for (final v in clients?.views ?? const <ClientView>[])
        if (!v.viewer)
          DesktopWindow(
            workspaceId: v.workspace,
            // Empty when the layout has not named it. That happens legitimately
            // — a census can arrive before the first layout — so a UI shows the
            // id rather than treating it as an error.
            workspaceName: names[v.workspace] ?? '',
            cols: v.cols,
            rows: v.rows,
            focused: v.focused,
            primary: v.primary,
            followed: v.workspace.isNotEmpty && v.workspace == showing,
          ),
    ];
  }

  /// The window this connection is currently looking through, or null when the
  /// census has not arrived or no desktop window shows this workspace (the
  /// desktop quit and left the session running, say).
  DesktopWindow? get followedWindow {
    for (final w in desktopWindows) {
      if (w.followed) return w;
    }
    return null;
  }

  /// The primary view — the desktop window every view-less caller and every
  /// unpinned viewer resolves through. Null before the first census.
  DesktopWindow? get primaryWindow {
    for (final w in desktopWindows) {
      if (w.primary) return w;
    }
    return null;
  }

  /// How many connections are viewers (phones, tablets), this one included.
  /// `clients.total - clients.sizers` says the same thing without needing
  /// [Clients.views]; this reads it off the views when they are there.
  int get viewerCount {
    final c = clients;
    if (c == null) return 0;
    if (c.views.isEmpty) return c.total - c.sizers;
    return c.views.where((v) => v.viewer).length;
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

/// One desktop window, as a phone sees it: the census entry joined to the name
/// the layout gives its workspace.
///
/// It is deliberately a window and not a workspace. A workspace with no window
/// on it is still running and still in the sidebar, but it is not something to
/// "follow" — following means seeing what somebody at the desk is seeing.
class DesktopWindow {
  const DesktopWindow({
    required this.workspaceId,
    required this.workspaceName,
    required this.cols,
    required this.rows,
    required this.focused,
    required this.primary,
    required this.followed,
  });

  /// The workspace this window is showing, resolved by the server (a window on
  /// a closed workspace reports the one it fell back to, not the stale id).
  final String workspaceId;

  /// The workspace's display name, or '' when no layout has named it yet.
  final String workspaceName;

  /// The window's grid in cells — what its panes are laid out against. Useful
  /// as a label ("200x60") when two windows sit on the same workspace and the
  /// name alone cannot tell them apart.
  final int cols;
  final int rows;

  /// Its OS window is in the foreground.
  final bool focused;

  /// It is the primary view: the most recently focused desktop window, which is
  /// what an unpinned viewer follows and what catctl acts on.
  final bool primary;

  /// This connection is currently showing this window's workspace.
  final bool followed;

  /// A label that stays useful before the first layout arrives.
  String get label => workspaceName.isNotEmpty ? workspaceName : workspaceId;
}
