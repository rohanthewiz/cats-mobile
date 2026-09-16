import XCTest

// Resume's probe-and-redial, on iOS.
//
// Backgrounding a phone kills the socket: iOS suspends the process outright, so
// the connection is gone within seconds and nothing on this side notices — a
// suspended app runs no code to notice with. Coming back is therefore not a
// matter of the socket resuming but of the app finding out it is dead and
// dialling again: core.OnLifecycle(.active) -> Connection.Resume -> a Ping probe
// -> redial on the backoff ladder. See Services.Bind and app/connection.go.
//
// The Android walk has watched this happen (about a minute on the ladder). This
// is the same walk on iOS, which no run had reached before: the two platforms
// sever the socket by different mechanisms — a suspended process here, a
// background firewall there — and only the second half, the redial, is shared.
//
// Needs an app that is already paired and connected; run CatsPairUITests first.
//
//   scripts/ios-walk.sh CatsResumeUITests
final class CatsResumeUITests: XCTestCase {

    // Anything the connection banner (root.go) can say. Its absence is the
    // signal for "connected": connectionBanner renders no node at all then.
    private let bannerPhrases = ["Connecting to", "Reconnecting", "Not connected",
                                 "session has expired", "certificate changed"]

    private func bannerText(_ app: XCUIApplication) -> String? {
        for phrase in bannerPhrases {
            let el = app.staticTexts.matching(
                NSPredicate(format: "label CONTAINS[c] %@", phrase)).firstMatch
            if el.exists { return el.label }
        }
        return nil
    }

    /// Waits for the banner to clear, which is this app's way of saying the
    /// socket is up. Returns whether it cleared inside the deadline.
    private func waitForConnected(_ app: XCUIApplication, timeout: TimeInterval) -> Bool {
        let deadline = Date().addingTimeInterval(timeout)
        while Date() < deadline {
            if bannerText(app) == nil { return true }
            usleep(500_000)
        }
        return false
    }

    func testForegroundingRedialsTheDesk() throws {
        let app = XCUIApplication()
        app.launch()

        // The roster is the proof the app is paired and folded a rollup; a run
        // against an unpaired app should say that rather than time out later.
        let row = app.staticTexts.matching(
            NSPredicate(format: "label CONTAINS[c] %@", "claude")).firstMatch
        XCTAssertTrue(row.waitForExistence(timeout: 20),
                      "no agent row -- is the app paired and connected? Run CatsPairUITests first.")
        XCTAssertTrue(waitForConnected(app, timeout: 30),
                      "the app never reached a connected state to begin with: \(bannerText(app) ?? "")")

        // Out of the foreground. iOS suspends shortly after, and the socket goes
        // with it; 20s is comfortably past that without being a long test.
        XCUIDevice.shared.press(.home)
        sleep(20)

        app.activate()

        // The redial is a backoff ladder, not an instant reconnect -- the
        // Android walk measured about a minute -- so this waits generously. What
        // is being asserted is that it comes back on its own, with no tap.
        let reconnected = waitForConnected(app, timeout: 120)

        let shot = XCTAttachment(screenshot: XCUIScreen.main.screenshot())
        shot.lifetime = .keepAlways
        shot.name = reconnected ? "resumed" : "still-disconnected"
        add(shot)

        XCTAssertTrue(reconnected,
                      "the app did not redial after foregrounding; the banner still says: \(bannerText(app) ?? "(none)")")
        XCTAssertTrue(row.waitForExistence(timeout: 20),
                      "reconnected, but the roster never came back")
    }
}
