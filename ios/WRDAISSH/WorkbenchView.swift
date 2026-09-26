import SwiftUI

struct WorkbenchView: View {
    @EnvironmentObject var app: AppState
    @ObservedObject var session: PhoneSession
    @State private var tab = 0
    @State private var reviewing: Proposal?
    @State private var ending = false
    @State private var switching = false
    @State private var follow = true
    @FocusState private var composing: Bool
    var body: some View {
        VStack(spacing: 0) {
            HStack {
                VStack(alignment: .leading, spacing: 3) { Text(session.host.displayName).font(.subheadline.weight(.semibold)); Text(session.host.target).font(.caption).foregroundStyle(.secondary).lineLimit(1) }
                Spacer()
                if !session.host.environment.isEmpty { Badge(text: session.host.environment, color: session.host.environment.contains("生产") ? .orange : .accentColor) }
            }.padding(.horizontal, 16).padding(.vertical, 9)
            Picker("工作区", selection: $tab) { Text("AI 对话").tag(0); Text("监控").tag(1); Text("终端").tag(2) }.pickerStyle(.segmented).padding(.horizontal, 16).padding(.bottom, 12)
            if !session.error.isEmpty { ErrorNotice(text: session.error).padding(.horizontal, 16).padding(.bottom, 8) }
            if session.status != .running {
                HStack { Text(session.status.rawValue).font(.footnote).foregroundStyle(.secondary); Spacer(); if session.status == .connecting { ProgressView() } else { Button("连接服务器") { app.connect(session) }.disabled(app.demo) } }.padding(.horizontal, 16).padding(.bottom, 8)
            }
            if tab == 0 { chat }
            else if tab == 1 { MonitorPane(session: session) }
            else { TerminalPane(session: session) }
        }
        .background(Color(uiColor: .systemGroupedBackground))
        .navigationTitle(session.saved.title).navigationBarTitleDisplayMode(.inline)
        .toolbar { ToolbarItemGroup(placement: .topBarTrailing) {
            Button("切换会话", systemImage: "square.on.square") { switching = true }
            Menu { if session.status == .running || session.status == .connecting { Button("结束 SSH 连接", systemImage: "network.slash", role: .destructive) { ending = true } }; Button("返回最新消息", systemImage: "arrow.down") { follow = true; tab = 0 } } label: { Image(systemName: "ellipsis.circle") }.accessibilityLabel("会话操作")
        } }
        .sheet(item: $reviewing) { proposal in CommandReviewView(session: session, proposal: proposal) }
        .sheet(isPresented: $switching) { NavigationStack { List(app.sessions) { item in Button { switching = false; app.selectedSession = item.id } label: { VStack(alignment: .leading, spacing: 5) { Text(item.saved.title); Text(item.host.displayName + " · " + item.status.rawValue).font(.caption).foregroundStyle(.secondary) } } }.navigationTitle("切换会话").toolbar { ToolbarItem(placement: .cancellationAction) { Button("取消") { switching = false } } } } }
        .confirmationDialog("结束这条 SSH 连接？", isPresented: $ending, titleVisibility: .visible) { Button("结束连接", role: .destructive) { app.disconnect(session) } } message: { Text("会话历史会保留。已经发送的远端命令可能仍在运行，请核对状态。") }
        .onChange(of: tab) { _, _ in composing = false }
        .onDisappear { composing = false; session.invalidateReview() }
    }
    private var chat: some View {
        VStack(spacing: 0) {
            ScrollViewReader { proxy in
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 20) {
                        if session.saved.messages.isEmpty { EmptyState(title: "和 AI 一起处理问题", icon: "sparkles", description: "描述你要排查的问题。SSH 与 AI 独立连接，AI 服务不可用时仍可操作终端。") }
                        ForEach(session.saved.messages) { message in MessageRow(message: message) }
                        if session.busy { MessageRow(message: ChatMessage(role: "assistant", text: session.streaming.isEmpty ? "正在分析…" : session.streaming)) }
                        Color.clear.frame(height: 1).id("latest")
                    }.padding(18)
                }
                .scrollDismissesKeyboard(.interactively)
                .simultaneousGesture(DragGesture().onChanged { _ in follow = false })
                .onChange(of: session.streaming) { _, _ in if follow { proxy.scrollTo("latest", anchor: .bottom) } }
                .onChange(of: session.saved.messages.count) { _, _ in if follow { proxy.scrollTo("latest", anchor: .bottom) } }
                .onChange(of: follow) { _, value in if value { withAnimation { proxy.scrollTo("latest", anchor: .bottom) } } }
                .overlay(alignment: .bottomTrailing) { if !follow { Button("最新消息", systemImage: "arrow.down") { follow = true }.font(.caption).padding(10).background(.regularMaterial, in: Capsule()).padding(12) } }
            }
            if let proposal = session.proposal {
                Button { composing = false; reviewing = proposal } label: { HStack(spacing: 12) { Image(systemName: "checkmark.shield"); VStack(alignment: .leading, spacing: 4) { Text("查看命令并确认").font(.subheadline.weight(.semibold)); Text(proposal.explanation).font(.caption).lineLimit(2) }; Spacer(); Image(systemName: "chevron.right") }.padding(14).background(Color.accentColor.opacity(0.09), in: RoundedRectangle(cornerRadius: 14)) }.buttonStyle(.plain).padding(.horizontal, 16).padding(.bottom, 10).accessibilityIdentifier("review-command")
            }
            VStack(spacing: 8) {
                if session.executing { HStack { ProgressView(); Text("命令执行中，请等待结果").font(.footnote); Spacer(); Button("中止并断开") { ending = true }.font(.footnote) } }
                HStack(alignment: .bottom, spacing: 10) {
                    TextField("描述问题，或补充线索…", text: $session.draft, axis: .vertical).lineLimit(1...5).padding(12).background(Color(uiColor: .secondarySystemGroupedBackground), in: RoundedRectangle(cornerRadius: 13)).focused($composing).accessibilityIdentifier("ai-prompt")
                    if session.busy { Button("停止", systemImage: "stop.fill") { session.aiTask?.cancel() }.labelStyle(.iconOnly).frame(width: 48, height: 48) }
                    else { Button { follow = true; composing = false; app.send(session) } label: { Image(systemName: "arrow.up").font(.headline).frame(width: 48, height: 48).background(Color.accentColor, in: RoundedRectangle(cornerRadius: 14)).foregroundStyle(.white) }.accessibilityLabel("发送问题").disabled(session.draft.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || session.proposal != nil || session.executing) }
                }
                Text(app.data.settings.includeTerminal ? "发送时附带最近输出 · 回车换行" : "仅发送对话内容 · 回车换行").font(.caption2).foregroundStyle(.secondary).frame(maxWidth: .infinity, alignment: .leading)
            }.padding(.horizontal, 16).padding(.vertical, 10).background(.regularMaterial)
        }
    }
}

