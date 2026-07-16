package ytdlp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"time"
)

type Entry struct {
	Index    int     `json:"index"`
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Channel  string  `json:"channel"`
	Duration float64 `json:"duration"`
}

type Info struct {
	Type       string  `json:"type"`
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	Channel    string  `json:"channel"`
	Duration   float64 `json:"duration"`
	PlaylistID string  `json:"playlist_id,omitempty"`
	Entries    []Entry `json:"entries,omitempty"`
}

func IsPlaylistURL(rawURL string) bool { return PlaylistID(rawURL) != "" }

func PlaylistID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Query().Get("list")
}

// rawInfo covers both yt-dlp -J shapes (video and flat playlist).
type rawInfo struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Channel  string  `json:"channel"`
	Uploader string  `json:"uploader"`
	Duration float64 `json:"duration"`
	Entries  []struct {
		ID       string  `json:"id"`
		Title    string  `json:"title"`
		Channel  string  `json:"channel"`
		Uploader string  `json:"uploader"`
		Duration float64 `json:"duration"`
	} `json:"entries"`
}

func pick(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func parseInfo(data []byte, playlist bool) (*Info, error) {
	var r rawInfo
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse yt-dlp json: %w", err)
	}
	info := &Info{ID: r.ID, Title: r.Title, Channel: pick(r.Channel, r.Uploader), Duration: r.Duration}
	if playlist {
		info.Type = "playlist"
		info.PlaylistID = r.ID
		for i, e := range r.Entries {
			info.Entries = append(info.Entries, Entry{
				Index: i + 1, ID: e.ID, Title: e.Title,
				Channel: pick(e.Channel, e.Uploader), Duration: e.Duration,
			})
		}
	} else {
		info.Type = "video"
	}
	return info, nil
}

// Resolve runs yt-dlp -J to fetch metadata for the options form. 90s timeout.
func Resolve(ctx context.Context, rawURL, proxy, cookies string) (*Info, error) {
	playlist := IsPlaylistURL(rawURL)
	args := []string{rawURL, "-J"}
	if playlist {
		args = append(args, "--flat-playlist", "--yes-playlist")
	} else {
		args = append(args, "--no-playlist")
	}
	if proxy != "" {
		args = append(args, "--proxy", proxy)
	}
	if cookies != "" {
		args = append(args, "--cookies", cookies)
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := errb.String()
		if len(msg) > 500 {
			msg = msg[len(msg)-500:]
		}
		return nil, fmt.Errorf("yt-dlp failed: %s", msg)
	}
	info, err := parseInfo(out.Bytes(), playlist)
	if err != nil {
		return nil, err
	}
	if playlist {
		info.PlaylistID = pick(PlaylistID(rawURL), info.PlaylistID)
	}
	return info, nil
}
