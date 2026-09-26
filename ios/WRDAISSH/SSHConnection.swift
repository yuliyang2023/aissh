import Foundation
import Citadel
import NIOCore
import NIOSSH
import Crypto

struct HostTrustRequired: LocalizedError, Identifiable {
    var id = UUID()
    let host: Host
    let publicKey: String
    let fingerprint: String
    let changed: Bool
    var errorDescription: String? { changed ? "主机指纹与手机保存的记录不一致，已停止连接。" : "首次连接需要核对主机指纹。" }
}

final class HostKeyChecker: NIOSSHClientServerAuthenticationDelegate, @unchecked Sendable {
    private let host: Host
    private let expected: String?
    private let lock = NSLock()
    private var issue: HostTrustRequired?
    init(host: Host, expected: String?) { self.host = host; self.expected = expected }
    var failure: HostTrustRequired? { lock.lock(); defer { lock.unlock() }; return issue }
    func validateHostKey(hostKey: NIOSSHPublicKey, validationCompletePromise: EventLoopPromise<Void>) {
        let key = String(openSSHPublicKey: hostKey)
        if key == expected { validationCompletePromise.succeed(()); return }
        let body = key.split(separator: " ").dropFirst().first.map(String.init) ?? ""
        let digest = SHA256.hash(data: Data(base64Encoded: body) ?? Data(key.utf8))
        let fingerprint = "SHA256:" + Data(digest).base64EncodedString().replacingOccurrences(of: "=", with: "")
        let failure = HostTrustRequired(host: host, publicKey: key, fingerprint: fingerprint, changed: expected != nil)
        lock.lock(); issue = failure; lock.unlock()
        validationCompletePromise.fail(failure)
    }
}

struct ExecutionResult: Equatable {
    let output: String
    let exitCode: Int?
    let truncated: Bool
    var summary: String { exitCode.map { $0 == 0 ? "执行完成 · 退出码 0" : "执行失败 · 退出码 \($0)" } ?? "执行状态未知，请核对服务器状态" }
}

enum ExecutionEnvelope {
    static func wrap(_ command: String, marker: String) -> String {
        let quoted = "'" + command.replacingOccurrences(of: "'", with: "'\\''") + "'"
        return "sh -c " + quoted + "; _wrd_status=$?; printf '\\n" + marker + "%s\\n' \"$_wrd_status\""
    }
    static func parse(_ output: String, marker: String, truncated: Bool = false) -> ExecutionResult {
        guard let range = output.range(of: "\n" + marker, options: .backwards) else { return ExecutionResult(output: output, exitCode: nil, truncated: truncated) }
        let suffix = output[range.upperBound...].trimmingCharacters(in: .whitespacesAndNewlines)
        guard let exit = Int(suffix), (0...255).contains(exit) else { return ExecutionResult(output: output, exitCode: nil, truncated: truncated) }
        return ExecutionResult(output: String(output[..<range.lowerBound]), exitCode: exit, truncated: truncated)
    }
}

@MainActor
final class SSHConnection {
    private(set) var client: SSHClient?
    private var chain: [SSHClient] = []
    private var writer: TTYStdinWriter?
    private var shellTask: Task<Void, Never>?
    private var writeTask: Task<Void, Never>?
    private var closing = false
    var onData: ((Data) -> Void)?
    var onClosed: ((String) -> Void)?
    var onReady: (() -> Void)?
    var ready = false

    static func authentication(_ host: Host, _ secret: HostSecret) throws -> SSHAuthenticationMethod {
        if host.authentication == .password {
            guard !secret.password.isEmpty else { throw AppFailure("请先保存这台主机的登录密码。") }
            return .passwordBased(username: host.username, password: secret.password)
        }
        guard !secret.privateKey.isEmpty else { throw AppFailure("请先导入 SSH 私钥。") }
        let passphrase = secret.passphrase.isEmpty ? nil : Data(secret.passphrase.utf8)
        if let key = try? Curve25519.Signing.PrivateKey(sshEd25519: secret.privateKey, decryptionKey: passphrase) {
            return .ed25519(username: host.username, privateKey: key)
        }
        if let key = try? Insecure.RSA.PrivateKey(sshRsa: secret.privateKey, decryptionKey: passphrase) {
            return .rsa(username: host.username, privateKey: key)
        }
        throw AppFailure("无法读取私钥。首版支持 OpenSSH 格式的 Ed25519 / RSA 私钥，请检查口令和格式。")
    }