struct MessageRow: View {
    let message: ChatMessage
    var body: some View {
        HStack(alignment: .top, spacing: 10) {
            if message.role == "user" { Spacer(minLength: 28) }
            else { Image(systemName: message.role == "assistant" ? "sparkles" : "info.circle").font(.subheadline).foregroundStyle(Color.accentColor).frame(width: 26, height: 28) }
            VStack(alignment: .leading, spacing: 6) {
                if message.role != "user" { Text(message.role == "assistant" ? "AI 助手" : "会话记录").font(.caption).foregroundStyle(.secondary) }
                Text(.init(message.text)).font(.body).textSelection(.enabled).frame(maxWidth: .infinity, alignment: .leading)
            }.padding(message.role == "user" ? 14 : 0).background(message.role == "user" ? Color.accentColor.opacity(0.09) : .clear, in: RoundedRectangle(cornerRadius: 16))
        }
    }
}

struct MonitorPane: View {
    @EnvironmentObject var app: AppState
    @ObservedObject var session: PhoneSession
    var body: some View {
        ScrollView { VStack(alignment: .leading, spacing: 16) {
            Surface { VStack(alignment: .leading, spacing: 10) {
                Toggle("实时采集", isOn: Binding(get: { session.monitoring }, set: { app.setMonitoring(session, enabled: $0) })).disabled(session.status != .running)
                Text("手机前台运行时每 15 秒采集。固定只读命令读取系统负载、内存和磁盘；使用率达到 90% 时记录告警。") .font(.footnote).foregroundStyle(.secondary)
                HStack { if let snapshot = session.monitor { Text("更新于 " + snapshot.date.formatted(date: .omitted, time: .standard)).font(.caption).foregroundStyle(.secondary); if Date().timeIntervalSince(snapshot.date) > 45 || session.status != .running { Badge(text: "数据已过期", color: .orange) } }; Spacer(); Button("刷新", systemImage: "arrow.clockwise") { Task { await app.refreshMonitor(session) } }.disabled(session.collecting || session.status != .running) }
            } }
            ErrorNotice(text: session.monitorError)
            if let snapshot = session.monitor {
                HStack(alignment: .top, spacing: 12) { metric("内存", value: snapshot.memoryPercent); metric("磁盘", value: snapshot.diskPercent) }
                Surface { VStack(alignment: .leading, spacing: 10) { Label("系统与负载", systemImage: "waveform.path.ecg").font(.headline); Text(snapshot.system); Text(snapshot.uptime).font(.system(.callout, design: .monospaced)).textSelection(.enabled) } }
                DisclosureGroup("查看完整采集结果") { Text(snapshot.raw.isEmpty ? snapshot.diskText : snapshot.raw).font(.system(.caption, design: .monospaced)).textSelection(.enabled).frame(maxWidth: .infinity, alignment: .leading) }.padding().background(Color(uiColor: .secondarySystemGroupedBackground), in: RoundedRectangle(cornerRadius: 16))
                Button("让 AI 分析当前监控") { session.draft = "请分析刚采集的监控数据：\n" + snapshot.raw; app.send(session) }.buttonStyle(PrimaryButton()).disabled(session.busy || session.executing || session.proposal != nil)
            } else { EmptyState(title: "按需查看服务器状态", icon: "waveform.path.ecg", description: "点击刷新，或开启实时采集。此页面不会自动假设服务器健康。") }
        }.padding(16) }
    }
    private func metric(_ title: String, value: Double?) -> some View { Surface { VStack(alignment: .leading, spacing: 10) { Text(title).font(.subheadline).foregroundStyle(.secondary); Text(value.map { "\(Int($0))%" } ?? "—").font(.system(.title, design: .rounded).weight(.semibold)); if let value { ProgressView(value: value, total: 100).tint(value >= 90 ? .orange : .accentColor) } else { Text("此系统未提供").font(.caption).foregroundStyle(.secondary) } } } }
}

