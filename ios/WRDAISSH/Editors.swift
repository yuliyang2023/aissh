import SwiftUI
import UniformTypeIdentifiers

struct HostEditor: View {
    @EnvironmentObject var app: AppState
    @Environment(\.dismiss) var dismiss
    @State var host: Host
    @State private var secret = HostSecret()
    @State private var port = "22"
    @State private var step = 0
    @State private var error = ""
    @State private var importing = false
    @State private var forgetting = false
    @State private var deleting = false
    @State private var loaded = false
    private enum Field: Hashable { case name, address, port, user, environment, password, privateKey, passphrase }
    @FocusState private var editing: Field?
    private var existing: Bool { app.data.hosts.contains { $0.id == host.id } }
    var body: some View {
        NavigationStack {
            Form {
                Section { HStack { ForEach(0..<3) { i in Capsule().fill(i <= step ? Color.accentColor : Color.secondary.opacity(0.15)).frame(height: 4) } }; Text(["1 / 3 · 连接目标", "2 / 3 · 身份验证", "3 / 3 · 核对连接"][step]).font(.caption).foregroundStyle(.secondary) }
                if step == 0 {
                    Section("服务器") {
                        TextField("主机名称（可选）", text: $host.name).accessibilityIdentifier("host-name").focused($editing, equals: .name)
                        TextField("IP 地址或域名", text: $host.address).textInputAutocapitalization(.never).autocorrectionDisabled().keyboardType(.URL).accessibilityIdentifier("host-address").focused($editing, equals: .address)
                        TextField("端口", text: $port).keyboardType(.numberPad).accessibilityIdentifier("host-port").focused($editing, equals: .port)
                        TextField("SSH 用户名", text: $host.username).textInputAutocapitalization(.never).autocorrectionDisabled().accessibilityIdentifier("host-user").focused($editing, equals: .user)
                        TextField("环境 / 分组，如生产环境", text: $host.environment).focused($editing, equals: .environment)
                    }
                    Section { Text("SSH 由 iPhone 直接连接。请确保手机能够访问目标网络。") .font(.footnote).foregroundStyle(.secondary) }
                } else if step == 1 {
                    Section("登录方式") {
                        Picker("身份验证", selection: $host.authentication) { Text("密码").tag(Host.Authentication.password); Text("SSH 私钥").tag(Host.Authentication.privateKey) }.pickerStyle(.segmented)
                        if host.authentication == .password {
                            SecureField("SSH 登录密码", text: $secret.password).textContentType(.password).accessibilityIdentifier("host-password").focused($editing, equals: .password)
                        } else {
                            LabeledContent("私钥", value: secret.privateKey.isEmpty ? "尚未导入" : "已导入")
                            Button("从文件导入私钥", systemImage: "doc.badge.plus") { importing = true }
                            DisclosureGroup("粘贴私钥内容") { TextEditor(text: $secret.privateKey).font(.system(.caption, design: .monospaced)).frame(minHeight: 160).textInputAutocapitalization(.never).autocorrectionDisabled().accessibilityLabel("私钥内容") }
                            SecureField("私钥口令（如有）", text: $secret.passphrase)
                            Text("支持 OpenSSH 格式的 Ed25519 / RSA 私钥，包括有口令的私钥。") .font(.footnote).foregroundStyle(.secondary)
                        }
                    }
                    Section { Text("密码、私钥和口令只保存到本机钥匙串，不进入 AI 请求或 iCloud 同步。") .font(.footnote).foregroundStyle(.secondary) }
                } else {
                    Section("连接信息") {
                        LabeledContent("主机", value: host.displayName)
                        LabeledContent("地址", value: host.target)
                        LabeledContent("验证", value: host.authentication == .password ? "密码" : "SSH 私钥")
                        if !host.environment.isEmpty { LabeledContent("环境", value: host.environment) }
                    }
                    Section("可选跳板机") {
                        Picker("通过跳板连接", selection: $host.jumpHostID) {
                            Text("直接连接").tag(UUID?.none)
                            ForEach(app.data.hosts.filter { $0.id != host.id }) { h in Text(h.displayName).tag(Optional(h.id)) }
                        }
                    }
                    if existing {
                        Section("管理") {
                            if app.data.trustedKeys[host.endpoint] != nil { Button("清除这台主机的信任记录", role: .destructive) { forgetting = true } }
                            Button("删除主机与登录凭据", role: .destructive) { deleting = true }
                        }
                    }
                }
                if !error.isEmpty { Section { ErrorNotice(text: error) } }
            }
            .navigationTitle(existing ? "编辑主机" : "添加主机").navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("取消") { dismiss() } }
                ToolbarItem(placement: .topBarTrailing) { if editing != nil { Button("完成输入") { editing = nil } } }
            }
            .safeAreaInset(edge: .bottom) { HStack(spacing: 14) {
                if step > 0 { Button("上一步") { error = ""; step -= 1 }.frame(minHeight: 48) }
                Button(step == 2 ? "保存主机" : "下一步") { advance() }.buttonStyle(PrimaryButton()).accessibilityIdentifier("host-next")
            }.padding().background(.regularMaterial) }
            .task { guard !loaded else { return }; loaded = true; port = String(host.port); if existing, !app.demo { do { secret = try app.store.hostSecret(host.id) } catch { self.error = error.localizedDescription } } }
            .fileImporter(isPresented: $importing, allowedContentTypes: [.data, .plainText], allowsMultipleSelection: false) { result in
                do {
                    guard let url = try result.get().first else { return }
                    let accessing = url.startAccessingSecurityScopedResource(); defer { if accessing { url.stopAccessingSecurityScopedResource() } }
                    let bytes = try Data(contentsOf: url)
                    guard bytes.count <= 64 * 1024, let text = String(data: bytes, encoding: .utf8) else { throw AppFailure("私钥文件过大或不是文本。") }
                    secret.privateKey = text
                } catch { self.error = error.localizedDescription }
            }
            .confirmationDialog("清除保存的指纹？", isPresented: $forgetting, titleVisibility: .visible) {
                Button("清除信任记录", role: .destructive) { app.data.trustedKeys.removeValue(forKey: host.endpoint); app.persist() }
            } message: { Text("下次连接必须重新核对服务器指纹。请先从可信渠道确认变更。") }
            .confirmationDialog("删除这台主机？", isPresented: $deleting, titleVisibility: .visible) {
                Button("删除主机与登录凭据", role: .destructive) { do { try app.deleteHost(host); dismiss() } catch { self.error = error.localizedDescription } }
            } message: { Text("会保留本机 AI 历史记录。") }
        }.interactiveDismissDisabled()
    }
    private func advance() {
        editing = nil
        error = ""
        do {
            if step == 0 { host.port = Int(port) ?? 0; host = try host.validated(); step = 1 }
            else if step == 1 { if !app.demo { _ = try SSHConnection.authentication(host, secret) }; step = 2 }
            else { try app.saveHost(host, secret: secret); dismiss() }
        } catch { self.error = error.localizedDescription }
    }
}

