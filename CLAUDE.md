# CLAUDE.md

이 파일은 이 저장소에서 작업하는 모든 Claude Code 세션(과 사람)을 위한 진입점입니다. 더 자세한 내용은
`.claude/rules/*.md`에 번호 순서로 정리돼 있습니다 — 작업 전에 관련 파일을 먼저 읽으세요.

## 이 프로젝트가 뭔가

`claude-window-keeper`는 [wavever/CCLimitPing](https://github.com/wavever/CCLimitPing)(MIT)을 fork해서, **개인
소유 Synology NAS 위 Docker 컨테이너 단 하나에서만** 24/7 동작하도록 범위를 극단적으로 좁힌 Go CLI 데몬입니다.
Claude Code / Codex / Spark 구독의 5시간 rate-limit window가 리셋되는 즉시 최소한의 요청을 보내 다음 window를
이어붙입니다.

다른 사람에게 배포하거나 공개 서비스로 만들 계획은 없습니다. 이 사실이 이 저장소의 거의 모든 설계 결정(Docker
전용 배포, macOS/Windows 코드 제거, 한국어 단일 CLI, "1인 운영이니 감수 가능한 리스크" 판단 등)의 근거입니다.

**중요**: `docs/`는 `.gitignore`로 제외돼 있습니다. 이 저장소를 새로 clone하면 원래의 spec/plan 문서는
따라오지 않습니다 — 이 `CLAUDE.md`와 `.claude/rules/*.md`가 그 내용을 대체하는 **유일하게 git에 남는 진짜
소스**입니다. 새로운 설계 결정이 생기면 여기를 갱신하세요.

## 지금 상태 (2026-08-25 기준)

- 원본 CCLimitPing → `claude-window-keeper`로의 전체 리라이트가 15개 태스크로 완료되어 `main`에 push됨.
- 최종 리뷰에서 발견된 배포-차단급 Critical 4개는 이미 수정 완료(README chown 안내, 자격증명 write-back 로깅,
  watch.lock PID1 처리, `authNotified` 레이스 수정).
- 남은 개선사항은 GitHub Issues로 추적 중입니다. **실제 NAS 배포/운영 검증(Issue #6)은 아직 안 됨** — 코드는
  완성됐지만 실사용 검증 전입니다.

## 절대 잊지 말아야 할 것

1. **`main`에 push하면 실제로 배포됩니다.** `project-auto-wizard`가 깐 CI/CD가 trunk-based라서, `main`에 올라가는
   모든 push가 자동으로 버전 bump + GitHub Release + Docker 빌드 + DockerHub push + NAS SSH 배포까지 이어집니다.
   실험적인 커밋을 가볍게 push하지 마세요. 자세한 내용: `.claude/rules/20-cicd-deployment.md`.
2. **`.github/workflows/PROJECT-*.yaml`는 대부분 `project-auto-wizard`가 관리합니다.** 파일 상단에
   `# project-auto-wizard:managed-workflow` 표시가 있는 파일의 `jobs:`/`steps:` 로직은 직접 고치지 마세요 — 다음
   마법사 업데이트 때 되돌아갈 수 있습니다. `env:` 블록(프로젝트별 설정 구간)만 안전하게 수정 가능한 영역입니다.
3. **DockerHub 저장소는 반드시 private이어야 합니다.** Discord webhook URL이 이미지에 구워지기 때문입니다
   (`.claude/rules/20-cicd-deployment.md` 참고).
4. **HTTP 서버가 없습니다.** 이 데몬은 포트를 열지 않습니다. 뭔가 HTTP 헬스체크를 되살리려는 시도를 보면
   의심하세요 — 마법사 워크플로우 기본값이 HTTP 서비스를 가정하고 있어서 잘못 되돌아갈 수 있습니다.
5. **CLI 텍스트는 한국어 단일 언어입니다** (`internal/cli/i18n.go`). 영어/중국어 이중언어로 되돌리지 마세요 —
   의도된 단순화입니다.

## 빠른 참조

| 알고 싶은 것 | 어디를 보면 되나 |
|---|---|
| 프로젝트 목표, provider 범위, 비목표 | `.claude/rules/00-project-overview.md` |
| 패키지 구조, 핵심 타입/인터페이스 | `.claude/rules/10-architecture.md` |
| CI/CD 파이프라인, Docker, 시크릿 관리 | `.claude/rules/20-cicd-deployment.md` |
| 빌드/테스트/검증 방법 | `.claude/rules/30-testing-and-verification.md` |
| git/커밋/리뷰 관례 | `.claude/rules/40-git-and-delivery.md` |
| 원 프로젝트와의 관계, 라이선스 | `ATTRIBUTION.md`, `LICENSE` |
