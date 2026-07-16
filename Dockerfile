FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /yt-web-downloader .

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ffmpeg ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*
# deno — JS runtime for yt-dlp --remote-components ejs:github.
# Taken from the official image via the registry instead of deno.land's install
# script: build hosts often can't reach deno.land/github directly (no proxy at
# build time), which made this step hang for minutes.
COPY --from=denoland/deno:bin /deno /usr/local/bin/deno
RUN deno --version
# Best-effort baked-in yt-dlp. If github is unreachable from the build host the
# build still succeeds (fast fail via timeouts) and entrypoint.sh bootstraps
# yt-dlp at first start through PROXY_URL instead.
RUN (curl -fL --retry 2 --connect-timeout 10 --max-time 120 \
        https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp_linux \
        -o /usr/local/bin/yt-dlp \
    && chmod +x /usr/local/bin/yt-dlp \
    && yt-dlp --version) \
    || { rm -f /usr/local/bin/yt-dlp; echo "WARN: yt-dlp not baked in; entrypoint will download it at first start via PROXY_URL"; }
COPY --from=build /yt-web-downloader /usr/local/bin/yt-web-downloader
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
# no EXPOSE: the listen port is runtime-configurable via PORT (.env), see compose.yml
ENTRYPOINT ["/entrypoint.sh"]
