import SwiftUI
import LocalAuthentication

struct MonitorSnapshot {
    var system = ""
    var uptime = ""
    var memoryPercent: Double?
    var diskPercent: Double?
    var diskText = ""
    var date = Date()
    var raw = ""
    static let command = "printf '__WRD_OS__\\n'; uname -s; printf '__WRD_UPTIME__\\n'; uptime; printf '__WRD_MEMORY__\\n'; if test -r /proc/meminfo; then head -n 5 /proc/meminfo; fi; printf '__WRD_DISK__\\n'; df -Pk /"
    static func parse(_ output: String) -> MonitorSnapshot {
        func section(_ start: String, _ end: String?) -> String {
            guard let r = output.range(of: start + "\n") else { return "" }
            let remaining = output[r.upperBound...]
            return String(end.flatMap { remaining.range(of: $0) }.map { remaining[..<$0.lowerBound] } ?? remaining).trimmingCharacters(in: .whitespacesAndNewlines)
        }
        var result = MonitorSnapshot(system: section("__WRD_OS__", "__WRD_UPTIME__"), uptime: section("__WRD_UPTIME__", "__WRD_MEMORY__"), diskText: section("__WRD_DISK__", nil), raw: output)
        let memory = section("__WRD_MEMORY__", "__WRD_DISK__")
        func value(_ name: String) -> Double? { memory.split(separator: "\n").first(where: { $0.hasPrefix(name + ":") })?.split(whereSeparator: \.isWhitespace).dropFirst().first.flatMap { Double($0) } }
        if let total = value("MemTotal"), total > 0, let available = value("MemAvailable") { result.memoryPercent = max(0, min(100, (1 - available / total) * 100)) }
        result.diskPercent = result.diskText.split(whereSeparator: \.isWhitespace).first(where: { $0.hasSuffix("%") && Double($0.dropLast()) != nil }).flatMap { Double($0.dropLast()) }
        return result
    }
}

@MainActor
final class PhoneSession: ObservableObject, Identifiable {
    enum Status: String { case offline = "未连接", connecting = "正在连接", running = "已连接", ended = "已断开" }
    let id: UUID
    @Published var saved: SavedSession
    @Published var status: Status = .offline
    @Published var error = ""
    @Published var busy = false
    @Published var executing = false
    @Published var draft = ""
    @Published var streaming = ""
    @Published var proposal: Proposal?
    @Published var monitor: MonitorSnapshot?
    @Published var monitorError = ""
    @Published var monitoring = false
    @Published var collecting = false
    @Published var reviewRevision = UUID()
    var generation = UUID()
    var terminalData = Data()
    var terminalSink: ((Data) -> Void)?
    lazy var terminal = TerminalController(session: self)
    var connection: SSHConnection?
    var connectionTask: Task<Void, Never>?
    var aiTask: Task<Void, Never>?
    var executionTask: Task<Void, Never>?
    var monitorTask: Task<Void, Never>?
    var host: Host { saved.host }
    init(_ saved: SavedSession) { self.id = saved.id; self.saved = saved }
    func invalidateReview() { reviewRevision = UUID() }
}

@MainActor
final class AppState: ObservableObject {
    @Published var data: LocalData
    @Published var sessions: [PhoneSession]
    @Published var error = ""
    @Published var trust: HostTrustRequired?
    @Published var trustSessionID: UUID?
    @Published var selectedSession: UUID?
    @Published var locked = false
    @Published var foreground = true
    @Published var selectedTab = 0
    let store: LocalStore
    private let ai = AIClient()
    private var loadFailed = false
    private var backgroundTask: UIBackgroundTaskIdentifier = .invalid
    var demo = false

    init(store: LocalStore = LocalStore(), demo: Bool = false) {
        self.store = store
        do { let loaded = try store.load(); data = loaded; sessions = loaded.sessions.map(PhoneSession.init) }
        catch { data = LocalData(); sessions = []; self.error = "本地数据读取失败，已暂停保存：" + error.localizedDescription; loadFailed = true }
        self.demo = demo
        #if DEBUG
        if demo { loadDemo() }
        #endif
        locked = data.settings.lockOnBackground
    }

