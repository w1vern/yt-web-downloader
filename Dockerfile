FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /yt-web-downloader .

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ffmpeg ca-certificates curl unzip \
    && rm -rf /var/lib/apt/lists/*
# deno — JS runtime for yt-dlp --remote-components ejs:github
RUN curl -fsSL https://deno.land/install.sh | DENO_INSTALL=/usr/local sh -s -- --yes \
    && deno --version
# baked-in yt-dlp (fallback; entrypoint tries to self-update on start)
RUN curl -fL https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp_linux \
        -o /usr/local/bin/yt-dlp \
    && chmod +x /usr/local/bin/yt-dlp \
    && yt-dlp --version
COPY --from=build /yt-web-downloader /usr/local/bin/yt-web-downloader
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
# no EXPOSE: the listen port is runtime-configurable via PORT (.env), see compose.yml
ENTRYPOINT ["/entrypoint.sh"]
