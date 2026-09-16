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
// # What counts as proof, and what does not
//
// An earlier version asked "is the connection banner absent?" and called that
// connected. It is not: absence of a banner is also what a backgrounded app,
// a transition, or a query against the wrong element type looks like. That
// version reported a clean redial while its own screenshot showed the home
// screen.
//
// So the signal here is positive and specific — the roster's agent row, which
// only exists once the app is connected AND has folded a rollup. The banner is
// read too, but only to make a failure legible.
//
// Needs an app that is already paired and connected; run CatsPairUITests first.
//
//   scripts/ios-walk.sh CatsResumeUITests
final class CatsResumeUITests: XCTestCase {

    // buttons, not staticTexts: the roster row's text lives inside a Box with
    // core.OnClick, so UIKit surfaces it as a Button. Plain Text nodes stay
    // StaticText, which is why the banner below is queried the other way.
    private func rosterRow(_ app: XCUIApplication) -> XCUIElement {
        app.buttons.matching(
            NSPredicate(format: "label CONTAINS[c] %@", "claude")).firstMatch
    }

    // Anything the connection banner (root.go) can say. Diagnostic only — never
    // a pass condition, for the reason in the file comment.
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

    func testForegroundingRedialsTheDesk() throws {
        let app = XCUIApplication()
        app.launch()

        let row = rosterRow(app)
        XCTAssertTrue(row.waitForExistence(timeout: 30),
                      "no agent row to begin with -- is the app paired and connected? "
                      + "Run CatsPairUITests first. Banner says: \(bannerText(app) ?? "(none)")")

        // Out of the foreground. iOS suspends shortly after, and the socket goes
        // with it; 20s is comfortably past that without being a long test.
        XCUIDevice.shared.press(.home)
        sleep(20)

        app.activate()

        // The app has to be genuinely back in front before anything on screen
        // means anything -- this is what the previous version skipped, and why
        // it screenshotted the home screen and called it a success.
        XCTAssertTrue(app.wait(for: .runningForeground, timeout: 30),
                      "the app never came back to the foreground after activate()")

        // The redial is a backoff ladder, not an instant reconnect -- the
        // Android walk measured about a minute -- so this waits generously. What
        // is asserted is that the roster returns on its own, with no tap.
        let start = Date()
        let back = row.waitForExistence(timeout: 150)
        let elapsed = Date().timeIntervalSince(start)

        let shot = XCTAttachment(screenshot: XCUIScreen.main.screenshot())
        shot.lifetime = .keepAlways
        shot.name = back ? "resumed" : "still-disconnected"
        add(shot)

        XCTAssertTrue(back,
                      "the app did not redial after foregrounding; "
                      + "the banner says: \(bannerText(app) ?? "(none)")")
        print("==> roster returned \(String(format: "%.1f", elapsed))s after foregrounding")
    }
}