    func persist() {
        guard !demo, !loadFailed else { return }
        data.sessions = sessions.prefix(100).map { session in
            var saved = session.saved; saved.messages = Array(saved.messages.suffix(200)); return saved
        }
        do { try store.save(data) } catch { self.error = "保存失败：" + error.localizedDescription }
    }
    func saveHost(_ input: Host, secret: HostSecret) throws {
        guard !loadFailed else { throw AppFailure("本地数据读取失败，不能覆盖现有配置。") }
        let host = try input.validated()
        try validateChain(host)
        if !demo { try store.saveSecret(secret, id: host.id) }
        if let index = data.hosts.firstIndex(where: { $0.id == host.id }) { data.hosts[index] = host }
        else { data.hosts.append(host) }
        persist()
    }
    func deleteHost(_ host: Host) throws {
        guard !data.hosts.contains(where: { $0.jumpHostID == host.id }) else { throw AppFailure("这台主机仍被用作跳板机，请先解除关联。") }
        guard !sessions.contains(where: { $0.host.id == host.id && [.running, .connecting].contains($0.status) }) else { throw AppFailure("请先断开这台主机的会话。") }
        if !demo { try store.secrets.delete("host:" + host.id.uuidString) }
        data.hosts.removeAll { $0.id == host.id }; persist()
    }
    private func validateChain(_ host: Host) throws {
        var current = host; var seen = Set<UUID>()
        while true {
            guard seen.insert(current.id).inserted, seen.count <= 4 else { throw AppFailure("跳板连接不能循环，最多支持四层主机。") }
            guard let jump = current.jumpHostID else { return }
            guard let next = data.hosts.first(where: { $0.id == jump }) else { throw AppFailure("找不到所选跳板机。") }
            current = next
        }
    }
    func chain(_ host: Host) throws -> [Host] {
        try validateChain(host)
        var hosts = [host]
        while let jump = hosts.last?.jumpHostID, let next = data.hosts.first(where: { $0.id == jump }) { hosts.append(next) }
        return hosts.reversed()
    }
    func newSession(_ host: Host) {
        guard sessions.filter({ [.running, .connecting].contains($0.status) }).count < 8 else { error = "最多同时连接 8 个会话。"; return }
        let session = PhoneSession(SavedSession(host: host)); sessions.insert(session, at: 0)
        selectedSession = session.id; selectedTab = 0; persist(); connect(session)
    }
    func connect(_ session: PhoneSession) {
        guard session.status != .connecting, session.status != .running, !demo else { return }
        session.connectionTask?.cancel(); session.generation = UUID(); session.proposal = nil
        session.status = .connecting; session.error = ""; session.invalidateReview()
        let generation = session.generation
        session.connectionTask = Task {
            do {
                let currentHost = data.hosts.first(where: { $0.id == session.host.id }) ?? session.host
                let hosts = try chain(currentHost)
                var secrets: [UUID: HostSecret] = [:]
                for host in hosts { secrets[host.id] = try store.hostSecret(host.id) }
                let connection = SSHConnection(); session.connection = connection
                connection.onData = { [weak session] bytes in
                    guard let session, session.generation == generation else { return }
                    session.terminalData.append(bytes)
                    if session.terminalData.count > 256 * 1024 { session.terminalData = Data(session.terminalData.suffix(256 * 1024)) }
                    session.terminalSink?(bytes)
                }
                connection.onReady = { [weak session] in guard session?.generation == generation else { return }; session?.status = .running }
                connection.onClosed = { [weak self, weak session] reason in
                    guard let self, let session, session.generation == generation else { return }
                    session.status = .ended; session.error = reason; session.proposal = nil; session.invalidateReview()
                    session.monitorTask?.cancel(); self.persist()
                }
                try await connection.connect(hosts: hosts, secrets: secrets, trustedKeys: data.trustedKeys)
                try Task.checkCancellation()
                guard session.generation == generation else { await connection.close(); return }
                session.saved.host = currentHost; session.saved.updatedAt = Date(); persist()
            } catch let issue as HostTrustRequired {
                guard session.generation == generation else { return }
                session.status = .offline; trustSessionID = session.id; trust = issue
            } catch {
                guard session.generation == generation else { return }
                session.status = .ended; session.error = error.localizedDescription
            }
        }
    }
    func trustHost(_ issue: HostTrustRequired) {
        guard !issue.changed, trust?.id == issue.id else { return }
        data.trustedKeys[issue.host.endpoint] = issue.publicKey; persist()
        let session = sessions.first { $0.id == trustSessionID }
        trust = nil; trustSessionID = nil
        if let session { connect(session) }
    }
    func disconnect(_ session: PhoneSession) {
        session.generation = UUID(); session.invalidateReview(); session.proposal = nil
        session.connectionTask?.cancel(); session.aiTask?.cancel(); session.monitorTask?.cancel(); session.executionTask?.cancel()
        session.status = .ended; session.busy = false; session.executing = false; session.collecting = false; session.streaming = ""
        let connection = session.connection; session.connection = nil
        Task { await connection?.close() }
        persist()
    }
    func allSecrets(_ session: PhoneSession) -> [String] {
        let hosts = (try? chain(session.host)) ?? [session.host]
        return hosts.flatMap { host -> [String] in guard let value = try? store.hostSecret(host.id) else { return [] }; return [value.password, value.privateKey, value.passphrase] }
    }
    func send(_ session: PhoneSession, prompt: String? = nil) {
        guard !demo else { session.error = "当前为交互演示，不会请求 AI 或连接服务器。"; return }
        let text = (prompt ?? session.draft).trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty, !session.busy, !session.executing, session.proposal == nil else { return }
        guard !data.settings.baseURL.isEmpty else { session.error = "请先在「设置 → AI 服务」配置服务地址、模型和 API Key。"; return }
        let secrets = allSecrets(session) + [(try? store.aiKey()) ?? ""]
        let safeText = SensitiveText.redact(text, secrets: secrets)
        session.saved.messages.append(ChatMessage(role: "user", text: safeText)); session.draft = ""
        if session.saved.title == "新会话" { session.saved.title = String(safeText.prefix(32)) }
        requestAI(session)
    }
    private func requestAI(_ session: PhoneSession) {
        session.busy = true; session.streaming = ""; session.error = ""
        let generation = session.generation
        let settings = data.settings
        session.aiTask = Task {
            defer { if session.generation == generation { session.busy = false; session.streaming = ""; persist() } }
            do {
                let key = try store.aiKey()
                let reply = try await ai.ask(settings: settings, key: key, host: session.host, messages: session.saved.messages, terminal: String(decoding: session.terminalData, as: UTF8.self), secrets: allSecrets(session)) { text in
                    guard session.generation == generation else { return }; session.streaming = text
                }
                try Task.checkCancellation()
                guard session.generation == generation else { return }
                if !reply.text.isEmpty { session.saved.messages.append(ChatMessage(role: "assistant", text: reply.text)) }
                if let command = reply.command {
                    session.proposal = Proposal(command: command, explanation: reply.explanation, expectedEffect: reply.expectedEffect, risk: reply.risk, generation: generation)
                    session.invalidateReview()
                    session.saved.messages.append(ChatMessage(role: "assistant", text: "建议命令（尚未执行）：\n```sh\n\(command)\n```\n\(reply.explanation)"))
                }
                session.saved.updatedAt = Date()
            } catch {
                guard session.generation == generation else { return }
                if !session.streaming.isEmpty { session.saved.messages.append(ChatMessage(role: "assistant", text: session.streaming + "\n\n[回复中断]")) }
                session.error = error is CancellationError ? "已停止 AI 请求。" : error.localizedDescription
            }
        }
        persist()
    }
    func reject(_ session: PhoneSession, proposal: Proposal) {
        guard session.proposal?.id == proposal.id else { return }
        session.proposal = nil; session.invalidateReview()
        session.saved.messages.append(ChatMessage(role: "user", text: "已拒绝执行建议命令。请调整方案，不要执行原命令。")); persist()
    }
    func approve(_ session: PhoneSession, proposal: Proposal, reviewRevision: UUID) {
        guard foreground, !locked, !session.busy, !session.executing, session.status == .running,
              session.reviewRevision == reviewRevision, session.proposal == proposal,
              proposal.generation == session.generation, let connection = session.connection else {
            session.error = "连接或命令状态已变化，请重新打开确认页。"; return
        }
        session.proposal = nil; session.executing = true; session.invalidateReview(); session.error = ""
        let generation = session.generation
        let secrets = allSecrets(session) + [(try? store.aiKey()) ?? ""]
        session.saved.messages.append(ChatMessage(role: "system", text: "用户已批准本次操作，准备发送命令。尚未确认完成；如果中断，请先核对服务器状态：\n" + proposal.command))
        persist()
        session.executionTask = Task {
            defer { if session.generation == generation { session.executing = false; persist() } }
            do {
                let result = try await connection.execute(proposal.command)
                guard session.generation == generation else { return }
                let output = SensitiveText.redact(result.output, secrets: secrets)
                session.saved.messages.append(ChatMessage(role: "user", text: "用户已批准且手机已发送命令：\n\(proposal.command)\n\(result.summary)\n\(result.truncated ? "[输出过长，仅保留末尾]\n" : "")\(output)"))
                if result.exitCode != nil, foreground { session.executing = false; requestAI(session) }
                else { session.error = result.summary }
            } catch {
                guard session.generation == generation else { return }
                session.error = "执行状态未知：" + error.localizedDescription
                session.saved.messages.append(ChatMessage(role: "system", text: "命令已发送，但未取得确定结果。请核对远端状态后再操作：\n" + proposal.command))
            }
        }
        persist()
    }
    func refreshMonitor(_ session: PhoneSession) async {
        guard !session.collecting, foreground, session.status == .running, let connection = session.connection else { return }
        session.collecting = true
        let generation = session.generation
        defer { if session.generation == generation { session.collecting = false } }
        do {
            let result = try await connection.execute(MonitorSnapshot.command, timeout: 15)
            guard session.generation == generation else { return }
            guard result.exitCode == 0 else { throw AppFailure(result.summary) }
            let snapshot = MonitorSnapshot.parse(result.output)
            session.monitor = snapshot; session.monitorError = ""
            for (title, percent) in [("磁盘使用率过高", snapshot.diskPercent), ("内存使用率过高", snapshot.memoryPercent)] {
                if let percent, percent >= 90, !data.incidents.contains(where: { $0.sessionID == session.id && $0.title == title && Date().timeIntervalSince($0.date) < 15*60 }) {
                    let incident = Incident(sessionID: session.id, hostName: session.host.displayName, title: title, detail: "当前使用率 \(Int(percent))%，阈值 90%。")
                    data.incidents.insert(incident, at: 0); data.incidents = Array(data.incidents.prefix(100)); persist()
                }
            }
        } catch { if session.generation == generation { session.monitorError = error.localizedDescription } }
    }
    func setMonitoring(_ session: PhoneSession, enabled: Bool) {
        session.monitoring = enabled; session.monitorTask?.cancel()
        guard enabled, foreground else { return }
        session.monitorTask = Task {
            while !Task.isCancelled, session.monitoring, session.status == .running, foreground {
                await refreshMonitor(session)
                try? await Task.sleep(nanoseconds: 15_000_000_000)
            }
        }
    }
    func sceneChanged(_ phase: ScenePhase) {
        for session in sessions { session.invalidateReview() }
        foreground = phase == .active
        if phase == .active {
            if backgroundTask != .invalid { UIApplication.shared.endBackgroundTask(backgroundTask); backgroundTask = .invalid }
            for session in sessions where session.monitoring { setMonitoring(session, enabled: true) }
        } else if phase == .background {
            persist()
            if data.settings.lockOnBackground { locked = true }
            for session in sessions { session.monitorTask?.cancel(); session.aiTask?.cancel() }
            if backgroundTask == .invalid {
                backgroundTask = UIApplication.shared.beginBackgroundTask(withName: "保存 SSH 会话") { [weak self] in
                    Task { @MainActor in
                        guard let self else { return }
                        for session in self.sessions where [.running, .connecting].contains(session.status) {
                            self.disconnect(session); session.error = "iOS 后台时间已结束，请重新连接。远端命令的状态需核对。"
                        }
                        self.persist()
                        UIApplication.shared.endBackgroundTask(self.backgroundTask); self.backgroundTask = .invalid
                    }
                }
            }
        }
    }
    func unlock() async {
        do {
            let accepted = try await LAContext().evaluatePolicy(.deviceOwnerAuthentication, localizedReason: "解锁 SSH 工作台与手机凭据")
            if accepted { locked = false }
        } catch { self.error = "未能解锁：" + error.localizedDescription }
    }
    #if DEBUG
    private func loadDemo() {
        let host = Host(name: "prod-api-01", address: "192.0.2.12", username: "ops", environment: "生产环境")
        data = LocalData(hosts: [host, Host(name: "staging-api", address: "192.0.2.18", username: "dev", environment: "测试环境")])
        let session = PhoneSession(SavedSession(host: host, title: "排查订单接口超时", messages: [ChatMessage(role: "user", text: "订单接口持续超时，帮我看看原因。"), ChatMessage(role: "assistant", text: "最近的输出出现上游连接超时。先查看系统负载，再核对上游服务状态。这里是交互演示，不会连接真实服务器。")]))
        session.proposal = Proposal(command: "uptime", explanation: "检查系统负载与运行时间。", expectedEffect: "读取当前负载，不修改服务器。", risk: "low", generation: session.generation)
        session.monitor = MonitorSnapshot(system: "Linux", uptime: "up 42 days · load average: 0.82, 0.68, 0.61", memoryPercent: 62, diskPercent: 71, diskText: "/dev/vda1  80G  57G  23G  71% /")
        sessions = [session]
        data.incidents = [Incident(sessionID: session.id, hostName: host.displayName, title: "订单接口超时增多", detail: "示例：5 分钟内出现 28 次上游超时。")]
    }
    #endif
}
