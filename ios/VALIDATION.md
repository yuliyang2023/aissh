# iOS 首版验证记录

日期：2026-09-26。环境：Apple Silicon Mac、Xcode 26.6（17F113）、Swift 6.3.3、iPhone 17 Pro 模拟器（iOS 26.5）。工程最低系统版本为 iOS 17；本轮没有 iOS 17 运行时实测。

| 项目 | 结果 |
| --- | --- |
| 模拟器 Debug 构建 | 通过，使用本机临时签名以验证真实 Keychain 访问 |
| 单元与 SSH 集成测试 | 16 项通过，0 失败，0 跳过 |
| 原生 UI 测试 | 2 项通过，0 失败 |
| iPhone Release 编译 | 通过，生成 ARM64 Mach-O；未使用设备分发签名 |
| 桌面回归 | `go test -tags desktop ./internal/desktop ./cmd/aissh-desktop ./scripts/ios-ssh-fixture` 通过 |
| 界面检查 | 已查看 AI 对话、命令确认、监控和终端的模拟器截图；终端旋转与输入草稿保留通过 UI 测试 |

测试入口：

```sh
bash scripts/test-ios.sh

xcodebuild -project ios/WRDAISSH.xcodeproj -scheme WRDAISSH \
  -configuration Release -destination 'generic/platform=iOS' \
  -derivedDataPath .cache/ios-device-build \
  -clonedSourcePackagesDirPath .cache/ios-packages \
  -disableAutomaticPackageResolution -skipPackageUpdates \
  -skipPackagePluginValidation CODE_SIGNING_ALLOWED=NO build
```

SSH 集成测试通过手机客户端协议栈连接本机 SSH fixture，覆盖首次指纹、变更指纹拒绝、PTY 字节收发、成功/失败退出码、缺失完成标记、超时、批准记录提前保存和重复批准只发送一次。Fixture 不执行 Shell，也不连接生产系统。

AI 使用 URLProtocol 响应模拟验证直接请求的 URL、认证头、流式解析和凭据替换；另有工具提案完整性与风险分类测试。本轮没有使用真实供应商 API Key，也未验证各供应商的模型兼容性、额度与计费。

原生 UI 测试覆盖分步添加主机、AI HTTPS 校验、待确认入口、断线禁用执行、草稿保留、监控页面和终端横竖屏。截图使用明确标注的离线演示数据，见 [界面预览](../docs/mobile/ios-native-preview.png)。

尚未完成：实体 iPhone 安装和真实网络验收、蜂窝网络切换、长时间后台恢复、真实私钥服务器与跳板链互通、真实模型联调、TestFlight / App Store 分发。开发者签名与安装方法见 [README](README.md)。
