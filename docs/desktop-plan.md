# Desktop and credentials implementation

The desktop app and CLI share a local catalog and the operating system's secret
store. The catalog contains hosts, credential metadata, service connections and
AI profiles. Secret values are never serialized into the catalog.

## Delivery

- [x] Transactional catalog and OS keyring, reference validation, secret rotation.
- [x] Managed SSH profiles, scoped askpass, host-key verification and jump hosts.
- [x] Desktop host/credential/resource/AI forms with validation and connection tests.
- [x] Embedded aissh sessions, tabs, resize, disconnect and process cleanup.
- [x] Bound resource execution, local/SSH routes, explicit grants, sanitized results.
- [x] AI provider credentials and resource operation proposals.
- [x] Regression/security tests, desktop build, UI verification and documentation.

## Boundaries

AI receives resource descriptions and proposes structured operations. Only the
trusted executor resolves secrets. Resource destinations come from saved
configuration. A model-generated arbitrary shell command never receives secrets.
Approval of an operation does not grant access to unrelated credentials.

SSH host keys must be verified before stored credentials can be used. Passwords
are passed through askpass's private response channel, never terminal input or
process arguments. Unknown authentication prompts do not receive saved passwords.
Private keys remain in their existing files or SSH agent.

An unavailable or locked OS keyring produces an actionable error; it never causes
a plaintext fallback. Configuration uses atomic writes and a cross-process lock.
GUI save requests can contain secrets but GUI read methods return metadata only.

Resource adapters enforce destinations, methods and server-side account privileges.
Remote accounts and administrators remain trusted when a secret is used remotely.
Local same-user compromise is outside the protection provided by an OS keyring.

The desktop embeds the existing terminal workflow via a PTY. This preserves the
existing AI conversation, monitors and command approval behavior while the shared
session core can be separated further later.

## Validation notes

See [desktop setup](desktop.md) for supported adapters, permissions, platform
requirements and verification limits. macOS is the runtime-verified platform.
The first release uses the OS credential store and preserves the existing TUI
inside native desktop tabs.