    func connect(hosts: [Host], secrets: [UUID: HostSecret], trustedKeys: [String: String]) async throws {
        guard client == nil, !hosts.isEmpty else { throw AppFailure("连接状态无效。") }
        closing = false
        do {
            for host in hosts {
                try Task.checkCancellation()
                let auth = try Self.authentication(host, secrets[host.id] ?? HostSecret())
                let checker = HostKeyChecker(host: host, expected: trustedKeys[host.endpoint])
                var settings = SSHClientSettings(host: host.address, port: host.port, authenticationMethod: { auth }, hostKeyValidator: .custom(checker))
                settings.connectTimeout = .seconds(15)
                do {
                    let connection: SSHClient
                    if let jump = chain.last { connection = try await jump.jump(to: settings) }
                    else { connection = try await SSHClient.connect(to: settings) }
                    chain.append(connection)
                } catch { throw checker.failure ?? error }
            }
            try Task.checkCancellation()
            guard !closing, let connection = chain.last else { throw CancellationError() }
            client = connection
            connection.onDisconnect { [weak self] in Task { @MainActor in self?.disconnected("SSH 连接已结束") } }
            shellTask = Task { [weak self] in
                guard let self else { return }
                do {
                    try await connection.withPTY(.init(wantReply: true, term: "xterm-256color", terminalCharacterWidth: 48, terminalRowHeight: 24, terminalPixelWidth: 0, terminalPixelHeight: 0, terminalModes: .init([.ECHO: 1]))) { output, input in
                        self.writer = input; self.ready = true; self.onReady?()
                        for try await chunk in output {
                            try Task.checkCancellation()
                            switch chunk {
                            case .stdout(let buffer), .stderr(let buffer): self.onData?(Data(buffer.readableBytesView))
                            }
                        }
                    }
                    self.disconnected("SSH 会话已结束")
                } catch { if !self.closing { self.disconnected("终端连接中断：" + error.localizedDescription) } }
            }
        } catch { await close(); throw error }
    }

    private func disconnected(_ reason: String) {
        guard !closing else { return }
        ready = false; writer = nil
        onClosed?(reason)
        Task { await close() }
    }
    func write(_ data: Data) {
        guard ready, !closing, let input = writer else { return }
        let previous = writeTask
        writeTask = Task { [weak self] in
            await previous?.value
            guard !Task.isCancelled, self?.ready == true else { return }
            do { try await input.write(ByteBuffer(bytes: data)) }
            catch { self?.disconnected("发送终端输入失败：" + error.localizedDescription) }
        }
    }
    func resize(cols: Int, rows: Int) {
        guard let writer, ready else { return }
        Task { try? await writer.changeSize(cols: max(20, min(cols, 300)), rows: max(5, min(rows, 200)), pixelWidth: 0, pixelHeight: 0) }
    }
    func execute(_ command: String, timeout: UInt64 = 60) async throws -> ExecutionResult {
        guard let client, !closing else { throw AppFailure("SSH 未连接。") }
        let marker = "__WRD_EXIT_" + UUID().uuidString.replacingOccurrences(of: "-", with: "") + "__"
        return try await withThrowingTaskGroup(of: ExecutionResult.self) { group in
            group.addTask {
                var bytes = Data(); var truncated = false
                do {
                    try await client.withExec(ExecutionEnvelope.wrap(command, marker: marker)) { output, _ in
                        for try await chunk in output {
                            try Task.checkCancellation()
                            switch chunk { case .stdout(let buffer), .stderr(let buffer): bytes.append(contentsOf: buffer.readableBytesView) }
                            if bytes.count > 128 * 1024 { bytes = Data(bytes.suffix(128 * 1024)); truncated = true }
                        }
                        try Task.checkCancellation()
                    }
                } catch ChannelError.alreadyClosed {
                    // Citadel closes the exec channel after EOF; the peer may
                    // already have closed it. Only our marker determines success.
                }
                try Task.checkCancellation()
                return ExecutionEnvelope.parse(String(decoding: bytes, as: UTF8.self), marker: marker, truncated: truncated)
            }
            group.addTask {
                try await Task.sleep(nanoseconds: timeout * 1_000_000_000)
                throw AppFailure("等待执行结果超时；命令可能仍在运行，请核对状态，不要重复执行。")
            }
            defer { group.cancelAll() }
            guard let result = try await group.next() else { throw CancellationError() }
            return result
        }
    }
    func close() async {
        closing = true; ready = false; writer = nil
        shellTask?.cancel(); shellTask = nil; writeTask?.cancel(); writeTask = nil
        let connections = chain.reversed(); chain = []; client = nil
        for connection in connections { try? await connection.close() }
    }
}
