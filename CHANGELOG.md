# Changelog

All notable changes to this project are documented here. The project follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html) before v1.0, so minor
releases may include API changes.

## [0.2.0] - 2026-08-25

### Added

- Compatibility with the fx v0.0.6 CLI and ACP protocol.
- Provider selection and login wrappers for the v0.0.6 provider model.
- Session mode support and expanded status, model, permission, and recovery data.
- Public CI, dependency updates, security reporting, and upstream release checks.

### Changed

- Stdin prompts now enforce fx's 8 MiB input limit.
- Client concurrency documentation now states that configuration must be frozen
  before concurrent calls.
- Invalid runtime, retry, and session ID values fail before spawning fx.

### Fixed

- ACP calls retain a final response delivered concurrently with process exit.
- Malformed ACP session updates are reported instead of silently downgraded.
- ACP stderr read failures are surfaced through the session error stream.
- Login flows continue draining process output after finding the authorization URL.
- Cancellation from `Version` is consistently classified as interrupted.
- Mock binaries are isolated per test package, eliminating full-suite build races.
- `SessionUsage` rejects path traversal and checks cancellation after file reads.

## [0.1.0] - 2026-08-22

- Initial SDK release with one-shot asks, ACP sessions, admin wrappers, fixtures,
  examples, and guarded dangerous-mode helpers.

[0.2.0]: https://github.com/Obedience-Corp/vercel-fx-go/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/Obedience-Corp/vercel-fx-go/tree/v0.1.0
