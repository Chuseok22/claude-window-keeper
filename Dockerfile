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
    && useradd -m -u 1001 keeper

COPY --from=build /out/claude-window-keeper /usr/local/bin/claude-window-keeper
COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh
# .env is never baked into the image on purpose: it's excluded via .dockerignore
# and there is no COPY for it here, so `docker build .` succeeds identically
# whether or not a local .env exists next to this Dockerfile. Secrets
# (TELEGRAM_BOT_TOKEN / TELEGRAM_CHAT_ID) belong in the container's runtime
# environment, not in an image layer that could be pulled/inspected later --
# pass them with `docker run -e TELEGRAM_BOT_TOKEN=... -e TELEGRAM_CHAT_ID=...`
# or `docker run --env-file .env` at deploy time (Task 15). entrypoint.sh's
# `/app/.env` sourcing step still exists as a fallback for anyone who builds a
# custom variant of this image with a COPY .env line added back in.

USER keeper
WORKDIR /home/keeper
ENV HOME=/home/keeper
ENV XDG_CONFIG_HOME=/home/keeper/.config

ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["watch"]
