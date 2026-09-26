import XCTest
@testable import WRDAISSH

final class MemorySecrets: SecretStorage {
    var items: [String: Data] = [:]
    func read(_ account: String) throws -> Data? { items[account] }
    func write(_ account: String, data: Data) throws { items[account] = data }
    func delete(_ account: String) throws { items.removeValue(forKey: account) }
}

final class CoreTests: XCTestCase {
    func testHostValidationAndRoutingIdentity() throws {
        let host = try Host(address: " server.example ", port: 2222, username: " ops ").validated()
        XCTAssertEqual(host.endpoint, "[server.example]:2222")
        XCTAssertEqual(host.target, "ops@server.example:2222")
        for address in ["", "https://server", "user@server", "bad host", "server\nname"] {
            XCTAssertThrowsError(try Host(address: address, username: "ops").validated())
        }
        XCTAssertThrowsError(try Host(address: "server", port: 0, username: "ops").validated())
    }
    func testAIEndpointNeverAllowsInsecureOrCredentialURLs() throws {
        XCTAssertEqual(try AISettings(baseURL: "https://model.example/v1/", model: "model").endpoint().absoluteString, "https://model.example/v1/chat/completions")
        XCTAssertEqual(try AISettings(baseURL: "https://model.example/chat/completions", model: "model").endpoint().path, "/chat/completions")
        for url in ["http://model.example", "https://user:key@model.example", "https://model.example?key=x", "https://model.example/#x", "file:///private/file", ""] {
            XCTAssertThrowsError(try AISettings(baseURL: url).endpoint(), url)
        }
    }
    func testRiskCannotBeLoweredByModelOrShellSyntax() {
        XCTAssertTrue(CommandSafety.isSimpleRead("uptime", claimedRisk: "low"))
        XCTAssertTrue(CommandSafety.isSimpleRead("df -h", claimedRisk: "read"))
        for command in ["uptime; reboot", "uptime\nreboot", "echo $(reboot)", "sudo rm -rf /", "curl example.com", "df > /etc/config", "df --output=$(id)", "tail /var/log/app.log", "unknown"] {
            XCTAssertFalse(CommandSafety.isSimpleRead(command, claimedRisk: "low"), command)
        }
        XCTAssertFalse(CommandSafety.isSimpleRead("uptime", claimedRisk: "unknown"))
        XCTAssertFalse(CommandSafety.isSimpleRead("uptime", claimedRisk: "critical"))
    }
    func testIncompleteAICommandIsNeverExecutable() throws {
        var parser = ToolAccumulator()
        try parser.accept(Data(#"{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"propose_command","arguments":"{\"command\":\"uptime\""}}]}}]}"#.utf8))
        XCTAssertThrowsError(try parser.response())
        try parser.accept(Data(#"{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":",\"explanation\":\"check load\",\"risk\":\"low\",\"expectedEffect\":\"read\"}"}}]},"finish_reason":"tool_calls"}]}"#.utf8))
        let result = try parser.response()
        XCTAssertEqual(result.command, "uptime")
        XCTAssertEqual(result.risk, "low")
    }
    func testTruncatedAndMultipleCommandsAreRejected() throws {
        var parser = ToolAccumulator()
        try parser.accept(Data(#"{"choices":[{"message":{"content":"partial"},"finish_reason":"length"}]}"#.utf8))
        XCTAssertThrowsError(try parser.response())
        parser = ToolAccumulator()
        try parser.accept(Data(#"{"choices":[{"message":{"tool_calls":[{"function":{"name":"propose_command","arguments":"{}"}},{"function":{"name":"propose_command","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}"#.utf8))
        XCTAssertThrowsError(try parser.response())
    }
    func testPlainTextNeverBecomesCommand() throws {
        var parser = ToolAccumulator()
        try parser.accept(Data(#"{"choices":[{"message":{"content":"Run sudo reboot"},"finish_reason":"stop"}]}"#.utf8))
        XCTAssertNil(try parser.response().command)
    }
    func testExecutionRequiresMatchingCompletionMarker() {
        let command = "printf '%s' \"quoted value\"; exit 7"
        let wrapped = ExecutionEnvelope.wrap(command, marker: "UNIQUE")
        XCTAssertTrue(wrapped.contains("'\\''"))
        XCTAssertEqual(ExecutionEnvelope.parse("some output\nUNIQUE7\n", marker: "UNIQUE").exitCode, 7)
        XCTAssertEqual(ExecutionEnvelope.parse("some output\nUNIQUE0\n", marker: "UNIQUE").output, "some output")
        XCTAssertNil(ExecutionEnvelope.parse("some output\nOTHER0\n", marker: "UNIQUE").exitCode)
        XCTAssertNil(ExecutionEnvelope.parse("some output\nUNIQUEbad\n", marker: "UNIQUE").exitCode)
    }
    func testSecretsNeverEnterLocalCatalog() throws {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: dir) }
        let store = LocalStore(directory: dir, secrets: MemorySecrets())
        let host = Host(name: "test", address: "test", username: "ops")
        try store.saveSecret(HostSecret(password: "secret-password", privateKey: "secret-private-key", passphrase: "secret-passphrase"), id: host.id)
        try store.saveAIKey("secret-ai-key")
        try store.save(LocalData(hosts: [host]))
        let contents = try String(contentsOf: dir.appendingPathComponent("workspace.json"), encoding: .utf8)
        XCTAssertFalse(contents.contains("secret-"))
        XCTAssertEqual(try store.hostSecret(host.id).password, "secret-password")
        XCTAssertEqual(try store.aiKey(), "secret-ai-key")
        XCTAssertEqual(try store.load().hosts, [host])
    }
    func testCorruptLocalHistoryIsNotSilentlyReset() throws {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: dir) }
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        try Data("corrupt".utf8).write(to: dir.appendingPathComponent("workspace.json"))
        XCTAssertThrowsError(try LocalStore(directory: dir, secrets: MemorySecrets()).load())
    }
    func testKeychainRoundTripUsesIsolatedNamespace() throws {
        let keychain = KeychainStore(service: "com.wrdtech.aissh.tests." + UUID().uuidString)
        defer { try? keychain.delete("test") }
        try keychain.write("test", data: Data("first".utf8))
        XCTAssertEqual(try keychain.read("test"), Data("first".utf8))
        try keychain.write("test", data: Data("second".utf8))
        XCTAssertEqual(try keychain.read("test"), Data("second".utf8))
        try keychain.delete("test")
        XCTAssertNil(try keychain.read("test"))
    }
    func testRedactionAndMonitorParsing() {
        let raw = "password=password123 token=token123\n-----BEGIN OPENSSH PRIVATE KEY-----\nprivate\n-----END OPENSSH PRIVATE KEY-----"
        let redacted = SensitiveText.redact(raw, secrets: ["password123", "token123"])
        XCTAssertFalse(redacted.contains("password123")); XCTAssertFalse(redacted.contains("token123")); XCTAssertFalse(redacted.contains("private"))
        let monitor = MonitorSnapshot.parse("__WRD_OS__\nLinux\n__WRD_UPTIME__\nup 3 days\n__WRD_MEMORY__\nMemTotal: 1000 kB\nMemAvailable: 200 kB\n__WRD_DISK__\nFilesystem 1024-blocks Used Available Capacity Mounted on\n/dev/vda 1000 930 70 93% /\n")
        XCTAssertEqual(monitor.memoryPercent, 80); XCTAssertEqual(monitor.diskPercent, 93); XCTAssertEqual(monitor.system, "Linux")
    }
}

@MainActor
final class StateTests: XCTestCase {
    func testApprovalRequiresCurrentConnectionAndReviewGeneration() {
        let app = AppState(store: LocalStore(directory: FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString), secrets: MemorySecrets()))
        let session = PhoneSession(SavedSession(host: Host(name: "test", address: "test", username: "ops")))
        let proposal = Proposal(command: "uptime", explanation: "read", expectedEffect: "read", risk: "low", generation: session.generation)
        session.proposal = proposal; session.status = .running; session.connection = SSHConnection()
        let revision = session.reviewRevision
        session.invalidateReview()
        app.approve(session, proposal: proposal, reviewRevision: revision)
        XCTAssertFalse(session.executing); XCTAssertNotNil(session.proposal)
        app.foreground = false
        app.approve(session, proposal: proposal, reviewRevision: session.reviewRevision)
        XCTAssertFalse(session.executing); XCTAssertNotNil(session.proposal)
        app.foreground = true; session.generation = UUID()
        app.approve(session, proposal: proposal, reviewRevision: session.reviewRevision)
        XCTAssertFalse(session.executing); XCTAssertNotNil(session.proposal)
    }
    func testJumpCycleRejected() throws {
        let app = AppState(store: LocalStore(directory: FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString), secrets: MemorySecrets()))
        var first = Host(name: "one", address: "one", username: "ops")
        let second = Host(name: "two", address: "two", username: "ops", jumpHostID: first.id)
        first.jumpHostID = second.id
        app.data.hosts = [first, second]
        XCTAssertThrowsError(try app.chain(first))
    }
}
