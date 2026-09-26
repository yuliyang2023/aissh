import XCTest
@testable import WRDAISSH

@MainActor
final class SSHIntegrationTests: XCTestCase {
    // The fixture is enabled explicitly by scripts/test-ios.sh. It only binds loopback.
    func testIndependentSSHTrustPTYExecAndFailure() async throws {
        guard ProcessInfo.processInfo.environment["WRD_SSH_INTEGRATION"] == "1" else { throw XCTSkip("Run scripts/test-ios.sh for SSH integration") }
        let host = Host(name: "Local test fixture", address: "127.0.0.1", port: 22882, username: "phone-test")
        let secret = HostSecret(password: "local-fixture-password")
        let first = SSHConnection()
        var trusted: String?
        do { try await first.connect(hosts: [host], secrets: [host.id: secret], trustedKeys: [:]); XCTFail("unknown host accepted") }
        catch let issue as HostTrustRequired { XCTAssertFalse(issue.changed); XCTAssertTrue(issue.fingerprint.hasPrefix("SHA256:")); trusted = issue.publicKey }
        guard let trusted else { return XCTFail("no host fingerprint received") }
        let changed = SSHConnection()
        do { try await changed.connect(hosts: [host], secrets: [host.id: secret], trustedKeys: [host.endpoint:"wrong-key"]); XCTFail("changed host key accepted") }
        catch let issue as HostTrustRequired { XCTAssertTrue(issue.changed) }
        let connection = SSHConnection()
        let ready = expectation(description: "PTY ready")
        let echoed = expectation(description: "terminal roundtrip")
        var didEcho = false
        connection.onReady = { ready.fulfill() }
        connection.onData = { data in if !didEcho && String(decoding:data,as:UTF8.self).contains("PHONE_TERMINAL_OK") { didEcho = true; echoed.fulfill() } }
        try await connection.connect(hosts: [host], secrets: [host.id: secret], trustedKeys: [host.endpoint:trusted])
        await fulfillment(of: [ready], timeout: 10)
        connection.write(Data("test\r".utf8))
        await fulfillment(of: [echoed], timeout: 10)
        let result = try await connection.execute("fixture-success")
        XCTAssertEqual(result.exitCode, 0); XCTAssertEqual(result.output, "PHONE_EXEC_OK")
        let failure = try await connection.execute("fixture-nonzero")
        XCTAssertEqual(failure.exitCode, 7); XCTAssertEqual(failure.output, "EXPECTED_FAILURE")
        let unknown = try await connection.execute("fixture-no-marker")
        XCTAssertNil(unknown.exitCode)
        do { _ = try await connection.execute("fixture-timeout", timeout: 1); XCTFail("timeout accepted as success") }
        catch { XCTAssertTrue(error.localizedDescription.contains("超时")) }
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: dir) }
        let store = LocalStore(directory: dir, secrets: MemorySecrets())
        let app = AppState(store: store)
        let session = PhoneSession(SavedSession(host: host))
        session.connection = connection; session.status = .running; app.sessions = [session]
        let proposal = Proposal(command: "fixture-success", explanation: "fixture only", expectedEffect: "test result", risk: "low", generation: session.generation)
        session.proposal = proposal
        let revision = session.reviewRevision
        app.approve(session, proposal: proposal, reviewRevision: revision)
        // The approval is recorded before any network await, so a killed app
        // cannot reopen a history that incorrectly says nothing was sent.
        XCTAssertTrue(try store.load().sessions[0].messages.contains { $0.text.contains("用户已批准本次操作") })
        app.approve(session, proposal: proposal, reviewRevision: revision)
        await session.executionTask?.value
        XCTAssertEqual(session.saved.messages.filter { $0.text.contains("手机已发送命令") }.count, 1)
        XCTAssertTrue(session.saved.messages.contains { $0.text.contains("PHONE_EXEC_OK") })
        XCTAssertNil(session.proposal)
        session.aiTask?.cancel()
        await connection.close()
        XCTAssertFalse(connection.ready)
    }
}
