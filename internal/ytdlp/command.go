package ytdlp

import "yt-web-downloader/internal/ranges"

const OutputTemplate = "%(channel)s - %(title)s [%(id)s].%(ext)s"
const ProgressTemplate = "download:PRG|%(info.id)s|%(progress._percent_str)s|%(progress._speed_str)s|%(progress._eta_str)s"

// sortSpec builds the yt-dlp -S sort: resolution dominates (capped unless "best"),
// codec preference av01 > vp9 > h265 > h264 breaks ties within a resolution.
func sortSpec(res string) string {
	if res == "best" {
		return "res,vcodec"
	}
	return "res:" + res + ",vcodec"
}

func (o Options) embeds() []string {
	var a []string
	if o.EmbedThumbnail {
		a = append(a, "--embed-thumbnail")
	}
	if o.EmbedMetadata {
		a = append(a, "--embed-metadata")
	}
	return a
}

func commonArgs(o Options, proxy, cookies string) []string {
	a := []string{
		"-P", ".", "-o", OutputTemplate,
		"-N", "4", "--retries", "2", "--ignore-errors", "--newline",
		"--progress-template", ProgressTemplate,
		"--remote-components", "ejs:github",
	}
	tagged := false
	if o.TagURLDate {
		a = append(a,
			"--parse-metadata", `%(webpage_url)s:(?P<meta_url>.+)`,
			"--parse-metadata", `%(upload_date>%Y-%m-%d)s:(?P<meta_date>\d{4}-\d{2}-\d{2})`,
		)
		tagged = true
	}
	if o.TagPlaylist && o.IsPlaylist {
		a = append(a, "--parse-metadata", `https\://www.youtube.com/playlist?list=%(playlist_id)s:(?P<meta_playlist_url>.+)`)
		tagged = true
	}
	if tagged && !o.EmbedMetadata {
		a = append(a, "--embed-metadata")
	}
	if proxy != "" {
		a = append(a, "--proxy", proxy)
	}
	if cookies != "" {
		a = append(a, "--cookies", cookies)
	}
	if o.IsPlaylist {
		a = append(a, "--yes-playlist")
		if len(o.Items) > 0 {
			a = append(a, "-I", ranges.ToYtdlp(o.Items))
		}
	} else {
		a = append(a, "--no-playlist")
	}
	return a
}

// BuildCommands returns one arg slice per yt-dlp pass; run as exec("yt-dlp", args...) with cmd.Dir = job dir.
func BuildCommands(o Options, proxy, cookies string) [][]string {
	var passes [][]string
	if o.needsAudio() {
		f := []string{"-f", "ba", "--remux-video", "webm>opus"}
		passes = append(passes, append([]string{o.URL}, append(f, o.embeds()...)...))
	}
	if o.Mode == "video" || o.Mode == "both" {
		f := []string{"-f", "bv", "-S", sortSpec(o.VideoRes), "--remux-video", "mkv"}
		passes = append(passes, append([]string{o.URL}, append(f, o.embeds()...)...))
	}
	if o.Mode == "merged" {
		f := []string{"-f", "bv+ba", "-S", sortSpec(o.VideoRes), "--merge-output-format", "mkv"}
		passes = append(passes, append([]string{o.URL}, append(f, o.embeds()...)...))
	}
	for i := range passes {
		passes[i] = append(passes[i], commonArgs(o, proxy, cookies)...)
	}
	return passes
}

func PassLabels(o Options) []string {
	switch o.Mode {
	case "audio":
		return []string{"audio"}
	case "video":
		return []string{"video"}
	case "both":
		return []string{"audio", "video"}
	default:
		return []string{"av"}
	}
}

func resLabel(res string) string {
	if res == "best" {
		return "best"
	}
	return res + "p"
}

// TypeSuffix is the human filename suffix for a pass: "audio opus", "video 1080p", "av best".
func TypeSuffix(pass, ext string, o Options) string {
	if pass == "audio" {
		switch ext {
		case ".opus", ".ogg":
			return "audio opus"
		case ".m4a":
			return "audio aac"
		default:
			return "audio"
		}
	}
	return pass + " " + resLabel(o.VideoRes)
}
