# 프로젝트 개요

## 무엇을 하는 프로젝트인가

`claude-window-keeper`는 Claude Code, Codex, Spark 구독의 **5시간 rolling rate-limit window**를 감시하다가,
window가 리셋되는 순간 공식 CLI(`claude`, `codex`)를 통해 최소한의 요청을 하나 보내 다음 window를 즉시
시작시키는 Go 데몬입니다. 창이 리셋됐는데 사람이 몇 시간 뒤에나 다시 그 도구를 쓰면, 그만큼 창 스케줄이
밀리는 낭비가 생기는데 이걸 막는 게 유일한 목적입니다.

## 배경

[wavever/CCLimitPing](https://github.com/wavever/CCLimitPing)(MIT License)을 fork했습니다. 원본은
macOS/Linux/Windows 크로스플랫폼으로 로컬 대화형 사용까지 지원하는 범용 도구였습니다. 이 fork는 그 범위를
**"개인 소유 Synology NAS(DS918+, Intel Celeron J3455, x86_64/amd64) 위 Docker 컨테이너 단 하나"** 로 극단적으로
좁혔습니다. 원 프로젝트와의 정확한 관계는 `ATTRIBUTION.md`를 보세요.

## 목표

- Claude/Codex/Spark 5h window를 끊김 없이 이어붙인다.
- 사람 개입 없이 24/7 무인 운영된다.
- 인증이 완전히 죽으면(재로그인 필요) Discord로 프로세스 실행 중 provider당 딱 한 번 알린다(`authNotified`가 인메모리 상태라 재시작하면 다시 알림 가능).
- 그 외에는 최대한 조용히, 로그만 남기며 동작한다.

## 비목표 (의도적으로 안 하는 것)

- **공개 배포/멀티유저 지원 안 함.** 이 도구는 순수 개인 유틸입니다. 다른 사람이 쓰게 만들 계획이 없어서,
  Anthropic/OpenAI의 subscription 약관·정책 문제도 이번 스코프에서 의도적으로 고려하지 않았습니다.
- **HTTP 서버 없음.** 웹 UI, 대시보드, REST API 전부 없음. 순수 백그라운드 루프 하나입니다.
- **macOS/Windows 지원 없음.** Linux/amd64 단일 타깃. 크로스플랫폼 코드(macOS Keychain, `osascript` 알림,
  Windows 프로세스 관리 등)는 전부 제거했습니다.
- **arm64 빌드 없음.** 대상 NAS가 x86_64라서 불필요합니다.
- **바이너리 배포 없음.** `install.sh`, goreleaser 전부 제거. Docker 이미지가 유일한 배포 형태입니다.

## Provider 범위

Claude / Codex / Spark **3개 모두 유지**합니다(제거 후보로 논의됐지만 최종적으로 전부 남기기로 결정).
Codex의 reset-credit 재사용(`redeem` 명령어)도 유지합니다.

## 남는 명령어 (전체)

`status`, `ping`, `watch`, `redeem`, `config`(`init`/`path`), `version` — 이게 전부입니다. 원본에 있던
`bg`(백그라운드 데몬 관리), `continue`(대화형 세션 프록시), `hooks`/`hook`(로컬 세션 활동 감지),
`schedule`(별도 cron식 트리거), `upgrade`(자체 업데이트), `uninstall`은 전부 제거됐습니다 — 전부 "로컬
대화형 사용" 또는 "자체 바이너리 배포"를 전제로 한 기능이라, Docker 컨테이너 하나로 좁힌 이 프로젝트에는
대상 자체가 없기 때문입니다. 새로 이런 종류의 기능을 다시 추가하려는 요청이 있으면, 왜 원래 제거했는지부터
확인하세요.

## 운영 모델

- 컨테이너는 `claude-window-keeper watch`를 entrypoint로 실행하는 단일 프로세스.
- Docker가 곧 프로세스 supervision(재시작 정책은 아직 없음 — Issue #2)이라, 원본에 있던 자체 데몬 관리
  기능(`bg`)이 필요 없습니다.
- 인증은 Claude Code/Codex CLI가 이미 만든 자격증명 파일(`~/.claude/.credentials.json`,
  `~/.codex/auth.json`)을 그대로 재사용합니다. 이 프로젝트는 자체 로그인 절차를 구현하지 않습니다 — 다른
  머신(맥)에서 로그인한 뒤 파일을 NAS로 복사해서 씁니다.
