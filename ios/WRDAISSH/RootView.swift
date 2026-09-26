import SwiftUI

struct RootView: View {
    @EnvironmentObject var app: AppState
    @State private var path: [UUID] = []
    @State private var addingHost = false
    @State private var choosingHost = false
    @State private var editingHost: Host?
    @State private var search = ""
    var body: some View {
        NavigationStack(path: $path) {
            TabView(selection: $app.selectedTab) {
                sessionList.tabItem { Label("会话", systemImage: "bubble.left.and.bubble.right") }.tag(0)
                alerts.tabItem { Label("告警", systemImage: "bell") }.badge(app.data.incidents.filter { !$0.acknowledged }.count).tag(1)
                hosts.tabItem { Label("主机", systemImage: "server.rack") }.tag(2)
                SettingsView().tabItem { Label("设置", systemImage: "gearshape") }.tag(3)
            }
            .navigationTitle(["会话", "告警", "主机", "设置"][app.selectedTab])
            .toolbar {
                if app.selectedTab == 0 || app.selectedTab == 2 {
                    ToolbarItem(placement: .topBarTrailing) { Button("新建", systemImage: "plus") { if app.selectedTab == 2 { addingHost = true } else { choosingHost = true } }.accessibilityIdentifier("new-item") }
                }
            }
            .navigationDestination(for: UUID.self) { id in
                if let session = app.sessions.first(where: { $0.id == id }) { WorkbenchView(session: session).toolbar(.hidden, for: .tabBar) }
                else { EmptyState(title: "会话已移除", icon: "bubble.left", description: "请返回列表选择其他会话。") }
            }
        }
        .onChange(of: app.selectedSession) { _, id in if let id, path.last != id { path = [id] } }
        .onChange(of: path) { _, newPath in if newPath.isEmpty { app.selectedSession = nil } }
        .sheet(isPresented: $addingHost) { HostEditor(host: Host()) }
        .sheet(item: $editingHost) { host in HostEditor(host: host) }
        .sheet(isPresented: $choosingHost) { NavigationStack { List {
            if app.data.hosts.isEmpty { EmptyState(title: "先添加一台主机", icon: "server.rack", description: "登录信息仅保存在本机钥匙串。") }
            ForEach(app.data.hosts) { host in Button { choosingHost = false; app.newSession(host) } label: { HostRow(host: host) } }
            Button("添加主机", systemImage: "plus") { choosingHost = false; app.selectedTab = 2; addingHost = true }
        }.navigationTitle("连接到哪台主机").toolbar { ToolbarItem(placement: .cancellationAction) { Button("取消") { choosingHost = false } } } } }
        .sheet(item: $app.trust) { issue in TrustSheet(issue: issue) }
        .alert("操作未完成", isPresented: Binding(get: { !app.error.isEmpty }, set: { if !$0 { app.error = "" } })) { Button("知道了") { app.error = "" } } message: { Text(app.error) }
    }
    private var sessionList: some View {
        ScrollView {
            LazyVStack(spacing: 14) {
                if app.demo { Badge(text: "交互演示 · 不连接真实服务器") }
                if let pending = app.sessions.first(where: { $0.proposal != nil }) {
                    Button { app.selectedSession = pending.id } label: {
                        HStack(spacing: 12) { Image(systemName: "checkmark.shield"); VStack(alignment: .leading, spacing: 4) { Text("有命令等待你确认").font(.subheadline.weight(.medium)); Text(pending.host.displayName).font(.caption) }; Spacer(); Image(systemName: "chevron.right").font(.caption) }.padding(16).background(Color.accentColor.opacity(0.09), in: RoundedRectangle(cornerRadius: 16))
                    }.buttonStyle(.plain).accessibilityIdentifier("pending-command")
                }
                if app.sessions.isEmpty {
                    EmptyState(title: "把工作台带在身边", icon: "terminal", description: "iPhone 直接连接服务器。选择主机，随时开始排障。")
                    Button("开始一次连接") { choosingHost = true }.buttonStyle(PrimaryButton())
                } else {
                    ForEach(app.sessions.filter { search.isEmpty || ($0.saved.title + $0.host.displayName + $0.host.address).localizedCaseInsensitiveContains(search) }) { session in
                        Button { app.selectedSession = session.id } label: { SessionCard(session: session) }.buttonStyle(.plain)
                    }
                }
            }.padding(18)
        }.background(Color(uiColor: .systemGroupedBackground)).searchable(text: $search, prompt: "搜索会话或主机")
    }
    private var hosts: some View {
        List {
            if app.data.hosts.isEmpty { EmptyState(title: "还没有主机", icon: "server.rack", description: "添加地址和登录凭据，手机即可独立连接 SSH。") }
            ForEach(app.data.hosts) { host in
                Button { app.newSession(host) } label: { HostRow(host: host) }.buttonStyle(.plain)
                    .swipeActions(edge: .trailing, allowsFullSwipe: false) { Button("编辑") { editingHost = host }.tint(.accentColor) }
                    .contextMenu { Button("连接") { app.newSession(host) }; Button("编辑主机") { editingHost = host } }
            }
            if !app.data.hosts.isEmpty { Section { Text("点击主机连接；向左轻扫可编辑。SSH 密码与私钥保存在 iPhone 钥匙串。") .font(.footnote).foregroundStyle(.secondary) } }
        }
    }
    private var alerts: some View {
        ScrollView { LazyVStack(spacing: 14) {
            Text("手机前台运行监控时采集；后台不会持续监测服务器。") .font(.footnote).foregroundStyle(.secondary).frame(maxWidth: .infinity, alignment: .leading)
            if app.data.incidents.isEmpty { EmptyState(title: "暂无告警记录", icon: "checkmark.shield", description: "进入会话的监控页，开启实时采集与阈值提醒。") }
            ForEach(app.data.incidents) { incident in Surface {
                VStack(alignment: .leading, spacing: 12) {
                    HStack { Badge(text: incident.acknowledged ? "已读" : "待处理", color: incident.acknowledged ? .secondary : .orange); Spacer(); Text(incident.date, style: .relative).font(.caption).foregroundStyle(.secondary) }
                    Text(incident.title).font(.headline)
                    Text(incident.hostName).font(.subheadline).foregroundStyle(.secondary)
                    Text(incident.detail).font(.callout)
                    Button("带上告警，让 AI 排查") { if let session = app.sessions.first(where: { $0.id == incident.sessionID }) { session.draft = "请分析这条告警：\(incident.title)；\(incident.detail)；发生于 \(incident.date.formatted())。"; app.selectedSession = session.id } }.buttonStyle(PrimaryButton())
                    if !incident.acknowledged { Button("标为已读") { if let i = app.data.incidents.firstIndex(where: { $0.id == incident.id }) { app.data.incidents[i].acknowledged = true; app.persist() } }.frame(maxWidth: .infinity, minHeight: 44) }
                }
            } }
        }.padding(18) }.background(Color(uiColor: .systemGroupedBackground))
    }
}

