# yt-web-downloader

Self-hosted web UI over [yt-dlp](https://github.com/yt-dlp/yt-dlp) for downloading audio/video from YouTube
(single videos and playlists). Runs in Docker on a home server, sits behind an
externally-managed Traefik reverse proxy, and is meant for a small number of
trusted users (owner + a few friends) behind a single shared login.

## Quick start

```sh
cp .env.example .env
# edit .env — at minimum set AUTH_PASSWORD and SESSION_SECRET
docker compose up -d --build
```

Then open <http://localhost:8080>.

Downloaded files are written to `./data` (mounted as `/data` in the
container) and are served back through the web UI until they expire.

## How downloads work

Files are passed through as YouTube serves them — nothing is transcoded.
Pick any combination of audio and video, plus an optional lossless merge:

- **Audio** — remuxed only, never re-encoded: `opus` streams end up as
  `.ogg`, `aac` streams end up as `.m4a`.
- **Video** — capped at your chosen max resolution (best/2160/1440/1080/720);
  within that cap yt-dlp prefers codecs in the order av1 → vp9 → h265 → h264.
  Always packaged as `.mkv` (lossless remux — mkv holds any codec and, unlike
  webm, supports embedded thumbnails and full tags). Note: iOS/Safari and
  browsers don't play mkv natively; use VLC/mpv/Kodi-class players.
- **Merge** — available only when both Audio and Video are selected; combines
  them losslessly into a single `.mkv`.

## Environment variables

| Variable              | Default                                     | Description                                                                                            |
| ---------------------- | -------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| `AUTH_LOGIN`          | —  (required)                                | Shared login for all users                                                                              |
| `AUTH_PASSWORD`       | —  (required)                                | Shared password for all users                                                                            |
| `SESSION_SECRET`      | —  (required)                                | HMAC key used to sign the session cookie, e.g. `openssl rand -hex 32`                                    |
| `PROXY_URL`           | empty (no proxy)                             | Proxy used for yt-dlp downloads, the YouTube Data API, and yt-dlp's own self-update, e.g. `socks5h://host.docker.internal:10808` |
| `GOOGLE_API_KEY`      | empty                                        | YouTube Data API key; optional, enables the "playlist added date" tag/metadata option                    |
| `FILE_TTL`            | `1h`                                         | How long finished job files are kept before the janitor deletes them (Go duration: `1h`, `30m`, `24h`, …) |
| `MAX_CONCURRENT_JOBS` | `2`                                          | Number of download jobs processed in parallel                                                            |
| `PORT`                | `8080`                                       | HTTP port the app listens on inside the container                                                        |
| `DATA_DIR`            | `/data`                                      | Root directory for job files (backed by the `./data` volume)                                             |
| `COOKIES_FILE`        | empty (no cookies)                           | Path *inside the container* to a Netscape `cookies.txt` for YouTube, e.g. `/cookies/cookies.txt`; see [YouTube cookies (optional)](#youtube-cookies-optional) |

Copy `.env.example` to `.env` and fill in the required values before starting
the container — `.env` is gitignored and must never be committed.

## YouTube cookies (optional)

YouTube sometimes rate-limits or shows a bot-check to signed-out requests,
and private/age-restricted/members-only videos need an authenticated session
at all. Supplying a cookies file works around both.

### Getting a working cookies.txt, step by step

Use a **secondary Google account**, not your main one — accounts whose
cookies are used by downloaders occasionally get flagged.

**Option A — browser extension:**

1. Open a **private/incognito window** and sign in to youtube.com with the
   secondary account. (Why private: YouTube rotates session cookies in any
   window that stays open. If you export from your normal browser session,
   the browser refreshes the session in the background and the exported
   copy goes stale within hours. A private window that you close right
   after exporting leaves the exported session "frozen" and valid.)
2. Install a Netscape-format export extension — e.g. **"Get cookies.txt
   LOCALLY"** (Chrome) or **"cookies.txt"** (Firefox) — open any YouTube
   page and export cookies for `youtube.com`.
3. **Close the private window** without logging out.

**Option B — no extension, any desktop with yt-dlp:** while logged in to
YouTube in your regular browser (still better under the secondary account),
run:

```
yt-dlp --cookies-from-browser firefox --cookies cookies.txt --simulate "https://www.youtube.com/"
```

(`firefox` can be `chrome`, `edge`, etc.) This dumps that browser's YouTube
session into `cookies.txt`. Note the same staleness caveat: the browser will
rotate this session while it keeps running, so Option A is more durable.

**Then install the file:**

4. Put it at `./cookies/cookies.txt` on the host (this repo's `compose.yml`
   bind-mounts `./cookies` to `/cookies` in the container).
5. Set `COOKIES_FILE=/cookies/cookies.txt` in `.env` and
   `docker compose up -d` (recreates the container with the new env).
6. Verify:
   `docker compose exec yt-web-downloader yt-dlp --cookies /cookies/cookies.txt --simulate "https://www.youtube.com/watch?v=dQw4w9WgXcQ"`
   should finish without a "Sign in to confirm you're not a bot" error.

yt-dlp reads *and rewrites* this file as it refreshes the session, so the
mount must stay read-write — don't make it read-only. You normally don't
need to touch the file again; only re-export and replace it once auth errors
reappear in a job's log.

## Notes

- **Traefik is external.** This repo's `compose.yml` only publishes port
  `8080` on the host; put a reverse proxy (e.g. Traefik) in front of it for
  TLS/routing if exposing it beyond localhost. Traefik itself is not part of
  this project and is not started by `compose.yml`.
- **Files expire.** Every finished job's output lives under `DATA_DIR` for
  `FILE_TTL` after completion, then a background janitor deletes it. Download
  what you need before it expires.
- **A proxy is usually required for YouTube.** Depending on your server's
  network, YouTube may rate-limit or block datacenter/residential IPs, so
  `PROXY_URL` should normally point at a working SOCKS5/HTTP proxy reachable
  from inside the container (the default in `.env.example` assumes a proxy
  running on the Docker host, reached via `host.docker.internal`).
- **yt-dlp self-updates on start.** The image bakes in a recent `yt-dlp`
  build, but `entrypoint.sh` tries to update it to the latest release (via
  `PROXY_URL` if set) every time the container starts, falling back to the
  baked-in version if the update fails.
