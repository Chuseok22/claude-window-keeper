# 빌드 / 테스트 / 검증

## 매 변경마다 필수로 통과시켜야 하는 것

```bash
gofmt -l .              # 출력이 없어야 함 (포맷 안 된 파일 있으면 실패로 간주)
go build ./...
go vet ./...
go test -race ./...
```

이 4개는 이 프로젝트의 리라이트 작업 전체에서 "매 태스크는 이 4개를 다 통과해야 완료로 친다"는 규칙으로
운영됐습니다. `main` push가 곧 실제 배포로 이어지는 trunk-based CI/CD 특성상(`20-cicd-deployment.md` 참고),
이 규율은 특히 중요합니다 — CI가 자동으로 빌드 실패를 막아주지 않기 때문입니다.

## `-race`가 못 잡는 것도 있다는 걸 기억하세요

`internal/scheduler`의 `authNotified` 데이터 레이스는 한동안 `go test -race`가 계속 clean으로 통과했는데,
이유는 기존 테스트가 전부 provider 1개짜리 `Target`으로 `runTarget()`을 직접 호출했기 때문입니다 — 실제
프로덕션 경로인 `Scheduler.Run()`(target마다 별도 goroutine)을 2개 이상의 target으로 구동하는 테스트가
하나도 없었습니다. **`Scheduler`에 공유 상태를 추가할 때는, `-race`가 초록불이어도 "이 필드를 실제로 여러
goroutine이 동시에 건드리는 테스트가 존재하는가"를 따로 확인하세요.** 참고 테스트:
`internal/scheduler/scheduler_test.go`의 `TestRun_ConcurrentAuthNotifiedWrites_NoRace` (barrier로 동시 쓰기를
강제하는 패턴).

## 느린 테스트가 두 개 있음

- `TestRunTarget_VerifiesWindowAfterTrigger_RetriesWhenNotActive`(`internal/scheduler/scheduler_test.go`)가
  실제 `postPingGrace`(15초)+`minBackoff`(30초)를 그대로 기다려서 ~45초 걸립니다.
- `TestRunTarget_VerifyFailureCap_StopsRetriggeringAfterMaxFailures`(같은 파일, Issue #1 fix)는 연속 검증
  실패 3회에 도달할 때까지 `postPingGrace`×3 + 실제 verify-backoff escalation(30초→60초)을 그대로 기다린 뒤,
  cap이 재시도를 실제로 막는지(단순히 지연시키는 게 아니라) 확인하려고 escalation-without-cap 회귀가
  4번째 핑을 쏠 시점(약 135초 뒤)까지도 더 기다립니다 — 실측 ~4분 30초로 이 스위트에서 가장 느린 테스트입니다.
  (초기 버전은 3회 도달 직후 2초만 기다리고 끝내서 cap 유무를 구분 못 하는 공허한 단언이었음 — fable5 리뷰에서
  발견, 대기시간을 늘려 실제로 회귀를 잡도록 고침.)

나머지 테스트는 전부 500ms 이하입니다. [Issue #5](https://github.com/Chuseok22/claude-window-keeper/issues/5)에
개선 아이디어가 있습니다(관련 상수를 테스트에서 주입 가능한 `var`로 바꾸기).

## Docker 스모크 테스트 (로컬)

```bash
docker build -t claude-window-keeper:local .
docker run --rm claude-window-keeper:local version
docker run --rm claude-window-keeper:local config init
```

실제 배포 검증(NAS 대상)은 코드 수준에서 끝나지 않는 별도 작업입니다 —
[Issue #6](https://github.com/Chuseok22/claude-window-keeper/issues/6)의 체크리스트를 따르세요.

## 실제 CLI를 호출하는 코드를 만질 때

`Trigger()`(`internal/provider/*.go`)는 진짜 `claude`/`codex` CLI 바이너리를 PTY로 구동합니다. 이 경로는
유닛 테스트로 완전히 커버할 수 없는 부분이라, 관련 코드를 고칠 때는 특히 신중하게 리뷰하세요 — CLI의 실제
동작(TUI 렌더링 타이밍, 프롬프트 텍스트 등)에 의존하는 로직입니다.
