# Git / 커밋 / 리뷰 관례

## 브랜치

이 저장소는 GitHub 저장소 이름과 실제 배포 브랜치 모두 `main` 하나입니다. `develop` 브랜치는 없습니다
(`version.yml`의 `metadata.template.branches.develop: "main"`도 `main`을 가리킵니다). 큰 리라이트 작업도
`main`에서 직접(사용자의 명시적 승인 하에) 진행된 전례가 있습니다.

**하지만 이것이 "항상 feature 브랜치 없이 main에서 바로 작업해도 된다"는 뜻은 아닙니다.** 이전 세션에서는
매번 사용자에게 명시적으로 확인받고 진행했습니다. `main` push는 곧 실제 배포로 이어지므로(위
`20-cicd-deployment.md` 참고), 브랜치 전략에 대해 확신이 없으면 먼저 물어보세요.

## 커밋 메시지

전형적인 conventional commits 접두사(`feat:`, `fix:` 등)를 엄격히 강제하지는 않지만, 각 커밋이 "무엇을
왜"를 한 줄 요약으로 설명하는 관례를 따릅니다(예: `remove goreleaser (superseded by project-auto-wizard's
release/deploy pipeline)`, `guard Scheduler.authNotified with a mutex`). `PROJECT-COMMON-RELEASE-PUBLISH.yaml`이
커밋 로그 기반으로 semver를 자동 판정하니(feat → minor, `!` → major, 그 외 → patch), 커밋 메시지가
실제로 버전 번호에 영향을 줍니다.

## 절대 하지 말 것 (전역 규칙, 이 프로젝트에도 그대로 적용)

- `git push --force`, `git reset --hard`, `git clean -fd`, `--no-verify`, `--no-gpg-sign` 등 어떤 형태의
  force/우회 플래그도 사용자의 명시적 요청 없이는 절대 쓰지 않습니다.
- `.gitignore`로 제외된 파일(`.env`, `config.toml` 등)은 제외된 상태를 존중합니다 — 커밋이 필요해 보여도
  강제로 추적시키지 말고 먼저 사용자에게 확인하세요. 실제로 최종 리뷰 fix wave 중 이 원칙 때문에 구현
  에이전트가 원래 지시받은 방식(`.env` placeholder를 git에 커밋)을 거부하고 `.gitignore`를 안 건드리는
  대안(`COPY .env* /app/` 와일드카드)을 스스로 찾은 전례가 있습니다 — 이게 맞는 판단이었습니다.

## `docs/`는 gitignore 대상입니다

`docs/superpowers/specs/`, `docs/superpowers/plans/` 아래에 이 리라이트의 원본 spec/plan 문서가 로컬에는
남아있을 수 있지만, git에는 안 올라갑니다. **새로운 설계 결정이나 계획 문서를 작성할 거라면, 최종적으로
남겨야 할 지식은 이 `.claude/rules/*.md`나 `CLAUDE.md`, `README.md`에 반영하세요** — 안 그러면 다음 clone에서
사라집니다.

## 리뷰 프로세스 (이 프로젝트에서 실제로 썼던 방식)

큰 변경 작업은 `superpowers:subagent-driven-development` 방식(태스크별 fresh subagent 구현 → task reviewer →
최종 whole-branch reviewer)으로 진행된 전례가 있습니다. 리뷰는 항상 "구현자의 보고를 그대로 믿지 말고 diff를
직접 확인" 원칙을 따랐고, 실제로 그 덕에 여러 실질적인 문제(UID/볼륨 권한 불일치, watch.lock 크래시루프,
`authNotified` 데이터 레이스 등)를 병합 전에 잡아냈습니다. 비슷한 규모의 변경을 할 때는 이 패턴을 참고하세요.
