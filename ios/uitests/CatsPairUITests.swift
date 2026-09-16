import XCTest

// Pairing on the simulator, through the pair screen's "Paste link" button.
//
// # Why a UI test and not a script
//
// The simulator offers no tap or text-entry CLI -- simctl has neither, and
// osascript is refused assistive access here -- so a UI test is the only way to
// touch this screen from a script. That is also why this file is tracked rather
// than written into a scratchpad per run, which is what previous walks did: a
// throwaway test is one nobody can re-run when the same question comes back.
// scripts/lib.sh copies it into the shell copy's UI-test target on extract.
//
// # The link comes from the pasteboard, not from a substitution
//
// The test never contains the link. `xcrun simctl pbcopy booted` puts a fresh
// one on the device pasteboard before the run, and the button under test is the
// one that reads it. That keeps this file compilable and committed, and it
// exercises the clipboard path rather than standing in for it.
//
// Run it (see scripts/ios-walk.sh, which does the whole sequence):
//   xcodebuild test -project GrMobApp.xcodeproj -scheme GrMobApp \
//     -destination 'platform=iOS Simulator,name=iPhone 17 Pro' \
//     -only-testing:GrMobUITests/CatsPairUITests
//
// -only-testing matters: the target also holds grmob's own demo tests, which
// are written against the tutorial app and would fail against this one.
final class CatsPairUITests: XCTestCase {

    func testPasteTheLinkAndPair() throws {
        let app = XCUIApplication()

        // # Never wait on this app to go idle
        //
        // XCUITest quiesces the app before each interaction, and a grmob app
        // never reports its run loop idle -- every step here logs "App event
        // loop idle notification not received". For an ordinary element tap
        // that is a warning and the tap proceeds. For `app.tap()`, which has to
        // resolve the Target Application element first, it is fatal: it retried
        // three times and failed the run after 8m44s with "process main thread
        // busy for 30.0s".
        //
        // So nothing here taps the application itself, and system alerts are
        // found by querying SpringBoard directly rather than through
        // addUIInterruptionMonitor -- a monitor only fires on the next
        // interaction with the app, and app.tap() was the nudge that used to
        // provide. The SpringBoard queries below are the proven path: the stale
        // alert dismissal is what cleared one that had survived a SpringBoard
        // terminate and an app relaunch.

        // A leftover system alert sits above every app and swallows the taps
        // below -- an "Open in …?" prompt from a deep link opened earlier is the
        // one that turns up here, and it survives both a SpringBoard terminate
        // and relaunching the app. An interruption monitor does not help: those
        // fire on an alert raised *during* the run, not one already on screen.
        // Clearing it here is what keeps a walk scriptable instead of needing a
        // hand on the simulator.
        let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
        for label in ["Cancel", "Close", "Dismiss", "OK"] {
            let button = springboard.buttons[label]
            if button.exists { button.tap(); break }
        }

        app.launch()

        let paste = app.buttons["Paste the pairing link"]
        XCTAssertTrue(paste.waitForExistence(timeout: 20),
                      "no Paste button on the pair screen -- is the app already paired?")
        paste.tap()

        // iOS can put an "Allow Paste" confirmation over a programmatic
        // pasteboard read. It belongs to SpringBoard, not the app, so it is
        // found there. waitForExistence rather than exists: the alert arrives a
        // moment after the tap, and a bare existence check races it. A short
        // timeout because the common case is no alert at all.
        for label in ["Allow Paste", "Paste", "Allow"] {
            let consent = springboard.buttons[label]
            if consent.waitForExistence(timeout: 3) { consent.tap(); break }
        }

        // Positional: comps.FormField renders its Label as a sibling Text node
        // rather than the input's accessibility label, so the pairing link
        // field is simply the first text field on the screen.
        let link = app.textFields.element(boundBy: 0)
        XCTAssertTrue(link.waitForExistence(timeout: 5), "no pairing-link field")

        // Polled, not read once. core.ReadClipboard takes a callback, so the
        // button's tap returns before the text arrives and the field fills a
        // render later -- reading `value` straight after the tap caught it
        // empty and failed a run in which the paste had in fact worked (the
        // same run went on to pair successfully). There is no "idle" to wait on
        // here either, for the reason in the comment at the top.
        var filled = ""
        // Named for its own wait: the pairing loop further down has a deadline
        // of its own, and this function is one scope.
        let pasteDeadline = Date().addingTimeInterval(10)
        while Date() < pasteDeadline {
            filled = (link.value as? String) ?? ""
            if filled.hasPrefix("cats://pair") { break }
            usleep(250_000)
        }
        XCTAssertTrue(filled.hasPrefix("cats://pair"),
                      "Paste did not fill the field within 10s; it holds: \(filled)")

        let pair = app.buttons["Pair"]
        XCTAssertTrue(pair.waitForExistence(timeout: 5), "no Pair button")
        pair.tap()

        // Pairing replaces the whole screen (root.go shows the pair screen only
        // until an endpoint is stored), so the header going away is the signal.
        // The fingerprint line, when it is still up, says which path got here.
        let pinned = app.staticTexts.matching(
            NSPredicate(format: "label CONTAINS[c] %@", "Pinned certificate")).firstMatch
        let header = app.staticTexts.matching(
            NSPredicate(format: "label CONTAINS[c] %@", "Pair with a desk")).firstMatch

        let deadline = Date().addingTimeInterval(30)
        var paired = false
        while Date() < deadline {
            if pinned.exists || !header.exists { paired = true; break }
            usleep(500_000)
        }

        let shot = XCTAttachment(screenshot: XCUIScreen.main.screenshot())
        shot.lifetime = .keepAlways
        shot.name = "pair-result"
        add(shot)

        if !paired {
            // A failure should say what the app said, not merely that a
            // predicate went unmet -- the screen's own error line is the thing
            // worth reading (pairErrorMessage in app/pair.go words it).
            let lines = app.staticTexts.allElementsBoundByIndex
                .prefix(30).map { $0.label }.joined(separator: " | ")
            XCTFail("still on the pair screen. On screen: \(lines)")
        }
    }
}
