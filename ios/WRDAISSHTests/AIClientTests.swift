import XCTest
@testable import WRDAISSH

final class AIStub: URLProtocol {
    static var handler: ((URLRequest) throws -> (Int, String, Data))?
    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }
    override func startLoading() {
        do {
            guard let handler = Self.handler else { throw AppFailure("Missing test handler") }
            let (status, type, data) = try handler(request)
            let response = HTTPURLResponse(url: request.url!, statusCode: status, httpVersion: "HTTP/1.1", headerFields: ["Content-Type": type])!
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            client?.urlProtocol(self, didLoad: data); client?.urlProtocolDidFinishLoading(self)
        } catch { client?.urlProtocol(self, didFailWithError: error) }
    }
    override func stopLoading() {}
}

final class AIClientTests: XCTestCase {
    override func tearDown() { AIStub.handler = nil; super.tearDown() }
    func testDirectStreamingRequestAndSecretRedaction() async throws {
        let configuration = URLSessionConfiguration.ephemeral; configuration.protocolClasses = [AIStub.self]
        AIStub.handler = { request in
            XCTAssertEqual(request.url?.absoluteString, "https://model.example/v1/chat/completions")
            XCTAssertEqual(request.value(forHTTPHeaderField: "Authorization"), "Bearer test-api-key")
            var body = request.httpBody ?? Data()
            if let stream = request.httpBodyStream {
                stream.open(); defer { stream.close() }; var bytes = [UInt8](repeating: 0, count: 4096)
                while stream.hasBytesAvailable { let n = stream.read(&bytes, maxLength: bytes.count); if n <= 0 { break }; body.append(contentsOf: bytes.prefix(n)) }
            }
            let text = String(decoding: body, as: UTF8.self)
            XCTAssertFalse(text.contains("ssh-secret")); XCTAssertFalse(text.contains("test-api-key"))
            XCTAssertTrue(text.contains("propose_command")); XCTAssertTrue(text.contains("stream"))
            let response = "data: {\"choices\":[{\"delta\":{\"content\":\"正在分析\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"完成\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
            return (200, "text/event-stream", Data(response.utf8))
        }
        let result = try await AIClient(configuration: configuration).ask(settings: AISettings(baseURL: "https://model.example/v1", model: "test-model"), key: "test-api-key", host: Host(address: "server", username: "ops"), messages: [ChatMessage(role: "user", text: "ssh-secret")], terminal: "test-api-key ssh-secret", secrets: ["ssh-secret"]) { _ in }
        XCTAssertEqual(result.text, "正在分析完成"); XCTAssertNil(result.command)
    }
    func testProviderErrorBodyNeverLeaksToken() async throws {
        let configuration = URLSessionConfiguration.ephemeral; configuration.protocolClasses = [AIStub.self]
        AIStub.handler = { _ in (401, "application/json", Data("secret-error-body".utf8)) }
        do {
            _ = try await AIClient(configuration: configuration).ask(settings: AISettings(baseURL: "https://model.example", model: "model"), key: "secret", host: Host(), messages: [], terminal: "", secrets: []) { _ in }
            XCTFail("expected failure")
        } catch { XCTAssertTrue(error.localizedDescription.contains("401")); XCTAssertFalse(error.localizedDescription.contains("secret-error-body")) }
    }
}