struct HostRow: View {
    let host: Host
    var body: some View { HStack(spacing: 14) {
        Image(systemName: "server.rack").font(.title3).foregroundStyle(Color.accentColor).frame(width: 42, height: 42).background(Color.accentColor.opacity(0.1), in: RoundedRectangle(cornerRadius: 12))
        VStack(alignment: .leading, spacing: 5) { Text(host.displayName).font(.headline); Text(host.target).font(.caption).foregroundStyle(.secondary); if !host.environment.isEmpty { Text(host.environment).font(.caption).foregroundStyle(.secondary) } }
        Spacer(); Image(systemName: "chevron.right").font(.caption).foregroundStyle(.tertiary)
    }.padding(.vertical, 6) }
}
struct SessionCard: View {
    @ObservedObject var session: PhoneSession
    var body: some View { Surface { VStack(alignment: .leading, spacing: 15) {
        HStack(spacing: 12) { Image(systemName: "bubble.left.and.bubble.right").font(.title3).foregroundStyle(Color.accentColor).frame(width: 42, height: 42).background(Color.accentColor.opacity(0.1), in: RoundedRectangle(cornerRadius: 12)); VStack(alignment: .leading, spacing: 5) { Text(session.saved.title).font(.headline).lineLimit(2); Text(session.host.displayName + (session.host.environment.isEmpty ? "" : " · " + session.host.environment)).font(.caption).foregroundStyle(.secondary) }; Spacer(minLength: 0) }
        HStack { Text(session.saved.updatedAt, style: .relative).font(.caption).foregroundStyle(.secondary); Spacer(); Badge(text: session.executing ? "执行中" : session.busy ? "AI 处理中" : session.proposal != nil ? "等待确认" : session.status.rawValue, color: session.proposal != nil ? .orange : session.status == .running ? .green : .secondary) }
    } } }
}
