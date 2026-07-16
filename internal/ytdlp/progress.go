package ytdlp

import "strings"

type Event struct {
	Type    string `json:"type"`
	VideoID string `json:"video_id,omitempty"`
	Percent string `json:"percent,omitempty"`
	Speed   string `json:"speed,omitempty"`
	ETA     string `json:"eta,omitempty"`
	Line    string `json:"line,omitempty"`
	State   string `json:"state,omitempty"`
	Error   string `json:"error,omitempty"`
}

// ParseLine turns one line of yt-dlp output into an Event.
// Progress lines match ProgressTemplate: "PRG|<id>|<percent>|<speed>|<eta>".
func ParseLine(line string) Event {
	if rest, ok := strings.CutPrefix(line, "PRG|"); ok {
		p := strings.Split(rest, "|")
		if len(p) == 4 {
			return Event{
				Type:    "progress",
				VideoID: p[0],
				Percent: strings.TrimSpace(p[1]),
				Speed:   strings.TrimSpace(p[2]),
				ETA:     strings.TrimSpace(p[3]),
			}
		}
	}
	return Event{Type: "log", Line: line}
}
