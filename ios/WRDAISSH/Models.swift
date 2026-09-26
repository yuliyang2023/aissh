import Foundation

struct Host: Identifiable, Codable, Equatable, Hashable {
    enum Authentication: String, Codable, CaseIterable { case password, privateKey }
    var id = UUID()
    var name = ""
    var address = ""
    var port = 22
    var username = ""
    var environment = ""
    var authentication: Authentication = .password
    var jumpHostID: UUID?
    var displayName: String { name.isEmpty ? "\(username)@\(address)" : name }
    var endpoint: String { "[\(address.lowercased())]:\(port)" }
    var target: String { "\(username)@\(address):\(port)" }
    func validated() throws -> Host {
        var value = self
        value.name = name.trimmingCharacters(in: .whitespacesAndNewlines)
        value.address = address.trimmingCharacters(in: .whitespacesAndNewlines)
        if value.address.hasPrefix("["), value.address.hasSuffix("]") { value.address = String(value.address.dropFirst().dropLast()) }
        value.username = username.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !value.address.isEmpty, !value.username.isEmpty, (1...65535).contains(port),
              !value.address.contains(where: { $0.isWhitespace || $0.isNewline }),
              !value.address.contains("/"), !value.address.contains("@"),
              value.address.rangeOfCharacter(from: .controlCharacters) == nil, value.username.rangeOfCharacter(from: .controlCharacters) == nil,
              !value.username.contains(where: { $0.isWhitespace || $0.isNewline }), jumpHostID != id else {
            throw AppFailure("请填写有效的服务器地址、端口和用户名。")
        }
        return value
    }
}

struct HostSecret: Codable, Equatable {
    var password = ""
    var privateKey = ""
    var passphrase = ""
}

struct AISettings: Codable, Equatable {
    var baseURL = ""
    var model = ""
    var includeTerminal = true
    var lockOnBackground = false
    func endpoint() throws -> URL {
        guard var parts = URLComponents(string: baseURL.trimmingCharacters(in: .whitespacesAndNewlines)),
              parts.scheme == "https", let host = parts.host, !host.isEmpty,
              parts.user == nil, parts.password == nil, parts.query == nil, parts.fragment == nil else {
            throw AppFailure("AI 地址需要使用 HTTPS，不能包含账号、密码或查询参数。")
        }
        var path = parts.path
        while path.hasSuffix("/") { path.removeLast() }
        if !path.hasSuffix("/chat/completions") { path += "/chat/completions" }
        parts.path = path
        guard let url = parts.url else { throw AppFailure("AI 地址无效。") }
        return url
    }
}

struct ChatMessage: Identifiable, Codable, Equatable {
    var id = UUID()
    var role: String
    var text: String
    var date = Date()
}

struct SavedSession: Identifiable, Codable {
    var id = UUID()
    var host: Host
    var title = "新会话"
    var messages: [ChatMessage] = []
    var updatedAt = Date()
}

struct LocalData: Codable {
    var version = 1
    var hosts: [Host] = []
    var trustedKeys: [String: String] = [:]
    var sessions: [SavedSession] = []
    var settings = AISettings()
    var incidents: [Incident] = []
}

struct Incident: Identifiable, Codable {
    var id = UUID()
    var sessionID: UUID
    var hostName: String
    var title: String
    var detail: String
    var date = Date()
    var acknowledged = false
}

struct Proposal: Identifiable, Equatable {
    var id = UUID()
    var command: String
    var explanation: String
    var expectedEffect: String
    var risk: String
    var generation: UUID
    var needsExtraConfirmation: Bool { !CommandSafety.isSimpleRead(command, claimedRisk: risk) }
}

enum CommandSafety {
    // Unknown syntax or model risk always falls back to an extra confirmation.
    static func isSimpleRead(_ command: String, claimedRisk: String) -> Bool {
        guard ["low", "read"].contains(claimedRisk.lowercased()), !command.isEmpty,
              command.rangeOfCharacter(from: CharacterSet(charactersIn: "\n\r;|&<>`$\\(){}")) == nil else { return false }
        let parts = command.split(whereSeparator: \.isWhitespace).map(String.init)
        guard let first = parts.first else { return false }
        if ["uptime", "whoami", "pwd", "hostname", "date"].contains(first) { return parts.count == 1 }
        if ["df", "free", "uname", "ps"].contains(first) { return parts.dropFirst().allSatisfy { $0.hasPrefix("-") && $0.allSatisfy { $0.isLetter || $0 == "-" } } }
        return false
    }
    static func validate(_ command: String) throws {
        guard !command.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty, command.utf8.count <= 64 * 1024,
              !command.unicodeScalars.contains(where: { $0.value == 0 || $0.value == 27 }) else {
            throw AppFailure("AI 返回了无效或过长的命令，未创建可执行操作。")
        }
    }
}

struct AppFailure: LocalizedError {
    let message: String
    init(_ message: String) { self.message = message }
    var errorDescription: String? { message }
}

enum SensitiveText {
    static func redact(_ text: String, secrets: [String]) -> String {
        var value = text
        for secret in secrets.filter({ !$0.isEmpty }).sorted(by: { $0.count > $1.count }) {
            value = value.replacingOccurrences(of: secret, with: "[已隐藏]")
        }
        return value.replacingOccurrences(of: "-----BEGIN [^-]*PRIVATE KEY-----[\\s\\S]*?-----END [^-]*PRIVATE KEY-----", with: "[私钥已隐藏]", options: .regularExpression)
    }
    static func terminalContext(_ text: String) -> String {
        let plain = text.replacingOccurrences(of: "\\x1B\\[[0-?]*[ -/]*[@-~]|\\x1B\\][^\\x07]*(?:\\x07|\\x1B\\\\)", with: "", options: .regularExpression)
        return String(plain.suffix(8000))
    }
}
