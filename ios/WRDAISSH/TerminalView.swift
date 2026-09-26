import SwiftUI
import SwiftTerm

final class SafeTerminalView: TerminalView {
    var onPaste: ((String) -> Void)?
    override func paste(_ sender: Any?) {
        guard let text = UIPasteboard.general.string, !text.isEmpty else { return }
        onPaste?(text)
    }
}

@MainActor
final class TerminalController: NSObject, ObservableObject, @preconcurrency TerminalViewDelegate {
    let view: SafeTerminalView
    weak var session: PhoneSession?
    @Published var pasteText: String?
    @Published var scrolledBack = false
    init(session: PhoneSession) {
        self.session = session
        view = SafeTerminalView(frame: .zero, font: .monospacedSystemFont(ofSize: 13, weight: .regular))
        super.init()
        view.terminalDelegate = self
        view.nativeBackgroundColor = UIColor(red: 0.065, green: 0.085, blue: 0.12, alpha: 1)
        view.nativeForegroundColor = UIColor(red: 0.87, green: 0.9, blue: 0.96, alpha: 1)
        view.accessibilityLabel = "SSH 交互终端"
        view.onPaste = { [weak self] text in self?.pasteText = text }
        if !session.terminalData.isEmpty { view.feed(byteArray: Array(session.terminalData)[...]) }
        session.terminalSink = { [weak self] data in self?.view.feed(byteArray: Array(data)[...]) }
    }
    func send(source: TerminalView, data: ArraySlice<UInt8>) { session?.connection?.write(Data(data)) }
    func sizeChanged(source: TerminalView, newCols: Int, newRows: Int) { session?.connection?.resize(cols: newCols, rows: newRows) }
    func setTerminalTitle(source: TerminalView, title: String) {}
    func hostCurrentDirectoryUpdate(source: TerminalView, directory: String?) {}
    func scrolled(source: TerminalView, position: Double) { scrolledBack = position < 0.99 }
    func requestOpenLink(source: TerminalView, link: String, params: [String: String]) {}
    func bell(source: TerminalView) { UIImpactFeedbackGenerator(style: .light).impactOccurred() }
    func clipboardCopy(source: TerminalView, content: Data) {} // Remote OSC 52 does not modify the phone clipboard.
    func clipboardRead(source: TerminalView) -> Data? { nil }
    func rangeChanged(source: TerminalView, startY: Int, endY: Int) {}
    func key(_ value: String) { view.send(txt: value); _ = view.becomeFirstResponder() }
    func sendPaste() {
        guard let text = pasteText else { return }
        // A trailing newline in pasted content must not silently execute it.
        let safe = text.trimmingCharacters(in: .newlines)
        view.send(txt: safe); pasteText = nil; _ = view.becomeFirstResponder()
    }
}

struct TerminalHost: UIViewRepresentable {
    @ObservedObject var controller: TerminalController
    func makeUIView(context: Context) -> SafeTerminalView { controller.view }
    func updateUIView(_ view: SafeTerminalView, context: Context) {}
}

struct TerminalPane: View {
    @ObservedObject var session: PhoneSession
    @ObservedObject var controller: TerminalController
    init(session: PhoneSession) { self.session = session; controller = session.terminal }
    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Label(session.status.rawValue, systemImage: session.status == .running ? "circle.fill" : "circle")
                    .foregroundStyle(session.status == .running ? .green : .secondary)
                Spacer()
                Button("键盘", systemImage: "keyboard") { _ = controller.view.becomeFirstResponder() }
                Button("收起", systemImage: "keyboard.chevron.compact.down") { _ = controller.view.resignFirstResponder() }
            }.font(.caption).padding(12)
            TerminalHost(controller: controller).accessibilityIdentifier("ssh-terminal")
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 8) {
                    ForEach(Array(zip(["Esc", "Tab", "Ctrl+C", "↑", "↓", "←", "→"], ["\u{1b}", "\t", "\u{03}", "\u{1b}[A", "\u{1b}[B", "\u{1b}[D", "\u{1b}[C"])), id: \.0) { label, code in
                        Button(label) { controller.key(code) }.font(.system(.callout, design: .monospaced)).frame(minWidth: 44, minHeight: 44).background(Color(uiColor: .tertiarySystemFill), in: RoundedRectangle(cornerRadius: 9))
                    }
                    Button("粘贴", systemImage: "doc.on.clipboard") { controller.view.paste(nil) }.frame(minHeight: 44)
                }.padding(10)
            }.disabled(session.status != .running)
        }
        .sheet(isPresented: Binding(get: { controller.pasteText != nil }, set: { if !$0 { controller.pasteText = nil } })) {
            NavigationStack {
                ScrollView { Text(controller.pasteText ?? "").font(.system(.body, design: .monospaced)).textSelection(.enabled).frame(maxWidth: .infinity, alignment: .leading).padding() }
                    .navigationTitle("确认粘贴内容").navigationBarTitleDisplayMode(.inline)
                    .safeAreaInset(edge: .bottom) { VStack(alignment: .leading, spacing: 12) {
                        Text("多行内容可能包含会执行的命令。确认后发送到当前终端，移除末尾回车。") .font(.footnote).foregroundStyle(.secondary)
                        Button("发送到 \(session.host.displayName)") { controller.sendPaste() }.buttonStyle(PrimaryButton()).disabled(session.status != .running)
                    }.padding().background(.regularMaterial) }
                    .toolbar { ToolbarItem(placement: .cancellationAction) { Button("取消") { controller.pasteText = nil } } }
            }
        }
        .onDisappear { _ = controller.view.resignFirstResponder() }
    }
}
