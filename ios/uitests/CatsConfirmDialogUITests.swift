import XCTest

// The Reveal confirmation dialog, on iOS, by eye.
//
// cats-mobile builds its own dialog rather than using a UIAlertController, so
// nothing but a look confirms it lands correctly on this platform: the Android
// walk has seen it, this side never had. The test drives the app to the dialog
// and attaches a screenshot; the assertions are there so a run that never got
// there fails loudly instead of attaching a picture of the wrong screen.
//
// Revealing a pane is the one desktop-moving action the app offers, and it sits
// behind this confirmation deliberately (see the package comment in
// app/register.go) -- which is exactly why it is worth looking at.
//
// Needs a paired, connected app with at least one agent in the roster; run
// CatsPairUITests first, or scripts/ios-walk.sh, which sequences both.
//
//   xcodebuild test -project GrMobApp.xcodeproj -scheme GrMobApp \
//     -destination 'platform=iOS Simulator,name=iPhone 17 Pro' \
//     -only-testing:GrMobUITests/CatsConfirmDialogUITests
final class CatsConfirmDialogUITests: XCTestCase {

    func testRevealConfirmDialogRendersOnIOS() throws {
        let app = XCUIApplication()
        app.launch()

        // Matched on the model line the roster renders rather than a fixed
        // index, so an empty or disconnected desk fails with a message that
        // says so instead of tapping whatever happens to be first.
        let row = app.staticTexts.matching(
            NSPredicate(format: "label CONTAINS[c] %@", "claude")).firstMatch
        XCTAssertTrue(row.waitForExistence(timeout: 20),
                      "no agent row -- is the app paired and connected to a desk with agents?")
        row.tap()

        // The header action is an eye glyph, so its accessibility label is the
        // only stable way to address it.
        let reveal = app.buttons["Reveal this pane at the desk"]
        XCTAssertTrue(reveal.waitForExistence(timeout: 10), "no Reveal action on the pane screen")
        reveal.tap()

        // The dialog's own copy, from app/pane.go.
        let title = app.staticTexts.matching(
            NSPredicate(format: "label CONTAINS[c] %@", "Reveal at the desk?")).firstMatch
        XCTAssertTrue(title.waitForExistence(timeout: 10), "the confirm dialog never appeared")

        let shot = XCTAttachment(screenshot: XCUIScreen.main.screenshot())
        shot.lifetime = .keepAlways
        shot.name = "reveal-confirm-dialog"
        add(shot)
    }
}
