#!/bin/sh
set -u
YTDLP_URL="https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp_linux"

if command -v yt-dlp >/dev/null 2>&1; then
    # Baked-in binary present: try to self-update, keep current version on failure.
    echo "[entrypoint] updating yt-dlp..."
    yt-dlp ${PROXY_URL:+--proxy "$PROXY_URL"} -U \
        || echo "[entrypoint] yt-dlp update failed, using $(yt-dlp --version)"
else
    # Image was built without network access to github — bootstrap now (via proxy if set).
    echo "[entrypoint] yt-dlp not baked in, downloading..."
    if curl -fL --retry 3 ${PROXY_URL:+--proxy "$PROXY_URL"} "$YTDLP_URL" -o /usr/local/bin/yt-dlp \
        && chmod +x /usr/local/bin/yt-dlp; then
        echo "[entrypoint] installed yt-dlp $(yt-dlp --version)"
    else
        rm -f /usr/local/bin/yt-dlp
        echo "[entrypoint] ERROR: could not download yt-dlp — downloads will fail until the container restarts with working network/proxy"
    fi
fi
exec yt-web-downloader
