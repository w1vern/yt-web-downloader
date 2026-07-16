package ytdlp

import "fmt"

type Options struct {
	URL            string `json:"url"`
	Mode           string `json:"mode"`
	VideoRes       string `json:"video_res"`
	EmbedThumbnail bool   `json:"embed_thumbnail"`
	EmbedMetadata  bool   `json:"embed_metadata"`
	TagURLDate     bool   `json:"tag_url_date"`
	TagPlaylist    bool   `json:"tag_playlist"`
	IsPlaylist     bool   `json:"is_playlist"`
	Items          []int  `json:"items"`
}

func (o *Options) needsAudio() bool { return o.Mode == "audio" || o.Mode == "both" }
func (o *Options) needsVideo() bool { return o.Mode == "video" || o.Mode == "both" || o.Mode == "merged" }

func (o *Options) Validate() error {
	if o.URL == "" {
		return fmt.Errorf("url is required")
	}
	switch o.Mode {
	case "audio", "video", "both", "merged":
	default:
		return fmt.Errorf("bad mode %q", o.Mode)
	}
	if o.needsVideo() {
		switch o.VideoRes {
		case "best", "2160", "1440", "1080", "720":
		default:
			return fmt.Errorf("bad video_res %q", o.VideoRes)
		}
	}
	return nil
}
