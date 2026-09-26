import SwiftUI

@main
struct WRDAISSHApp: App {
    @StateObject private var state: AppState
    @Environment(\.scenePhase) private var scenePhase
    init() {
        #if DEBUG
        let demo = ProcessInfo.processInfo.arguments.contains("--demo")
        let testing = ProcessInfo.processInfo.arguments.contains("--ui-testing")
        let folder = testing || demo ? FileManager.default.temporaryDirectory.appendingPathComponent("wrd-ui-tests-" + UUID().uuidString) : nil
        _state = StateObject(wrappedValue: AppState(store: LocalStore(directory: folder, secrets: KeychainStore(service: testing || demo ? "com.wrdtech.aissh.ios.uitests" : "com.wrdtech.aissh.ios.credentials")), demo: demo))
        #else
        _state = StateObject(wrappedValue: AppState())
        #endif
    }
    var body: some Scene {
        WindowGroup {
            ZStack {
                if state.locked {
                    VStack(spacing: 22) { Image(systemName: "lock.shield").font(.system(size: 44)).foregroundStyle(Color.accentColor); Text("解锁你的工作台").font(.title2.bold()); Button("验证身份") { Task { await state.unlock() } }.buttonStyle(PrimaryButton()); if !state.error.isEmpty { ErrorNotice(text: state.error) } }.padding(30)
                } else { RootView().environmentObject(state) }
                if scenePhase != .active { Color(uiColor: .systemBackground).ignoresSafeArea().overlay { Label("WRD AI SSH", systemImage: "lock.shield").font(.title2) } }
            }
            .onChange(of: scenePhase) { _, phase in state.sceneChanged(phase) }
        }
    }
}
