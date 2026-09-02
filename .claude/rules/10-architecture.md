# 아키텍처

## 패키지 구조

```
cmd/claude-window-keeper/   main.go — 진입점
internal/
├── auth/       OAuth 자격증명 로드/갱신 (Claude Code/Codex CLI와 자격증명 파일 공유)
├── cli/        cobra 커맨드 정의, i18n(한국어 텍스트), watch 락
├── config/     TOML 설정 로드/기본값
├── notify/     Discord Webhook 알림 (유일한 알림 채널)
├── pricing/    토큰 비용 계산 (status/ping 출력용)
├── provider/   Provider 인터페이스 + Claude/Codex/Spark 구현
├── scheduler/  watch 루프의 핵심 — window 감시, 트리거, 검증, 알림 연결
└── usage/      Usage/Window 데이터 타입
```

## 핵심 인터페이스

```go
// internal/provider/provider.go
type Provider interface {
    Name() string
    ReadUsage(ctx context.Context) (*usage.Usage, error)   // 읽기 전용, quota 안 씀
    Trigger(ctx context.Context, dryRun bool) (*TriggerResult, error) // 공식 CLI로 최소 요청 발송
}
```

`Claude`, `Codex`, `Spark`(Codex CLI 기반) 세 구현체가 이 인터페이스를 만족합니다. `Trigger()`는 실제
`claude`/`codex` CLI를 **PTY로 인터랙티브하게** 실행합니다(print 모드는 API/Agent SDK 과금이라 구독 quota를
안 씀 — 의도적으로 피함). TUI가 렌더링될 때까지 기다렸다가 프롬프트를 제출하고, 응답이 잠잠해질 때까지
기다린 뒤 종료합니다.

## 인증 에러 분류 (중요 — 알림 조건 판단의 핵심)

두 가지 서로 다른 실패를 구분합니다. 헷갈리면 안 됩니다:

- **`provider.ClaudeSubscriptionAccessError`** (`internal/provider/claude.go`) — 조직/계정이 구독 자체를
  비활성화한 경우. 토큰은 멀쩡한데 접근이 막힌 상태.
- **`provider.AuthExpiredError`** (`internal/provider/provider.go`) — refresh token이 **완전히 죽어서**
  (`auth.ErrRefreshRejected`, HTTP 400/401 응답으로 판정) 사람이 재로그인해야 하는 상태. 이 두 에러 타입 중에서는
  **`AuthExpiredError`만** 인증 실패 Discord 알림을 트리거합니다(`ClaudeSubscriptionAccessError`는 트리거하지
  않음). 트리거 *성공* 알림은 이것과 별개 조건으로 발송됩니다 — 아래 "Discord 알림" 섹션 참고.

```go
// internal/provider/provider.go
type AuthExpiredError struct{ Err error }
```

## Scheduler (internal/scheduler/scheduler.go) — 가장 중요한 파일

```go
type Scheduler struct {
    cfg           config.Config
    targets       []Target
    dryRun        bool
    log           *log.Logger
    live          *liveStatus
    notifyCfg     notify.Config          // os.Getenv("DISCORD_WEBHOOK_URL")로 구성, config.toml 아님
    notifySuccess bool                   // envBoolDefaultTrue("DISCORD_NOTIFY_ON_SUCCESS")로 구성, 기본 true
    authMu        sync.Mutex             // authNotified 보호 — target별 goroutine이 동시에 씀
    authNotified  map[string]bool
}
```

`Run()`이 target(provider)마다 별도 goroutine으로 `runTarget()`을 돌립니다. **`Scheduler`에 새 공유 필드를
추가할 때는 반드시 동시성을 고려하세요** — `authNotified`가 처음엔 락 없이 추가됐다가 최종 리뷰에서 데이터
레이스로 잡혀서 나중에 `sync.Mutex`로 고쳐진 전례가 있습니다(`internal/scheduler/live.go`의 `liveStatus.items`가
같은 문제를 이미 mutex로 풀어놓은 참고 패턴입니다).

