import XCTest

final class PhoneFlowTests: XCTestCase {
    private func capture(_ app: XCUIApplication, _ name: String) {
        let shot = XCTAttachment(screenshot: app.screenshot()); shot.name = name; shot.lifetime = .keepAlways; add(shot)
    }
    func testNativeNavigationReviewAndDraft() {
        let app = XCUIApplication(); app.launchArguments = ["--demo", "--ui-testing"]; app.launch()
        XCTAssertTrue(app.tabBars.buttons["主机"].waitForExistence(timeout: 15))
        app.buttons.matching(identifier: "pending-command").firstMatch.tap()
        XCTAssertTrue(app.buttons["review-command"].waitForExistence(timeout: 5))
        capture(app, "01-AI对话"); app.buttons["review-command"].tap()
        XCTAssertTrue(app.staticTexts["complete-command"].waitForExistence(timeout: 5))
        XCTAssertFalse(app.buttons["approve-command"].isEnabled); capture(app, "02-命令确认")
        app.buttons["稍后处理"].tap()
        let prompt = app.textFields["ai-prompt"]
        XCTAssertTrue(prompt.waitForExistence(timeout: 5)); prompt.tap(); prompt.typeText("check nginx")
        app.segmentedControls.buttons["监控"].tap()
        XCTAssertTrue(app.staticTexts["62%"].waitForExistence(timeout: 5)); capture(app, "03-监控")
        app.segmentedControls.buttons["AI 对话"].tap()
        XCTAssertEqual(prompt.value as? String, "check nginx")
        app.segmentedControls.buttons["终端"].tap()
        XCTAssertTrue(app.buttons["Ctrl+C"].waitForExistence(timeout: 5))
        XCUIDevice.shared.orientation = .landscapeLeft
        XCTAssertTrue(app.buttons["Tab"].exists)
        XCUIDevice.shared.orientation = .portrait
        let shot = XCTAttachment(screenshot: app.screenshot()); shot.name = "iPhone native terminal"; shot.lifetime = .keepAlways; add(shot)
    }
    func testHostWizardAndAIValidation() {
        let app = XCUIApplication(); app.launchArguments = ["--ui-testing"]; app.launch()
        app.tabBars.buttons["主机"].tap()
        app.buttons["new-item"].tap()
        app.buttons["host-next"].tap()
        XCTAssertTrue(app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "有效的服务器地址")).firstMatch.exists)
        let address = app.textFields["host-address"]; address.tap(); address.typeText("192.0.2.50"); app.buttons["完成输入"].tap()
        let user = app.textFields["host-user"]; user.tap(); user.typeText("ops")
        app.buttons["host-next"].tap()
        let password = app.secureTextFields["host-password"]; XCTAssertTrue(password.waitForExistence(timeout: 5)); password.tap(); password.typeText("ios-ui-test-password")
        app.buttons["host-next"].tap(); app.buttons["host-next"].tap()
        XCTAssertTrue(app.staticTexts["ops@192.0.2.50"].waitForExistence(timeout: 5))
        app.tabBars.buttons["设置"].tap(); app.buttons["ai-settings"].tap()
        let url = app.textFields["ai-url"]; url.tap(); url.typeText("http://model.example")
        let model = app.textFields["ai-model"]; model.tap(); model.typeText("test-model")
        app.buttons["save-ai"].tap()
        XCTAssertTrue(app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "HTTPS")).firstMatch.exists)
    }
}
