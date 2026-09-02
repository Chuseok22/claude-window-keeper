# Changelog

**현재 버전:** 0.12.3  
**마지막 업데이트:** 2026-09-02T16:29:57Z  

---

## [0.12.3] - 2026-09-02

**🐛 수정**
- change container UID from 1001 to 1026 (Synology ACL owner-not-found)

---

## [0.12.2] - 2026-09-02

**📝 문서**
- README_전체_개편_및_업스트림_아이콘_소셜_프리뷰_교체 — README.md 전면 재작성 - 한국어, fork 고유 로직·운영 매뉴얼 구조

---

## [0.12.1] - 2026-09-02

**📝 문서**
- 20260825_#4_문서_README에_DockerHub_private_요구사항_및_로컬_env_주의사항_누락 — DockerHub private 요구사항, 로컬 .env 빌드 주의사항, Discord 설정 문서 보강

---

## [0.12.0] - 2026-09-02

**✨ 기능**
- add optional provider arg to status command

**🐛 수정**
- address PR #21 review findings in credentials sync workflow
- address final review findings in Claude credentials sync workflow

**🔧 변경사항**
- add manual workflow to sync Claude OAuth credentials to NAS

---

## [0.11.0] - 2026-09-02

**✨ 기능**
- send a Discord notification when a trigger's window verifies active
- add DISCORD_NOTIFY_ON_SUCCESS toggle scaffolding

**🐛 수정**
- use Korean wording for token/cost info in Discord success notification

**📝 문서**
- address PR review feedback on remaining auth-only Discord wording
- fix stale toggle-restart claim and auth-only wording after Discord success notify
- document the Discord trigger-success notification and its toggle

---

## [0.10.7] - 2026-09-01

**🐛 수정**
- reject non-HTTPS Discord webhooks and stop leaking the URL in error logs
- log Discord alerting enabled/disabled state at watch startup
- surface Discord webhook send failures instead of swallowing them

**📝 문서**
- clarify Discord alert dedupe is per-process, and the disabled state is logged
- update internal design docs for Discord alerting, drop known-issues file
- update deploy-facing docs for the Discord webhook alert channel

---

## [0.10.6] - 2026-09-01

**🐛 수정**
- cap watch.lock removal retries with backoff instead of busy-looping
- thread a writer into acquireWatchLock for stale-lock-removal logging

---

## [0.10.5] - 2026-09-01

**🐛 수정**
- add container restart policy, remove unused port mapping from CI/CD deploy

**📝 문서**
- record #2 restart-policy fix and #14 partial fix (port removal)
- record restart-policy fix and port-var removal in CI/CD notes

---

## [0.10.4] - 2026-08-28

## [0.10.4]

---

## [0.10.3] - 2026-08-28

**🔧 변경사항**
- ignore .claude/worktrees/ (Claude Code git worktrees)

---

## [0.10.2] - 2026-08-28

## [0.10.2]

---

## [0.10.1] - 2026-08-25

**📝 문서**
- add CLAUDE.md and .claude/rules project onboarding docs

---

## [0.10.0] - 2026-08-25

**✨ 기능**
- add live status line (spinner + countdowns) to watch
- add background (bg) command to run watch detached
- hook-based active-session detection for Claude and Codex

**🐛 수정**
- submit the prompt in Claude's interactive trigger

**♻️ 리팩토링**
- hooks-only active-session detection; auto-install hooks
- drop GLM provider, target Claude Code and Codex only

**🔧 변경사항**
- install project-auto-wizard Go CI/CD templates
- roll bg command and watch animation into v0.4.0
- add project icon and social-preview assets
- release v0.4.0
- guard Scheduler.authNotified with a mutex
- fix stale watch.lock treating own PID as a live conflict
- log credential write-back failures instead of swallowing them
- document required chown of NAS credential volume to UID 1001
- point PROJECT-GO-SIMPLE-CICD at this daemon's actual healthcheck/volume needs
- bake .env into the image per spec section 9 design
- add root Dockerfile and .env-sourcing entrypoint
- translate README header to English
- rewrite README for Docker/NAS-only, Korean framing, new branding
- fix leftover limitping references in Korean i18n text
- collapse i18n to Korean-only CLI text
- verify Trigger() actually started a window instead of trusting PTY exit
- notify Telegram once per auth-expiry episode, drop other alert sites
- replace macOS notify with Telegram Bot API alerting
- add typed AuthExpiredError for definitively-rejected refresh tokens
- add ATTRIBUTION.md, credit wavever/CCLimitPing as upstream
- rename module to github.com/Chuseok22/claude-window-keeper, binary to claude-window-keeper
- remove install.sh (Docker-only distribution)
- remove goreleaser (superseded by project-auto-wizard's release/deploy pipeline)
- drop macOS Keychain auth and Windows-only files (Linux/amd64 only)
- remove bg/continue/hooks/schedule/upgrade/uninstall and active-task detection
- Stop the reset-credit tests from reading real Codex credentials
- Prepare v0.9.0
- Add Codex reset-credit redemption and zone-tagged reset times
- Diagnose disabled Claude subscription access behind usage 429s
- Prepare v0.8.0
- Show remaining lifetime on Codex reset credits
- Localize status and ping text output for Chinese locales
- Adapt Codex to the weekly-only rate-limit regime
- Prepare v0.7.0
- Show Codex reset credits in status
- Add scheduled ping command
- Add `continue` interactive proxy that auto-resumes on 5h recovery
- Add Spark as a Codex-backed provider (#4)
- Add status --json output
- Reduce idle watch polling
- Add status progress output
- Reduce watch power usage
- Add bg ping history and ASCII heartbeat
- Prepare v0.4.2 release
- Fix Codex ping window trigger
- Fix usage endpoint reads
- Add README icon
- Prepare v0.3.0 release docs
- Improve CLI help aliases and localization
- Defer pings while provider tasks run
- Add upgrade and uninstall commands
- Retry usage status requests
- Prepare repository for open source release
- Switch Claude trigger to interactive CLI
- Add per-provider watch: `watch <claude|codex|glm>` watches just that one
- Initial commit: limitping — keep Claude/Codex/GLM windows back-to-back

---

