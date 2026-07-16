#!/bin/sh
set -u
# Try to update yt-dlp to the latest release; keep baked-in version on failure.
if [ -n "${PROXY_URL:-}" ]; then
    echo "[entrypoint] updating yt-dlp via proxy..."
    yt-dlp --proxy "$PROXY_URL" -U || echo "[entrypoint] yt-dlp update failed, using baked-in $(yt-dlp --version)"
else
    echo "[entrypoint] updating yt-dlp..."
    yt-dlp -U || echo "[entrypoint] yt-dlp update failed, using baked-in $(yt-dlp --version)"
fi
exec yt-web-downloader