struct SettingsView: View {
    @EnvironmentObject var app: AppState
    @State private var editingAI = false
    var body: some View {
        List {
            Section {
                HStack(spacing: 14) { Image(systemName: "terminal").font(.title).foregroundStyle(Color.accentColor); VStack(alignment: .leading, spacing: 4) { Text("WRD AI SSH").font(.headline); Text("独立 iPhone 工作台").font(.caption).foregroundStyle(.secondary) } }.padding(.vertical, 10)
            }
            Section("AI") {
                Button { editingAI = true } label: { HStack { Label("AI 服务", systemImage: "sparkles"); Spacer(); Text(app.data.settings.model.isEmpty ? "未配置" : app.data.settings.model).foregroundStyle(.secondary).lineLimit(1); Image(systemName: "chevron.right").font(.caption) } }.foregroundStyle(.primary).accessibilityIdentifier("ai-settings")
                Toggle("附带最近的终端输出", isOn: Binding(get: { app.data.settings.includeTerminal }, set: { app.data.settings.includeTerminal = $0; app.persist() }))
                Text("发送问题时，将最近最多 8,000 字符的终端输出直接发送给你配置的 AI 服务。关闭后仅发送对话内容。") .font(.footnote).foregroundStyle(.secondary)
            }
            Section("隐私") {
                Toggle("返回 App 时验证身份", isOn: Binding(get: { app.data.settings.lockOnBackground }, set: { app.data.settings.lockOnBackground = $0; app.persist() }))
                Text("使用 Face ID、Touch ID 或设备密码解锁。登录凭据保存在本机钥匙串；会话历史保存在受系统文件保护的本机目录。") .font(.footnote).foregroundStyle(.secondary)
            }
            Section("连接与后台") {
                Label("SSH 与 AI 均由手机直连", systemImage: "iphone.gen3.radiowaves.left.and.right")
                Text("切换页面不会断开连接。进入后台后，iOS 可能暂停或结束网络任务；后台时间用尽会保存会话并断开，回到前台后可手动重连。监控仅在前台采集。") .font(.footnote).foregroundStyle(.secondary)
            }
            Section { LabeledContent("版本", value: "0.1.0"); LabeledContent("终端与 SSH", value: "SwiftTerm · Citadel") }
        }.sheet(isPresented: $editingAI) { AISettingsEditor() }
    }
}

