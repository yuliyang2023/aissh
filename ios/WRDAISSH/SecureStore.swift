import Foundation
import Security

protocol SecretStorage {
    func read(_ account: String) throws -> Data?
    func write(_ account: String, data: Data) throws
    func delete(_ account: String) throws
}

final class KeychainStore: SecretStorage {
    let service: String
    init(service: String = "com.wrdtech.aissh.ios.credentials") { self.service = service }
    private func query(_ account: String) -> [String: Any] {
        [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: service,
         kSecAttrAccount as String: account, kSecAttrSynchronizable as String: false]
    }
    func read(_ account: String) throws -> Data? {
        var q = query(account)
        q[kSecReturnData as String] = true
        q[kSecMatchLimit as String] = kSecMatchLimitOne
        var result: CFTypeRef?
        let status = SecItemCopyMatching(q as CFDictionary, &result)
        if status == errSecItemNotFound { return nil }
        guard status == errSecSuccess else { throw failure(status) }
        return result as? Data
    }
    func write(_ account: String, data: Data) throws {
        let attributes: [String: Any] = [kSecValueData as String: data,
            kSecAttrAccessible as String: kSecAttrAccessibleWhenUnlockedThisDeviceOnly]
        let status = SecItemUpdate(query(account) as CFDictionary, attributes as CFDictionary)
        if status == errSecItemNotFound {
            var q = query(account); attributes.forEach { q[$0.key] = $0.value }
            let added = SecItemAdd(q as CFDictionary, nil)
            guard added == errSecSuccess else { throw failure(added) }
        } else if status != errSecSuccess { throw failure(status) }
    }
    func delete(_ account: String) throws {
        let status = SecItemDelete(query(account) as CFDictionary)
        guard status == errSecSuccess || status == errSecItemNotFound else { throw failure(status) }
    }
    private func failure(_ status: OSStatus) -> AppFailure {
        AppFailure("无法访问本机钥匙串（\(status)）。请解锁 iPhone 后重试。")
    }
}

final class LocalStore {
    let directory: URL
    let secrets: SecretStorage
    init(directory: URL? = nil, secrets: SecretStorage = KeychainStore()) {
        self.directory = directory ?? FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0].appendingPathComponent("WRDAISSH", isDirectory: true)
        self.secrets = secrets
    }
    func load() throws -> LocalData {
        let path = directory.appendingPathComponent("workspace.json")
        guard FileManager.default.fileExists(atPath: path.path) else { return LocalData() }
        let data = try JSONDecoder().decode(LocalData.self, from: Data(contentsOf: path))
        guard data.version == 1 else { throw AppFailure("本地数据版本不兼容，请更新客户端。") }
        return data
    }
    func save(_ data: LocalData) throws {
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true, attributes: [.protectionKey: FileProtectionType.complete])
        let path = directory.appendingPathComponent("workspace.json")
        try JSONEncoder().encode(data).write(to: path, options: [.atomic, .completeFileProtection])
        var dir = directory
        var values = URLResourceValues(); values.isExcludedFromBackup = true
        try dir.setResourceValues(values)
    }
    func hostSecret(_ id: UUID) throws -> HostSecret {
        guard let data = try secrets.read("host:" + id.uuidString) else { return HostSecret() }
        return try JSONDecoder().decode(HostSecret.self, from: data)
    }
    func saveSecret(_ secret: HostSecret, id: UUID) throws {
        try secrets.write("host:" + id.uuidString, data: JSONEncoder().encode(secret))
    }
    func aiKey() throws -> String { String(data: try secrets.read("ai:key") ?? Data(), encoding: .utf8) ?? "" }
    func saveAIKey(_ key: String) throws { try secrets.write("ai:key", data: Data(key.utf8)) }
}
