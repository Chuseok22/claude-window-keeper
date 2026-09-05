# syntax=docker/dockerfile:1

FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/claude-window-keeper ./cmd/claude-window-keeper

# node:22-bookworm-slim (not debian:bookworm-slim + apt nodejs/npm): Debian
# bookworm's apt repo only ships Node 18.20, but @anthropic-ai/claude-code
# declares engines.node >=22.0.0 (npm install succeeds with just an
# EBADENGINE warning on Node 18, verified at implementation time -- it isn't
# a hard failure today, but shipping a version npm itself flags as
# unsupported is asking for a silent break on the next claude-code release).
FROM node:22-bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl \
    && rm -rf /var/lib/apt/lists/* \
    && npm install -g @anthropic-ai/claude-code @openai/codex \
    # 65536은 Debian의 기본 UID/GID_MAX(60000)를 넘어서, useradd만으로는 GID가
    # 자동으로 UID와 안 맞고 엉뚱한 값으로 떨어진다 (경고만 뜨고 실패하진 않음) -
    # groupadd로 GID를 명시해서 UID:GID를 65536:65536으로 대칭시킨다.
    && groupadd -g 65536 keeper \
    && useradd -m -u 65536 -g 65536 keeper

COPY --from=build /out/claude-window-keeper /usr/local/bin/claude-window-keeper
COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh
# .env is baked into the image by design (spec Sec.9): DISCORD_WEBHOOK_URL
# is a static secret that never needs runtime rewriting, and
# the DockerHub repo hosting this image stays private, so build-time baking
# is acceptable here. CI (Task 15) writes .env into the build context from
# the ENV_FILE secret immediately before `docker build` runs.
#
# `COPY .env* /app/` (wildcard, not `COPY .env /app/.env`) so the build still
# succeeds when no local .env exists -- verified: a wildcard COPY with zero
# matches is a no-op, it does not fail the build the way naming an exact,
# possibly-missing file would. No placeholder .env is committed to the repo:
# .env stays covered by .gitignore's secret-file exclusion (untouched by
# this task), it has just been dropped from .dockerignore below so a local
# or CI-written .env sitting next to this Dockerfile is visible to the build
# context. Local builds with no .env present get no /app/.env at all, so
# entrypoint.sh's `[ -f /app/.env ]` check is false and alerting stays
# silently disabled -- the correct default outside of CI.
COPY .env* /app/

USER keeper
WORKDIR /home/keeper
ENV HOME=/home/keeper
ENV XDG_CONFIG_HOME=/home/keeper/.config

ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["watch"]
