# CI/CD, Docker, 배포

## ⚠️ `main` push = 실제 배포입니다

`.github/workflows/`는 `project-auto-wizard`(범용 프로젝트 부트스트래핑 도구)가 설치한 **trunk-based** 파이프라인입니다. `version.yml`의 `metadata.template.branches.mode: "trunk-based"`가 이 모드를 켭니다. 이 말은:

- **`main`에 push할 때마다 자동으로**: 버전 bump → 체인지로그 생성 → git 태그 → GitHub Release 발행 → Docker 이미지 빌드 → DockerHub push → SSH로 NAS 접속해서 컨테이너 재배포까지 **전부 자동으로 이어집니다.**
- `PROJECT-GO-CI.yaml`(빌드 검증)과 `PROJECT-COMMON-RELEASE-PUBLISH.yaml`(릴리즈)은 **서로 독립된 워크플로우 파일**이라 `needs:` 의존관계가 없습니다 — **빌드가 깨진 커밋도 그대로 릴리즈되고 배포될 수 있습니다.** 이건 알려진 갭이고([Issue #2](https://github.com/Chuseok22/claude-window-keeper/issues/2) 근처 논의 참고), 워크플로우 로직을 못 고치는 제약(아래 참고) 때문에 "main에 깨진 커밋을 push하지 않는다"는 운영 규율로만 막고 있습니다. **push 전에 반드시 로컬에서 `gofmt -l .`, `go build ./...`, `go vet ./...`, `go test -race ./...`를 통과시키세요.**

## 워크플로우 파일: 어디까지 고쳐도 되나

`.github/workflows/PROJECT-*.yaml` 파일 상단에 `# project-auto-wizard:managed-workflow` 주석이 있습니다. 이건
**마법사가 관리하는 파일**이라는 뜻입니다:

- **`jobs:`/`steps:` 로직은 직접 고치지 마세요.** 다음에 `project-auto-wizard`를 다시 돌리면 덮어써질 수
  있습니다.
- **`env:` 블록(파일 상단, 주석에 "프로젝트별 수정 필요"라고 명시된 구간)은 안전하게 수정 가능**하고, 실제로
  이미 이 프로젝트에 맞게 커스터마이즈돼 있습니다(아래 표 참고).
- `.github/scripts/*.py`(`version_manager.py`, `changelog_manager.py`, `issue_helper.py`,
  `truncate_release_notes.py`)도 마법사가 관리하는 로직입니다. 손대지 마세요.
- `.github/.wizard/`는 마법사 자체의 상태 추적 디렉터리입니다. 건드리지 마세요.

### 이미 커스터마이즈된 값 (`PROJECT-GO-SIMPLE-CICD.yaml`)

이 데몬은 HTTP 서버가 없어서, 마법사 기본값(HTTP 서비스 전제)을 이렇게 고쳐놨습니다:

| 변수 | 값 | 이유 |
|---|---|---|
| `HEALTHCHECK_PATH` | `""` (빈 값) | HTTP 헬스체크 비활성화 — 이 데몬은 포트를 안 엽니다 |
| `HEALTHCHECK_LOG_PATTERN` | `"watching"` | `scheduler.go`의 `Run()`이 찍는 시작 로그(`"watching %v (...)"`와 매칭. **이 로그 문구를 바꾸면 헬스체크가 깨집니다** |
| `ENABLE_VOLUME_MOUNT` | `"true"` | |
| `VOLUME_HOST_PATH` | `/volume1/project/claude-window-keeper/home` | NAS 쪽 실제 경로 |
| `VOLUME_CONTAINER_PATH` | `/home/keeper` | 컨테이너의 `$HOME` 전체(Dockerfile의 `ENV HOME=/home/keeper`와 일치) — `.claude`/`.codex`가 이 아래 서브디렉터리로 자동 영속화됨. 볼륨 쌍을 하나만 지원하는 스크립트 제약 때문에 둘을 따로 마운트하는 대신 `$HOME` 전체를 마운트하는 방식을 씀 |
| `--restart unless-stopped` (docker run 플래그, env 변수 아님) | 추가됨 | 컨테이너가 크래시하거나 NAS가 재부팅돼도 자동 재시작됨 — 수동 `docker stop`은 존중(`always`가 아니라 `unless-stopped`) — [Issue #2](https://github.com/Chuseok22/claude-window-keeper/issues/2) 수정 완료 |

**포트 관련 env 변수(`CONTAINER_INTERNAL_PORT`, `DEPLOY_PORT`)는 완전히 제거됐습니다** — 이 데몬은 포트를
전혀 안 열기 때문에, `docker run`의 `-p` 옵션 자체를 뺐습니다
([Issue #14](https://github.com/Chuseok22/claude-window-keeper/issues/14) 포트 부분 수정 완료). 같은 이슈가
지적한 `.env` 이미지 baking 방식은 검토 후 **현행 유지**로 결정했습니다 — DockerHub 저장소를 private으로 유지하는
것은 여전히 배포자가 첫 배포 전에 직접 확인해야 하는 수동 절차입니다(자동 검증 없음, 의도적으로 문서화도 추가하지
않기로 결정 — 자세한 배경은 GitHub Issue #14를 참고).

**주의**: `version.yml`의 `deploy.go.VOLUME_HOST_PATH`/`VOLUME_CONTAINER_PATH`는 위 표와 값이 **다릅니다**(아직
옛날 `/mnt/__PROJECT_NAME__` 형태 그대로) — 이건 마법사가 별도로 기억하는 메타데이터라서 워크플로우 파일 자체를
직접 고칠 때는 안 갱신됐습니다. 마법사를 다시 돌려서 재동기화가 일어나면 이 값들이 워크플로우 파일을 덮어쓸
수도 있으니, 그런 상황이 생기면 이 표를 기준으로 다시 맞춰야 합니다.

## Claude 자격증명 재동기화 (수동, `SYNC-CLAUDE-CREDENTIALS.yaml`)

Claude Code의 OAuth refresh token이 완전히 만료되면(`AuthExpiredError`, Discord 알림 발송) 사람이 로컬에서
재로그인해야 합니다. 이후 NAS에 반영하는 절차를 `.github/workflows/SYNC-CLAUDE-CREDENTIALS.yaml`
(`workflow_dispatch`로만 실행, `project-auto-wizard:managed-workflow` 마커 없음 — 마법사가 관리하지 않는 순수
수동 파일)로 대체할 수 있습니다.

사용 절차: 로컬에서 `claude` CLI로 재로그인 → `gh secret set CLAUDE_CREDENTIALS_JSON <
~/.claude/.credentials.json`으로 secret 갱신 → GitHub Actions 탭에서 이 워크플로우를 수동 실행. SSH 접속은
`PROJECT-GO-SIMPLE-CICD.yaml`과 동일한 secret(`SERVER_HOST`/`SERVER_USER`/`SERVER_PASSWORD`/`SSH_KEY`)을
재사용하므로 새로 등록할 secret은 `CLAUDE_CREDENTIALS_JSON` 하나뿐입니다. 실행이 성공하면 `gh secret delete
CLAUDE_CREDENTIALS_JSON`으로 secret을 즉시 삭제합니다 — 나중에 이 워크플로우가 실수로 재실행되면 그 시점의
secret 값이 그대로 NAS에 반영되는데, 이미 회전됐거나 오래된 토큰일 수 있어 인증을 오히려 깨뜨릴 수 있기
때문입니다.

**의도적으로 trunk-based 배포 파이프라인과 분리되어 있습니다.** 매 `main` push마다 자동 실행되면, 컨테이너가
이미 refresh해서 최신 상태인 토큰을 이 워크플로우에 저장된 구버전 secret 값으로 덮어써버릴 위험이 있기
때문입니다 — 이 워크플로우는 사람이 재로그인 직후에만 의도적으로 트리거해야 합니다.

**컨테이너 재시작이나 재배포는 필요 없습니다.** `internal/provider/provider.go`의 `fetchWithAuth`가 API 401
응답마다 `Reload()`로 디스크에서 자격증명을 다시 읽으므로(`internal/auth/claude.go`), NAS 파일만 갱신하면
스케줄러의 다음 재시도 사이클에서 자동으로 새 토큰을 집어 읽습니다. 워크플로우 마지막 스텝이 컨테이너 안에서
`claude-window-keeper status claude`(`internal/cli/status.go`의 provider 위치 인자)를 실행해 그 자리에서 바로
파일이 올바르게 반영됐는지 검증합니다. 단, 이 검증은 파일이 정상인지만 확인할 뿐이며, `watch` 데몬 자체가
실제로 재시도해서 window를 복구하는 데는 백오프 상한(`internal/scheduler/scheduler.go`의 `maxBackoff`,
10분)만큼 지연될 수 있습니다.

파일을 덮어쓰기 전에 기존 자격증명을 `.credentials.json.bak`으로 백업하고, NAS 경로는 하드코딩된 값 대신
실행 중인 컨테이너의 실제 볼륨 마운트에서 가져옵니다 — 검증이 실패하면 SSH로 접속해 `.bak` 파일을 원래
이름으로 되돌려 복구할 수 있습니다.

Codex/Spark 자격증명(`~/.codex/auth.json`)은 이 워크플로우의 대상이 아닙니다 — 지금까지처럼 수동 `scp`로
옮깁니다.

## goreleaser는 없습니다

원래 있던 `.goreleaser.yaml`(크로스플랫폼 바이너리 릴리즈용)은 완전히 삭제했습니다. Docker 전용 배포로
좁히면서 바이너리 아카이브 배포 자체가 불필요해졌고, 마법사의 릴리즈 파이프라인이 버전/체인지로그 자동화를
더 잘 해줍니다. `install.sh`(curl 바이너리 설치 스크립트)도 같은 이유로 삭제했습니다.

## Docker

- **`Dockerfile`은 저장소 루트에 있습니다** (`docker/Dockerfile`이 아님) — 마법사 워크플로우의 `file:
  ./Dockerfile` 기본값을 그대로 따르기 위해서입니다.
- 멀티스테이지: `golang:1.25-bookworm`으로 빌드 → `node:22-bookworm-slim`(Debian bookworm의 apt nodejs가
  v18이라 `@anthropic-ai/claude-code`의 `engines.node >=22` 요구사항을 못 맞춰서 이 베이스 이미지를 씀)에
  `@anthropic-ai/claude-code`, `@openai/codex`를 npm으로 설치.
- 컨테이너는 UID **1001**(`keeper` 유저)로 돕니다 — `node:22-bookworm-slim` 베이스 이미지가 이미 UID 1000을
  자체 `node` 유저로 쓰고 있어서 1001을 씀.
- `entrypoint.sh`가 `/app/.env`가 있으면 `set -a; . /app/.env; set +a`로 소싱한 뒤 바이너리를 exec합니다.
- `.env`는 **의도적으로 이미지에 구워집니다** (아래 시크릿 섹션 참고). `COPY .env* /app/`(와일드카드 —
  파일이 없어도 빌드가 안 깨짐, `COPY .env /app/.env`처럼 정확한 파일명을 쓰면 없을 때 빌드가 실패함).

## 시크릿 관리

| 시크릿 | 어디서 오나 | 어떻게 컨테이너에 도달하나 |
|---|---|---|
| `DISCORD_WEBHOOK_URL` | GitHub Secret `ENV_FILE`의 내용 (`.env` 형식) | CI가 빌드 직전에 `.env` 파일로 씀 → Dockerfile이 이미지에 구움 → entrypoint.sh가 소싱 |
| OAuth 자격증명(`.credentials.json`, `auth.json`) | 사람이 다른 머신(맥)에서 로그인 후 수동으로 복사 | 이미지가 아니라 **볼륨 마운트**로만 관리 — 재발급/refresh로 계속 바뀌는 데이터라 이미지에 구우면 안 됨 |

`DISCORD_NOTIFY_ON_SUCCESS`도 이미지에 구워지는 값이다(정적 시크릿은 아니지만 같은 `.env` 경로를 탄다).
기본값이 `true`(켜짐)이라 이 값을 명시적으로 `.env`/`ENV_FILE`에 넣지 않아도 성공 알림은 그대로 나간다 —
끄고 싶을 때만 `DISCORD_NOTIFY_ON_SUCCESS=false` 줄을 추가하면 된다. **단, `ENV_FILE` 시크릿만 고치고
컨테이너를 재시작하는 것만으로는 반영되지 않는다** — `.env`는 CI 빌드 시점에 이미지 안에 구워지고, NAS의
`docker run`은 `--env-file`/`-e DISCORD_*`를 쓰지 않아서 런타임에 다시 읽는 경로가 없다. 값을 바꾸려면
시크릿을 고친 뒤 `main`에 push해서 새 이미지를 빌드·재배포해야 한다(trunk-based CI/CD, 위 "main push = 실제
배포" 참고).

**`DISCORD_WEBHOOK_URL`은 왜 이미지에 구워도 되는가**: OAuth 토큰과 달리 런타임에 재기록될 필요가 없는 정적 시크릿이기
때문입니다. 단, 이게 성립하려면 **DockerHub 저장소가 반드시 private이어야 합니다** — public이면 이미지를
pull한 누구나 `docker run --entrypoint cat <image> /app/.env`로 토큰을 꺼내볼 수 있습니다. 이 요구사항이
README에 명시적으로 안 써있는 게 [Issue #4](https://github.com/Chuseok22/claude-window-keeper/issues/4)입니다.

로컬에서 `docker build .`을 돌릴 때 저장소 루트에 진짜 `.env`가 있으면 그 내용도 그대로 이미지에 들어갑니다 —
주의하세요.

## 자격증명 볼륨 권한

컨테이너는 UID 1001로 돕니다. NAS 쪽 볼륨 디렉터리를 `sudo mkdir -p`로 만들면 root 소유가 되므로, **반드시
`sudo chown -R 1001:1001 <볼륨 경로>`를 해줘야** 컨테이너가 자격증명 파일을 읽고 쓸 수 있습니다(README의
"How it deploys" 섹션에 이 단계가 명시돼 있습니다 — 빠뜨리면 인증이 조용히 영원히 실패하고, 알림도 안 옵니다.
이유: 권한 에러는 `AuthExpiredError`가 아니라서 Discord 알림 조건에 안 걸립니다).