struct CommandReviewView: View {
    @EnvironmentObject var app: AppState
    @Environment(\.dismiss) var dismiss
    @ObservedObject var session: PhoneSession
    let proposal: Proposal
    @State private var stage = 0
    @State private var confirmedHost = ""
    @State private var checked = false
    @State private var revision = UUID()
    private var valid: Bool { app.foreground && !app.locked && session.status == .running && session.proposal == proposal && session.generation == proposal.generation && session.reviewRevision == revision && !session.busy && !session.executing }
    var body: some View {
        NavigationStack {
            ScrollView { VStack(alignment: .leading, spacing: 18) {
                Badge(text: proposal.needsExtraConfirmation ? "需要额外确认" : "只读操作", color: proposal.needsExtraConfirmation ? .orange : .accentColor)
                Text(proposal.explanation).font(.title3.weight(.semibold))
                Surface { VStack(spacing: 12) { LabeledContent("执行主机", value: session.host.displayName); LabeledContent("环境", value: session.host.environment.isEmpty ? "未标注" : session.host.environment); LabeledContent("目标", value: session.host.target); LabeledContent("方式", value: "独立 SSH 命令通道") }.font(.callout) }
                if !proposal.expectedEffect.isEmpty { Text("预期效果：" + proposal.expectedEffect).font(.callout).foregroundStyle(.secondary) }
                HStack { Text("完整命令").font(.headline); Spacer(); Button("复制", systemImage: "doc.on.doc") { UIPasteboard.general.string = proposal.command }.font(.caption) }
                Text(proposal.command).font(.system(.callout, design: .monospaced)).textSelection(.enabled).frame(maxWidth: .infinity, alignment: .leading).padding(16).background(Color(uiColor: .secondarySystemGroupedBackground), in: RoundedRectangle(cornerRadius: 14)).accessibilityIdentifier("complete-command")
                Text("命令在新通道中执行，不继承交互终端里的目录与临时环境变量。") .font(.footnote).foregroundStyle(.secondary)
                if stage == 1 {
                    Toggle("我已核对目标与完整命令", isOn: $checked)
                    TextField("输入主机名称：" + session.host.displayName, text: $confirmedHost).textInputAutocapitalization(.never).autocorrectionDisabled().textFieldStyle(.roundedBorder).accessibilityIdentifier("confirm-host")
                }
                if !valid { ErrorNotice(text: session.status != .running ? "SSH 未连接，不能执行。请关闭此页并连接服务器。" : "命令或连接状态已变化，请重新打开确认页。") }
            }.padding(18) }
            .background(Color(uiColor: .systemGroupedBackground))
            .navigationTitle(stage == 0 ? "确认操作" : "再次核对执行目标").navigationBarTitleDisplayMode(.inline)
            .toolbar { ToolbarItem(placement: .cancellationAction) { Button("稍后处理") { dismiss() } } }
            .safeAreaInset(edge: .bottom) { VStack(spacing: 10) {
                Button(stage == 0 && proposal.needsExtraConfirmation ? "继续确认" : "批准执行一次") {
                    if stage == 0 && proposal.needsExtraConfirmation { stage = 1 }
                    else { app.approve(session, proposal: proposal, reviewRevision: revision); dismiss() }
                }.buttonStyle(PrimaryButton()).disabled(!valid || (stage == 1 && (!checked || confirmedHost != session.host.displayName))).accessibilityIdentifier("approve-command")
                Button("拒绝本次操作", role: .destructive) { app.reject(session, proposal: proposal); dismiss() }.frame(maxWidth: .infinity, minHeight: 44)
            }.padding().background(.regularMaterial) }
        }
        .onAppear { revision = session.reviewRevision }
        .onChange(of: session.reviewRevision) { _, _ in checked = false; confirmedHost = "" }
        .interactiveDismissDisabled()
    }
}
