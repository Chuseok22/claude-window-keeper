<p align="center">
  <img src="assets/icon.png" alt="claude-window-keeper icon" width="160">
</p>

# claude-window-keeper

<!-- AUTO-VERSION-SECTION: DO NOT EDIT MANUALLY -->
## Latest Version : v0.12.1 (2026-09-02)

[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![CI](https://github.com/Chuseok22/claude-window-keeper/actions/workflows/PROJECT-GO-CI.yaml/badge.svg)](https://github.com/Chuseok22/claude-window-keeper/actions/workflows/PROJECT-GO-CI.yaml)
[![Release](https://img.shields.io/github/v/release/Chuseok22/claude-window-keeper?include_prereleases&sort=semver)](https://github.com/Chuseok22/claude-window-keeper/releases)
![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)
![Platform](https://img.shields.io/badge/platform-Linux%2Famd64%20(Docker)-lightgrey)

**개인 소유 Synology NAS 위 Docker 컨테이너 하나에서 24/7 돌아가는, Claude Code / Codex / Spark 구독의 5시간
rate-limit window를 이어붙이는 Go 데몬입니다.** window가 리셋되는 순간 공식 CLI로 최소한의 요청을 보내 다음
window를 곧바로 시작시켜서, 사람이 몇 시간 뒤에나 다시 그 도구를 써서 window 스케줄이 밀리는 낭비를 막습니다.

[wavever/CCLimitPing](https://github.com/wavever/CCLimitPing)(MIT License)을 fork해서 개인 NAS 단일 컨테이너
용도로 범위를 극단적으로 좁히고 대부분을 새로 작성했습니다 — 원본과의 정확한 관계는
[ATTRIBUTION.md](ATTRIBUTION.md)를 참고하세요. 다른 사람에게 배포할 계획이 없는 순수 개인 도구이고, 그 전제가
아래 대부분의 설계 결정(Docker 전용 배포, 한국어 단일 CLI, 시크릿 baking 등 "1인 운영이니 감수 가능한 리스크"
판단)의 근거입니다.

## 이게 왜 필요한가

Claude Code / Codex / Spark 구독은 5시간짜리 rolling rate-limit window로 운영됩니다. window가 리셋됐는데
사람이 몇 시간 뒤에야 다시 그 도구를 쓰면, 그만큼 다음 window 스케줄이 뒤로 밀리는 낭비가 생깁니다.
`claude-window-keeper`는 이 문제 하나만 해결합니다 — window를 감시하다가 리셋되는 순간 공식 CLI를 통해 최소
요청 하나를 보내 다음 window를 즉시 열어줍니다. 그 외에는 최대한 조용히, 로그만 남기며 무인 운영됩니다.

- **window를 끊김 없이 이어붙입니다.** 활성화된 provider마다 5시간 window를 감시하다가 리셋되는 순간 최소
  트리거 요청을 보내서, 사람이 다시 쓸 때까지 기다리지 않고 새 window를 즉시 시작시킵니다.
- **usage 조회는 quota를 안 씁니다.** window 상태를 확인하는 폴링은 provider의 usage 엔드포인트를 읽기
  전용으로 조회할 뿐이고, 실제 quota를 쓰는 건 의도적인 트리거 요청뿐입니다.
- **트리거는 공식 CLI를 통해서만 보냅니다.** "세션 시작" 자체에 비공식/비공개 요청 형태를 쓰지 않고, 직접
  손으로 실행하는 것과 동일한 `claude`/`codex` 바이너리를 그대로 shell out합니다.
- **알림은 딱 필요한 만큼만, Discord로.** 인증이 완전히 죽었을 때(재로그인 필요) provider당 한 번, 그리고
  트리거 성공이 실제로 검증됐을 때(기본 켜짐) — 이 두 경우 외에는 재시도/백오프 로직이 조용히 처리합니다.
- **Provider 3종을 독립적으로 운영합니다.** `claude`/`codex`는 기본 활성화, `spark`(Codex 기반의 두 번째
  워치 대상)는 기본 비활성 — 켜기 전까지는 quota를 추가로 쓰지 않습니다.

## 어떻게 동작하는가

- **`internal/provider/`** — `Provider` 인터페이스(`Name`, `ReadUsage`, `Trigger`)를 `Claude`/`Codex`/`Spark`
  세 구현체가 만족합니다. `Trigger()`는 실제 `claude`/`codex` CLI를 **PTY로 인터랙티브하게** 실행합니다 —
  print 모드는 API/Agent SDK 과금이라 구독 quota를 안 쓰기 때문에 의도적으로 피했습니다. TUI가 렌더링될
  때까지 기다렸다가 프롬프트를 제출하고, 응답이 잠잠해질 때까지 기다린 뒤 종료합니다.
- **`internal/scheduler/`** — watch 루프의 핵심. provider(target)마다 별도 goroutine이 돕니다. usage를 읽어
  window 리셋 시각을 계산하고, `reset_buffer`만큼 여유를 두고 잔 뒤 트리거를 보냅니다. **트리거 직후
  곧바로 성공을 믿지 않고** 15초(`postPingGrace`) 뒤 usage를 다시 읽어서 window가 실제로 `Active()`가
  됐는지 검증합니다 — PTY 자동화가 로그인 프롬프트 같은 걸 성공으로 오인할 수 있어서 생긴 안전장치입니다.
  weekly usage가 임계치(`weekly_threshold`)를 넘으면 그 provider의 트리거를 weekly window가 자체적으로
  리셋될 때까지 쉽니다.
- **`internal/auth/`** — Claude Code/Codex CLI가 이미 만들어둔 자격증명 파일(`~/.claude/.credentials.json`,
  `~/.codex/auth.json`)을 그대로 재사용합니다. 이 프로젝트는 자체 로그인 절차를 구현하지 않습니다. API가
  401을 반환하면 먼저 디스크에서 파일을 다시 읽고(`Reload` — 공식 CLI가 그 사이 갱신했을 수 있으므로),
  그래도 안 되면 refresh token으로 access token을 갱신하고 결과를 파일에 다시 씁니다. refresh token 자체가
  거부되면(HTTP 400/401) `AuthExpiredError`로 분류됩니다 — 조직/계정이 구독을 비활성화한 경우(토큰은
  멀쩡함, `ClaudeSubscriptionAccessError`)와는 다른 별개의 실패이고, **Discord 알림을 트리거하는 건
  `AuthExpiredError`뿐**입니다.
- **`internal/notify/`** — Discord Incoming Webhook 하나만 쓰는 알림 채널. `DISCORD_WEBHOOK_URL` 환경변수로
  설정하고(`config.toml`에는 안 들어감 — 배포 시크릿으로 다루기 위한 의도적 분리), 전송이 실패하면 에러를
  로그로 남기고 재시도 없이 넘어갑니다. 인증 완전 실패 알림은 같은 provider가 실패 상태인 동안 최초 1회만
  갑니다(성공하면 리셋).

## 배포 방법

### 평상시 배포 — trunk-based CI/CD

`main`에 push할 때마다 GitHub Actions가 자동으로: 빌드/테스트/lint → 버전 bump + GitHub Release → Docker
이미지 빌드 → DockerHub push → NAS로 SSH 배포까지 전부 처리합니다(`.github/workflows/PROJECT-GO-CI.yaml`,
`PROJECT-COMMON-RELEASE-PUBLISH.yaml`, `PROJECT-GO-SIMPLE-CICD.yaml`). 평소에 수동으로 `docker build`/
`docker run`을 칠 일이 없습니다.

**DockerHub 저장소는 반드시 private이어야 합니다.** `ENV_FILE` GitHub Secret의 내용(`DISCORD_WEBHOOK_URL` 등)이
빌드 시점에 이미지 레이어에 그대로 구워지기 때문입니다(`Dockerfile`의 `COPY .env* /app/`,
`entrypoint.sh`가 컨테이너 시작 시 소싱). public 저장소였다면 누구나 이미지를 pull해서
`docker run --entrypoint cat <image> /app/.env`로 웹훅 URL을 뽑아낼 수 있습니다. 코드/CI 어디에도 이걸
강제하는 장치는 없고, private 유지는 배포자가 직접 책임지는 수동 요건입니다.

로컬에서 `docker build -t claude-window-keeper:local .`을 돌릴 때도 같은 위험이 있습니다 — `.dockerignore`가
`.env`를 일부러 제외 목록에서 뺐기 때문에(CI가 만든 `.env`가 빌드 컨텍스트에 도달해야 해서), 저장소 루트에
진짜 시크릿이 든 `.env`가 있는 상태로 로컬 빌드를 돌리면 그 내용도 로컬 이미지에 그대로 구워집니다.
로컬 이미지는 어디에도 push하지 말고, 실제 시크릿이 든 `.env`로 빌드했다면 그 이미지를 남겨두지 마세요.

### 최초 배포 — 자격증명 수동 배치

`claude-window-keeper`는 자체 로그인 절차가 없고, Claude Code/Codex CLI가 다른 머신에서 이미 만들어둔
OAuth 자격증명 파일을 재사용합니다. 이 파일들은 git이나 CI를 절대 거치지 않으므로, 최초 배포 전에 직접
NAS로 옮겨야 합니다 — 배포 워크플로우가 컨테이너의 `$HOME`으로 마운트하는 경로 아래에:

```sh
scp ~/.claude/.credentials.json  <nas-user>@<nas-host>:/volume1/project/claude-window-keeper/home/.claude/.credentials.json
scp ~/.codex/auth.json           <nas-user>@<nas-host>:/volume1/project/claude-window-keeper/home/.codex/auth.json
```

컨테이너는 root가 아니라 비특권 유저(`keeper`, UID 1026)로 돕니다. 배포 워크플로우가 그 호스트 디렉터리를
`sudo mkdir -p`로 만들고, 위 `scp`는 SSH 로그인 유저 권한으로 파일을 내려놓기 때문에 둘 다 root(또는 SSH
로그인 유저) 소유로 남습니다. 소유권을 맞춰주지 않으면 컨테이너가 자기 자격증명을 읽지도 쓰지도 못합니다:

```sh
sudo chown -R 1026:1026 /volume1/project/claude-window-keeper/home
```

Synology NAS(Btrfs 볼륨)라면 `chown`만으로 안 풀릴 수 있습니다 — 공유 폴더에 Windows ACL이 걸려 있으면 ACL이
POSIX 소유권보다 우선 적용되어, 위 `chown`을 했는데도 컨테이너가 `permission denied`로 계속 crash-loop할 수
있습니다. `sudo /usr/syno/bin/synoacltool -get <경로>`로 확인해보고, 필요하면 `sudo find
/volume1/project/claude-window-keeper/home -exec /usr/syno/bin/synoacltool -del {} \;`로 ACL을 제거하세요.

### refresh token이 나중에 완전히 죽으면 — `SYNC-CLAUDE-CREDENTIALS` 워크플로우

최초 배치 이후, Claude의 refresh token이 완전히 만료돼서(`AuthExpiredError`, Discord 알림 발송) 재로그인이
필요해지면 위 `scp` 절차를 반복할 필요 없이 `.github/workflows/SYNC-CLAUDE-CREDENTIALS.yaml`
(`workflow_dispatch` 전용 — `main` push로는 절대 실행되지 않음)을 씁니다:

1. 로컬에서 `claude` CLI로 재로그인
2. `gh secret set CLAUDE_CREDENTIALS_JSON < ~/.claude/.credentials.json`
3. GitHub Actions 탭에서 이 워크플로우를 수동 실행(Run workflow)
4. 실행이 성공하면 즉시 `gh secret delete CLAUDE_CREDENTIALS_JSON`으로 secret을 삭제

기존 배포 워크플로우와 같은 SSH secret(`SERVER_HOST`/`SERVER_USER`/`SERVER_PASSWORD`/`SSH_KEY`)을 재사용해
NAS에 파일을 반영하고, 소유권/권한을 맞추고, 덮어쓰기 전에 기존 파일을 `.bak`으로 백업한 뒤, 컨테이너 안에서
`status claude`를 실행해 refresh token이 실제로 Anthropic 서버까지 왕복하는지 검증합니다 — 검증이 실패하면
자동으로 `.bak`을 복원(또는 첫 실행이었다면 새로 쓴 파일을 제거)하고 실패로 끝납니다.

**컨테이너 재시작/재배포는 필요 없습니다.** API가 401을 받을 때마다 디스크에서 자격증명을 다시 읽으므로,
NAS 파일만 갱신되면 스케줄러의 다음 재시도 사이클에서 자동으로 복구됩니다(다만 재시도 backoff 상한만큼,
최대 10분 지연될 수 있습니다).

**왜 배포 파이프라인에 합쳐두지 않고 별도 워크플로우로 뒀는가**: 매 `main` push마다 자동 실행되면, 컨테이너가
이미 refresh해서 최신 상태인 토큰을 이 워크플로우에 저장된 구버전 secret 값으로 덮어써버릴 위험이 있기
때문입니다. 그래서 사람이 재로그인 직후에만 의도적으로 트리거하는 수동 워크플로우로 분리했습니다. 4번(secret
삭제)도 같은 이유입니다 — secret을 지워두지 않으면, 나중에 이 워크플로우가 실수로 재실행될 때(GitHub의
"Re-run job" 등) 그 시점 secret에 남아있는 오래된/이미 회전된 토큰이 그대로 NAS에 반영돼 인증을 오히려
깨뜨릴 수 있습니다. Codex/Spark 자격증명(`~/.codex/auth.json`)은 이 워크플로우의 대상이 아니라 지금도 수동
`scp`로 옮깁니다.

### Discord 알림 설정

Discord 알림은 `config.toml`이 아니라 **환경변수**로만 설정합니다 — `ENV_FILE` GitHub Secret에
`DISCORD_WEBHOOK_URL=...`을 넣으면 CI가 빌드 시점에 이미지에 구워 넣고, `entrypoint.sh`가 컨테이너 시작 시
소싱합니다. `watch`의 시작 로그가 `discord alerting: enabled` 또는 `disabled (DISCORD_WEBHOOK_URL not set)`을
남기므로, 시크릿이 비어있으면 `docker logs`로 바로 알 수 있습니다.

- **인증 완전 실패 알림**: provider의 refresh token이 완전히 거부되면(재로그인 필요) 그 provider가 계속
  실패 상태인 동안 최초 1회만 발송(프로세스 재시작 시 다시 알림 가능).
- **트리거 성공 알림**: 트리거의 window 검증이 실제로 성공했을 때(provider 이름, 다음 리셋 시각, 있으면
  토큰/비용까지) 발송 — 초기 운영 검증에 특히 유용합니다. 기본 켜짐이고
  `DISCORD_NOTIFY_ON_SUCCESS=false`로 끌 수 있습니다. `DISCORD_WEBHOOK_URL`과 마찬가지로 빌드 시점에
  구워지고 컨테이너 시작 시 한 번만 읽으므로, **`docker restart`만으로는 반영되지 않습니다** — 시크릿을
  바꾼 뒤 `main`에 push해서 새로 빌드·배포해야 합니다.

### 로컬 개발

`docker build -t claude-window-keeper:local .` 후 `docker run --rm claude-window-keeper:local <command>`로
위 절차 없이 로컬 테스트가 가능합니다. `.env` 관련 주의사항은 위 "평상시 배포" 항목을 참고하세요.

## 운영 중 발생 가능한 시나리오

| 상황 | 무슨 일이 일어나는가 | 사람이 할 일 |
|---|---|---|
| 평상시 | window가 리셋될 때마다 자동으로 트리거하고 검증까지 조용히 반복 | 없음 |
| access token 만료 | API 401 → 디스크 재조회 → 그래도 안 되면 refresh token으로 자동 갱신 | 없음 |
| weekly 한도 도달 | 해당 provider의 트리거를 weekly window 자체 리셋까지 대기(로그만 남김) | 없음 |
| refresh token 완전 만료 | `AuthExpiredError` → Discord 알림 1회 → 데몬은 계속 루프를 돌며 재시도만 함(스스로 복구 불가) | 위 "`SYNC-CLAUDE-CREDENTIALS` 워크플로우" 절차 수행 |
| 트리거 성공 검증됨 | Discord로 성공 알림 발송(기본 켜짐) | 없음(끄고 싶으면 `DISCORD_NOTIFY_ON_SUCCESS=false`) |
| Codex reset credit 보유 | `status`로 확인 가능, 자동 사용은 `auto_redeem` 설정 시에만 | `redeem`으로 수동 사용(비가역적이라 기본은 수동) |

## 명령어

| 명령어 | 하는 일 |
|---|---|
| `claude-window-keeper status [provider] [-v] [--json]` | 지정한(또는 기본 활성화된 전체) provider의 현재 window 사용량과 리셋까지 남은 시간을 출력합니다. `provider`는 `claude`/`codex`/`spark`/`all`(생략 시 활성화된 전체), `-v`/`--verbose`는 상세 정보, `--json`은 기계 판독 가능한 출력. |
| `claude-window-keeper ping [provider] [--dry-run]` | 트리거 요청을 즉시 한 번 보냅니다(`claude`/`codex`/`spark`/`all`, 기본 `all`). `--dry-run`은 실행할 명령만 보여주고 실제로 보내지 않습니다. |
| `claude-window-keeper watch [provider] [--dry-run] [--live]` | 위에서 설명한 장기 실행 watch 루프입니다. `--live`는 인터랙티브 터미널에 실시간 상태 줄을 그리고, `--dry-run`은 실제 트리거 없이 로그만 남깁니다. |
| `claude-window-keeper redeem [--dry-run]` | 사용 가능하고 회수 가능한 Codex reset credit이 있으면 수동으로 씁니다. 비가역적이라 항상 명시적으로 실행해야 하고, 자동 경로는 opt-in `auto_redeem` 설정뿐입니다. |
| `claude-window-keeper config init [--force]` / `config path` | 주석이 달린 기본 `config.toml`을 씁니다(`--force` 없이는 기존 파일을 덮어쓰지 않음), 또는 그 경로를 출력합니다. |
| `claude-window-keeper version` | 바이너리 버전을 출력합니다. |

각 명령어의 전체 플래그 목록은 `claude-window-keeper help [command]`로 확인하세요. CLI 자체의 출력 텍스트는
한국어 단일 언어입니다.

## 설정

Provider 선택, 프롬프트, 모델, 스케줄링 값(`weekly_threshold`, `reset_buffer`, provider별 `align_start` 등)은
`config.toml`에 있습니다 — `claude-window-keeper config init`으로 주석이 달린 기본값을 생성하고,
`claude-window-keeper config path`로 실제 읽는 경로를 확인하세요(기본 `~/.config/claude-window-keeper/config.toml`,
`$XDG_CONFIG_HOME`을 존중합니다). 주요 필드:

- `weekly_threshold` (기본 `0.99`) — weekly usage가 이 비율(0~1) 이상이면 weekly window 자체가 리셋될 때까지
  해당 provider의 트리거를 건너뜁니다.
- `reset_buffer` (기본 `10s`) — window 리셋 시각 이후 이만큼 더 기다렸다가 트리거를 보내서, 리셋이 실제로
  반영됐는지 여유를 둡니다.
- `usage_display` (기본 `"used"`) — `status`가 보여주는 퍼센트 표기 기준(`"used"` 또는 `"remaining"`).
- `[claude]` / `[codex]` / `[spark]` — provider별로 `enabled`, `prompt`(트리거에 보낼 최소 메시지), `model`,
  `extra_args`, `align_start`(첫 window 위상을 고정하고 싶을 때의 RFC3339 앵커). `auto_redeem`(Codex 전용,
  기본 꺼짐)은 켜면 `watch`가 곧 만료될 banked reset credit을 자동으로 회수합니다 — 비가역적이라 기본은
  꺼져 있고 `redeem` 명령으로 수동 실행하는 쪽을 권장합니다.

Discord 알림(`DISCORD_WEBHOOK_URL`, `DISCORD_NOTIFY_ON_SUCCESS`)은 예외적으로 `config.toml`에 두지 않고
환경변수로만 읽습니다 — 배포 시점 시크릿으로 다루기 위한 의도적 분리입니다.

## 라이선스

MIT — [LICENSE](LICENSE)를 참고하세요. 원본 프로젝트와의 정확한 관계와 변경 내역은
[ATTRIBUTION.md](ATTRIBUTION.md)에 정리돼 있습니다.
