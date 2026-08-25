# 알려진 미해결 이슈

2026-08-25 리라이트의 최종 whole-branch 리뷰에서 나온 항목들입니다. 배포를 막는 Critical 4개는 이미 고쳐서
`main`에 있습니다(아래 "이미 고쳐진 것" 참고) — 여기 남은 건 전부 follow-up으로 GitHub Issues에 등록됨.

## 열려있는 이슈

| # | 제목 | 요약 |
|---|---|---|
| [#1](https://github.com/Chuseok22/claude-window-keeper/issues/1) | scheduler 재시도/backoff 보강 | `Trigger()` 검증 실패가 반복돼도 backoff가 escalate 안 됨 — 최악의 경우 quota를 계속 태우는 재시도 루프 가능 |
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

## 작업 시작 전 체크

새 기능/버그 수정을 시작하기 전에, 이미 알려진 문제를 또 발견해서 새 이슈를 만드는 일이 없도록 위 표를
먼저 훑어보세요. 특히 `internal/scheduler/scheduler.go`, `internal/cli/watch_lock.go`,
`.github/workflows/PROJECT-GO-SIMPLE-CICD.yaml`을 건드릴 계획이면 관련 이슈부터 확인하세요.