struct AISettingsEditor: View {
    @EnvironmentObject var app: AppState
    @Environment(\.dismiss) var dismiss
    @State private var settings = AISettings()
    @State private var key = ""
    @State private var error = ""
    @State private var loaded = false
    @State private var hasKey = false
    var body: some View {
        NavigationStack {
            Form {
                Section("兼容 Chat Completions 的服务") {
                    TextField("服务地址，如 https://api.example.com/v1", text: $settings.baseURL).keyboardType(.URL).textInputAutocapitalization(.never).autocorrectionDisabled().accessibilityIdentifier("ai-url")
                    TextField("模型名称", text: $settings.model).textInputAutocapitalization(.never).autocorrectionDisabled().accessibilityIdentifier("ai-model")
                    SecureField(hasKey ? "API Key（留空保留原值）" : "API Key", text: $key).textInputAutocapitalization(.never).autocorrectionDisabled().accessibilityIdentifier("ai-key")
                }
                Section {
                    Text("iPhone 直接通过 HTTPS 调用此服务。模型需支持流式回答和 function calling；AI 只能建议命令，执行始终由你在手机上确认。") .font(.footnote).foregroundStyle(.secondary)
                    if hasKey { Button("删除已保存的 API Key", role: .destructive) { do { try app.store.secrets.delete("ai:key"); hasKey = false; key = "" } catch { self.error = error.localizedDescription } } }
                }
                if !error.isEmpty { Section { ErrorNotice(text: error) } }
            }
            .navigationTitle("AI 服务").navigationBarTitleDisplayMode(.inline)
            .toolbar { ToolbarItem(placement: .cancellationAction) { Button("取消") { dismiss() } }; ToolbarItem(placement: .confirmationAction) { Button("保存") { save() }.accessibilityIdentifier("save-ai") } }
            .task { guard !loaded else { return }; loaded = true; settings = app.data.settings; do { hasKey = !(try app.store.aiKey()).isEmpty } catch { self.error = error.localizedDescription } }
        }.interactiveDismissDisabled()
    }
    private func save() {
        do {
            _ = try settings.endpoint()
            settings.model = settings.model.trimmingCharacters(in: .whitespacesAndNewlines)
            guard !settings.model.isEmpty else { throw AppFailure("请填写模型名称。") }
            if !key.isEmpty, !app.demo { try app.store.saveAIKey(key.trimmingCharacters(in: .whitespacesAndNewlines)) }
            app.data.settings = settings; app.persist(); dismiss()
        } catch { self.error = error.localizedDescription }
    }
}
