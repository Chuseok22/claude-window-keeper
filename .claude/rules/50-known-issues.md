# 알려진 미해결 이슈

2026-08-25 리라이트의 최종 whole-branch 리뷰에서 나온 항목들입니다. 배포를 막는 Critical 4개는 이미 고쳐서
`main`에 있습니다(아래 "이미 고쳐진 것" 참고) — 여기 남은 건 전부 follow-up으로 GitHub Issues에 등록됨.

## 열려있는 이슈

| # | 제목 | 요약 |
|---|---|---|
| [#2](https://github.com/Chuseok22/claude-window-keeper/issues/2) | 컨테이너 `--restart` 정책 없음 | NAS 재부팅/크래시 시 다음 `main` push 전까지 데몬이 안 살아남 |
| [#3](https://github.com/Chuseok22/claude-window-keeper/issues/3) | 리브랜딩 잔재 정리 | `config init`이 생성하는 파일에 "limitping" 문구, 죽은 `ContinuePrompt` 필드 등 |
| [#4](https://github.com/Chuseok22/claude-window-keeper/issues/4) | 문서 보완 | README에 DockerHub private 요구사항, 로컬 `.env` 주의사항 누락 |
| [#5](https://github.com/Chuseok22/claude-window-keeper/issues/5) | 잡다한 개선사항 | 버전 하드코딩(ldflags 미배선), 45초짜리 느린 테스트, 미사용 curl, env-var 배선 테스트 공백 등 |
| [#6](https://github.com/Chuseok22/claude-window-keeper/issues/6) | **실제 NAS 배포/운영 검증** | 코드는 완성됐지만 실제 NAS에 배포해서 검증하는 작업은 아직 안 함 — 다음 배포 관련 작업을 시작하기 전에 먼저 이 체크리스트부터 |

## 이미 고쳐진 것 (2026-08-25 최종 리뷰 fix wave, 참고용 기록)

과거에 "4개 알려진 버그"로 추적하던 항목 중 2개는 실제로는 배포 자체를 막는 수준으로 심각하다는 게 최종
리뷰에서 드러나서 즉시 고쳐졌습니다:

- **`authNotified` 데이터 레이스**: 기본 설정(claude+codex 둘 다 활성화)에서 시작하자마자 크래시할 수 있는
  수준이었음 → `internal/scheduler/scheduler.go`에 `sync.Mutex` 추가로 해결.
- **watch.lock/PID1 크래시루프**: 이번 리라이트의 볼륨 마운트 결정(`$HOME`을 통째로 마운트) 때문에 lock
  파일이 재배포 사이에 살아남게 돼서, **두 번째 재배포부터 무조건 죽는** 문제로 악화됐던 걸 발견 →
  `internal/cli/watch_lock.go`가 자기 자신의 PID를 stale로 정확히 판별하도록 수정.

같은 fix wave에서 새로 발견돼서 함께 고친 것:
- UID(1001)/NAS 볼륨(root 소유) 불일치로 자격증명 읽기/쓰기가 조용히 실패하던 문제 → README에 `chown`
  안내 추가.
- 자격증명 write-back 에러가 로그도 없이 무시되던 문제 → `internal/auth/claude.go`, `codex.go`에 로깅 추가.

## 2026-08-26 fix: scheduler 검증-실패 backoff/cap

**[#1](https://github.com/Chuseok22/claude-window-keeper/issues/1) scheduler 재시도/backoff 보강**: 검증
실패(`Trigger()`가 err==nil을 반환했지만 postPingGrace 후에도 5h 창이 안 열린 경우) 시 늘린 `backoff`가 루프
상단의 `ReadUsage` 성공 시 무조건 `minBackoff`로 리셋되던 버그를 고쳤습니다. `internal/scheduler/scheduler.go`의
`runTarget`에 검증 실패 전용 `verifyBackoff`(공용 `backoff`와 분리되어 리셋되지 않음)와 연속 실패
카운터(`verifyFailures`)를 추가: `maxVerifyFailures`(3회) 미만이면 기존처럼 즉시 재시도를 허용하되 이제
30s→60s→120s로 실제 escalate하고, 3회에 도달하면 `lastPingAt`을 리셋하지 않아 기존 "중복 핑 방지" 가드가
자연스럽게 발동해 다음 자연 창 주기(보통 5h)까지 대기하도록 함(알림은 보내지 않고 로그만 남김). fable5
최종 리뷰에서 나온 후속 수정: (1) 5h 창이 다시 활성으로 관측되는 시점에 `verifyFailures`/`verifyBackoff`를
리셋해서, 지연 반영된 검증 성공이 다음 에피소드의 카운트에 잘못 이어붙는 걸 막음, (2) cap 도달 회차의
로그/라이브 상태 문구가 "재시도 예정"처럼 보이지 않도록 분리. 부수적으로 `verr != nil`(읽기 자체 실패)과
`!vu.FiveHour.Active()`(창이 안 열림) 로그 메시지도 구분함. 회귀 테스트:
`TestRunTarget_VerifyFailureCap_StopsRetriggeringAfterMaxFailures`(cap이 실제로 재시도를 막는지까지
검증하도록 대기시간을 보강함 — 초기 버전은 3~4번째 트리거 사이 최소 간격보다 짧게 대기해서 cap 유무를
구분하지 못하는 공허한 단언이었음, fable5 리뷰에서 발견).

## 작업 시작 전 체크

새 기능/버그 수정을 시작하기 전에, 이미 알려진 문제를 또 발견해서 새 이슈를 만드는 일이 없도록 위 표를
먼저 훑어보세요. 특히 `internal/scheduler/scheduler.go`, `internal/cli/watch_lock.go`,
`.github/workflows/PROJECT-GO-SIMPLE-CICD.yaml`을 건드릴 계획이면 관련 이슈부터 확인하세요.