`runTarget()`의 핵심 루프 흐름:
1. `ReadUsage` → 에러면 `AuthExpiredError`인지 확인해서 알림 여부 판단, 아니면 그냥 backoff 재시도
2. weekly 한도 소진/5h window 활성 상태면 대기
3. window가 열려있으면 `Trigger()` 호출
4. **`Trigger()`가 성공(`err == nil`)해도 곧바로 믿지 않고**, `postPingGrace`(15초) 대기 후 `ReadUsage`를 다시
   불러서 `FiveHour.Active()`가 진짜 true인지 확인합니다(PTY 자동화가 로그인 프롬프트 같은 걸 성공으로 오인할
   수 있어서 생긴 검증 로직). 이 로직에 재시도 상한이 없는 게 [Issue #1](https://github.com/Chuseok22/claude-window-keeper/issues/1)로 남아있습니다.

## Discord 알림 (`internal/notify/notify.go`)

```go
type Config struct{ WebhookURL string }
func (c Config) Enabled() bool
func Notify(cfg Config, title, message string) error  // 실패 시 error 반환(재시도는 안 함), 10초 타임아웃
```

`config.toml`이 아니라 **환경변수**(`DISCORD_WEBHOOK_URL`)에서 읽습니다. 이유: Docker/CI 배포 파이프라인이
`.env` 기반 시크릿 전달을 이미 갖고 있어서 거기 맞춘 설계입니다 (자세한 내용은 `20-cicd-deployment.md`).
**`config.toml`에 Discord 관련 필드를 추가하려는 시도가 있으면 의도적으로 피한 설계임을 알려주세요.**

인증 완전 실패 알림은, 같은 provider가 실패 상태인 동안 최초 1회만 보냅니다(`authNotified` map으로
추적, `ReadUsage` 성공 시 리셋). 실패한 전송은 `Notify()`가 반환한 error를 `notifyAuthExpired` 호출부가
`s.log`에 남기며, 재시도는 하지 않습니다 — watch 루프를 막지 않는 게 우선입니다.

5h window 트리거가 실제로 검증(재조회로 `FiveHour.Active()` 확인)됐을 때도 Discord로 알림을 보낸다
(`Scheduler.notifyTriggerSucceeded`, `internal/scheduler/scheduler.go`). 알림 시점은 `Trigger()` 호출
성공이 아니라 그 이후의 검증 성공 지점이다 — `Trigger()`만 믿으면 로그인 프롬프트 오인 같은 경우에
거짓 성공 알림이 나갈 수 있기 때문이다. 환경변수 `DISCORD_NOTIFY_ON_SUCCESS`(기본값 `true`, `false`로
설정하면 끔, `DISCORD_WEBHOOK_URL`과 동일하게 컨테이너 시작 시 1회만 읽음)로 켜고 끌 수 있다. dry-run
모드에서는 애초에 이 코드 경로에 도달하지 않으므로 알림이 나가지 않는다. 반대로 트리거는 실제로 성공했는데
검증 재조회 자체가 일시적으로 실패하는 경우(네트워크 순단 등)에는 그 window에 대한 성공 알림이 그냥
누락된다 — 실패 알림처럼 별도 dedup 상태를 두지 않기로 한 설계상 트레이드오프이며 버그가 아니다.

## 자격증명 로딩 (`internal/auth/`)

`ClaudeAuth`/`CodexAuth`가 각각 `~/.claude/.credentials.json`, `~/.codex/auth.json`(또는
`$CODEX_HOME/auth.json`)을 읽고, 만료되면 refresh합니다. **macOS Keychain 지원은 제거됐습니다** — Linux
파일 경로만 씁니다. refresh 실패 시 `auth.ErrRefreshRejected`(HTTP 400/401일 때만) sentinel을 반환해서 위의
`AuthExpiredError` 분류로 이어집니다. write-back 실패는 로그만 남기고(swallow하지 않음 — Critical fix #2로
추가됨) 호출자에게는 에러를 전파하지 않습니다.
