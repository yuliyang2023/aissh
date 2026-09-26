import SwiftUI

struct PrimaryButton: ButtonStyle {
    @Environment(\.isEnabled) var enabled
    func makeBody(configuration: Configuration) -> some View {
        configuration.label.font(.body.weight(.semibold)).frame(maxWidth: .infinity, minHeight: 48)
            .foregroundStyle(.white).background(Color.accentColor.opacity(enabled ? (configuration.isPressed ? 0.7 : 1) : 0.35), in: RoundedRectangle(cornerRadius: 14))
    }
}
struct Surface<Content: View>: View {
    @ViewBuilder var content: Content
    var body: some View { content.padding(16).frame(maxWidth: .infinity, alignment: .leading).background(Color(uiColor: .secondarySystemGroupedBackground), in: RoundedRectangle(cornerRadius: 18)) }
}
struct ErrorNotice: View {
    let text: String
    var body: some View { if !text.isEmpty { Label(text, systemImage: "exclamationmark.circle").font(.footnote).foregroundStyle(.red).frame(maxWidth: .infinity, alignment: .leading).padding(12).background(.red.opacity(0.07), in: RoundedRectangle(cornerRadius: 12)).textSelection(.enabled) } }
}
struct Badge: View {
    let text: String
    var color = Color.accentColor
    var body: some View { Text(text).font(.caption.weight(.medium)).padding(.horizontal, 8).padding(.vertical, 4).foregroundStyle(color).background(color.opacity(0.1), in: RoundedRectangle(cornerRadius: 7)) }
}
struct EmptyState: View {
    let title: String
    let icon: String
    let description: String
    var body: some View { ContentUnavailableView(title, systemImage: icon, description: Text(description)) }
}
struct TrustSheet: View {
    @EnvironmentObject var app: AppState
    let issue: HostTrustRequired
    var body: some View {
        NavigationStack {
            ScrollView { VStack(alignment: .leading, spacing: 20) {
                Image(systemName: issue.changed ? "exclamationmark.shield" : "checkmark.shield").font(.largeTitle).foregroundStyle(issue.changed ? .red : Color.accentColor)
                Text(issue.host.displayName).font(.title2.bold())
                Text(issue.host.target).font(.callout).foregroundStyle(.secondary)
                Text(issue.changed ? "这台主机的公钥与手机保存的记录不同。连接已阻止，请先通过可信渠道核实变更。" : "请通过可信渠道核对服务器指纹。确认后才会进行身份验证。")
                Text(issue.fingerprint).font(.system(.callout, design: .monospaced)).textSelection(.enabled).padding().frame(maxWidth: .infinity, alignment: .leading).background(Color(uiColor: .secondarySystemGroupedBackground), in: RoundedRectangle(cornerRadius: 12))
                if issue.changed { Text("核实后，可在主机编辑页清除旧信任记录，再重新连接。") .font(.footnote).foregroundStyle(.secondary) }
            }.padding() }
            .navigationTitle(issue.changed ? "主机指纹已变化" : "确认主机指纹").navigationBarTitleDisplayMode(.inline)
            .toolbar { ToolbarItem(placement: .cancellationAction) { Button("取消") { app.trust = nil; app.trustSessionID = nil } } }
            .safeAreaInset(edge: .bottom) { if !issue.changed { Button("指纹一致，信任并连接") { app.trustHost(issue) }.buttonStyle(PrimaryButton()).padding().background(.regularMaterial) } }
        }.interactiveDismissDisabled()
    }
}
