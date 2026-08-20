# CHANGELOG

## [v0.1.1] - 2026-08-20
### Added
- **Diagnostic Snapshot Export**: Added contextual snapshot feature via the `<o>` hotkey. Generates full broker dumps from the main list or queue-specific deep-dive dumps from the queue detail view.
- **Environment Safety Controls**: Added per-environment `readonly` settings in `config.yml`. Production environments can now be locked by default, overrideable via the `--readonly=false` CLI flag.
- **Visual Environment Badge**: The current environment (e.g., `[DEV]`, `[PROD]`) is now prominently displayed with color-coding on the connection status bar to prevent accidental operations in production.
- **Enhanced Consumer Metrics**: Consumers now display `Destination Name`, extracted `PID`, and `Uptime`.

### Changed
- Improved TUI sorting: Queues, Topics, and Consumers are now sorted case-insensitively for better readability.


## [v0.1.0] - 2026-07-30
### Added
- Initial public release of amqcli
- Support for ActiveMQ management via Jolokia (JMX)
- Support for message sending via AMQP 1.0 and STOMP
- Interactive TUI using charmbracelet/bubbletea
- Cross-platform support for Linux, macOS, Windows
