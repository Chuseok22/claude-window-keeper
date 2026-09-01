# Attribution

claude-window-keeper is a fork of [CCLimitPing](https://github.com/wavever/CCLimitPing) by
[wavever](https://github.com/wavever), licensed under the MIT License.

This fork narrows CCLimitPing's scope to a single deployment target — a self-hosted Docker container on a personal
Synology NAS — and removes everything that only makes sense for local, interactive, multi-OS use (background daemon
management, the interactive-session proxy, local hook-based activity detection, self-upgrade, and macOS-specific
code). It adds Docker packaging, Discord alerting on OAuth failure, and a Korean-only CLI.

See [LICENSE](LICENSE) for the full MIT license text and copyright notices.
