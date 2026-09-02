<p align="center">
  <img src="assets/icon.png" alt="claude-window-keeper icon" width="160">
</p>

# claude-window-keeper

<!-- AUTO-VERSION-SECTION: DO NOT EDIT MANUALLY -->
## Latest Version : v0.10.7 (2026-09-02)

[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![CI](https://github.com/Chuseok22/claude-window-keeper/actions/workflows/PROJECT-GO-CI.yaml/badge.svg)](https://github.com/Chuseok22/claude-window-keeper/actions/workflows/PROJECT-GO-CI.yaml)
[![Release](https://img.shields.io/github/v/release/Chuseok22/claude-window-keeper?include_prereleases&sort=semver)](https://github.com/Chuseok22/claude-window-keeper/releases)
![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)
![Platform](https://img.shields.io/badge/platform-Linux%2Famd64%20(Docker)-lightgrey)

A personal NAS daemon that re-stitches your Claude Code / Codex / Spark subscription's 5-hour rate-limit window
back together the moment it resets.

A fork of [wavever/CCLimitPing](https://github.com/wavever/CCLimitPing) — see [ATTRIBUTION.md](ATTRIBUTION.md)
for the full relationship. Scope has been narrowed to run as a single Docker container on a Synology NAS.

## Highlights

- **Keeps windows back-to-back.** Watches each enabled provider's 5-hour rate-limit window and fires a minimal
  ping the moment it resets, so a new window starts as soon as possible instead of sitting idle until you
  happen to use the CLI again.
- **Read-only usage checks.** Polling a provider's usage endpoint to see where a window stands consumes no
  quota — only the deliberate trigger ping does.
- **Triggers through the official CLIs.** No private/undocumented request shapes for the actual "start a
  session" step — it shells out to the same `claude` / `codex` binaries you'd run by hand.
- **Two Discord alerts, both non-blocking.** A message is sent when a provider's OAuth refresh token is outright
  rejected — i.e. you need to log back in on that provider — and, on by default, when a trigger's window
  verification confirms a new 5-hour window actually started (set `DISCORD_NOTIFY_ON_SUCCESS=false` to turn the
  latter off). Everything else (rate limits, transient network errors) is handled silently by the retry/backoff
  logic in the watch loop.
- **Three independent providers.** `claude` and `codex` are enabled by default; `spark` (a second Codex-backed
  target) is off by default so it doesn't add another quota-consuming ping until you opt in.

## How it deploys

Every push to `main` runs through GitHub Actions automatically: build/test/lint, then a version bump + GitHub
Release, then a Docker image build pushed to DockerHub and deployed to the NAS over SSH — see
`.github/workflows/PROJECT-GO-CI.yaml`, `PROJECT-COMMON-RELEASE-PUBLISH.yaml`, and `PROJECT-GO-SIMPLE-CICD.yaml`.
There's no manual `docker build`/`docker run` step for a normal release.

One thing the pipeline doesn't do for you: claude-window-keeper reuses whatever OAuth credentials Claude Code /
Codex already produced on another machine, and those never go through git or CI. Before the first deploy, copy
them onto the NAS host by hand, under the path the deploy workflow mounts as the container's `$HOME`:

```sh
scp ~/.claude/.credentials.json  <nas-user>@<nas-host>:/volume1/project/claude-window-keeper/home/.claude/.credentials.json
scp ~/.codex/auth.json           <nas-user>@<nas-host>:/volume1/project/claude-window-keeper/home/.codex/auth.json
```

The container runs as an unprivileged user (`keeper`, UID 1001), not root — but the deploy workflow creates that
host directory with `sudo mkdir -p`, and the `scp` above lands the files as your NAS login user, so both end up
root-owned (or owned by whoever you SSH'd in as). Fix the ownership after copying the files in, or the container
won't be able to read or write its own credentials:

```sh
sudo chown -R 1001:1001 /volume1/project/claude-window-keeper/home
```

After that first manual copy, if a refresh token later dies outright (an `AuthExpiredError` Discord alert
fires), you don't have to repeat the scp dance — re-run `claude` CLI login locally, then
`gh secret set CLAUDE_CREDENTIALS_JSON < ~/.claude/.credentials.json` and manually trigger the
`SYNC-CLAUDE-CREDENTIALS` GitHub Actions workflow (`workflow_dispatch` only — it never runs on a normal `main`
push). It reuses the same SSH secrets as the deploy workflow, writes the file to the NAS with the right
ownership/permissions, and verifies the file landed correctly by running `status claude` inside the
container — no restart or redeploy needed, though the watch loop's own retry can still lag up to its backoff
cap (10 minutes) before it actually recovers the window. See `.claude/rules/20-cicd-deployment.md` for why
it's a separate, manually-triggered workflow instead of being folded into the deploy pipeline. Codex/Spark
credentials (`~/.codex/auth.json`) still go through the manual scp above; this workflow only covers Claude.
Once the run succeeds, delete the secret (`gh secret delete CLAUDE_CREDENTIALS_JSON`) — a stale value left
sitting in a public repo's secret store is a real credential-rotation hazard if the workflow is ever
accidentally re-run later.

Discord alerting (sent once per provider per process run, the first time that provider's OAuth refresh token is
rejected outright — the in-memory dedupe resets on restart) is configured via the `ENV_FILE` GitHub Secret — set
`DISCORD_WEBHOOK_URL=...` there, not in a local config file. If you're updating an existing deploy that used
`TELEGRAM_BOT_TOKEN` / `TELEGRAM_CHAT_ID`, update the secret *before or at the same time as* deploying this
version — otherwise no alert can fire (the webhook URL is unset, so `Notify` treats itself as disabled) until you
fix it. This is not entirely silent: `watch`'s startup log records `discord alerting: enabled` or `disabled
(DISCORD_WEBHOOK_URL not set)`, so `docker logs` shows the misconfiguration even though no Discord message goes
out.

The same webhook also sends one message when a trigger's post-ping verification confirms the 5-hour window
actually started (provider name, next reset time, and token/cost if the CLI reported any) — useful for
confirming the daemon is actually working during early operation. This is on by default; set
`DISCORD_NOTIFY_ON_SUCCESS=false` in the `ENV_FILE` secret to turn it off once you've confirmed things are
stable. Like `DISCORD_WEBHOOK_URL`, this is baked into the image at build time and read once at container
startup — a plain `docker restart` reuses the same image and will NOT pick up a changed secret. Turning it off
takes a new build+deploy (this repo's CI/CD is trunk-based, so pushing to `main` after updating the secret is
enough).

For local development, `docker build -t claude-window-keeper:local .` and `docker run --rm
claude-window-keeper:local <command>` work without any of the above.

## How it works

1. **Watch loop.** `watch` starts one loop per enabled provider. Each loop reads the provider's current usage,
   works out when the 5-hour window resets, sleeps until just after that (plus `reset_buffer` from config), then
   sends a minimal trigger prompt so a new window opens right away. A weekly usage ceiling (`weekly_threshold`)
   pauses pinging for a provider until its weekly window itself resets.
2. **Usage reads.** Each provider implements a `ReadUsage` call against that provider's own OAuth-backed usage
   endpoint — the same data your official client would show you, fetched read-only and consuming no quota.
3. **Triggering.** `Trigger` shells out to the official CLI (`claude` or `codex`) through a PTY, sends the
   configured minimal prompt, and parses the CLI's own output for token/cost usage where available.
4. **Verification.** Right after a trigger, the scheduler re-reads usage to confirm the window actually rolled
   over before going back to sleep, instead of trusting the trigger call blindly.
5. **Auth failure alert.** If a provider's OAuth refresh token is rejected outright (not just rate-limited), the
   watch loop sends one Discord message via `DISCORD_WEBHOOK_URL` and keeps looping — it doesn't crash the
   daemon, and it doesn't re-alert every cycle for the same provider. A failed send is logged and dropped, never
   retried.
6. **Trigger success alert.** Once a trigger's window is verified active (see step 4), the watch loop also
   sends one Discord message confirming it — gated by `DISCORD_NOTIFY_ON_SUCCESS` (default on). Same
   non-blocking, log-and-drop-on-failure behavior as the auth-failure alert.

## Commands

| Command | What it does |
|---|---|
| `claude-window-keeper status [-v] [--json]` | Prints each enabled provider's current window usage and time to reset. `-v`/`--verbose` adds detail; `--json` emits machine-readable output. |
| `claude-window-keeper ping [provider] [--dry-run]` | Sends one trigger ping immediately (`claude`, `codex`, `spark`, or `all`, default `all`). `--dry-run` shows the command that would run without executing it. |
| `claude-window-keeper watch [provider] [--dry-run] [--live]` | Runs the long-lived watch loop described above. `--live` draws a live status line on an interactive terminal; `--dry-run` logs what it would trigger without actually pinging. |
| `claude-window-keeper redeem [--dry-run]` | Manually spends a banked Codex reset credit, if one is available and redeemable. Redeeming is irreversible, so this is always explicit — the automatic path is the opt-in `auto_redeem` config flag. |
| `claude-window-keeper config init [--force]` / `config path` | Writes a commented default `config.toml` (refusing to overwrite unless `--force`), or prints the path it would use. |
| `claude-window-keeper version` | Prints the binary version. |

Run `claude-window-keeper help [command]` for a command's full flag list. The CLI's own text is Korean-only; this
README stays in English so it's easier to diff against the upstream
[wavever/CCLimitPing](https://github.com/wavever/CCLimitPing) project it was forked from.

## Configuration

Provider selection, prompts, models, and scheduling knobs (`weekly_threshold`, `reset_buffer`, `auto_redeem`, …)
live in `config.toml` — see `claude-window-keeper config init` for a fully commented default, and
`claude-window-keeper config path` for where it's read from. Discord alerting is the one exception: it's read
from the `DISCORD_WEBHOOK_URL` environment variable only, never from `config.toml`, so it can be set as a
deploy-time secret instead of a checked-in file.
