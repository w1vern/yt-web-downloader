package ytdlp

import (
	"reflect"
	"testing"
)

func TestBuildCommandsAudioOpusPlaylist(t *testing.T) {
	o := Options{
		URL: "https://www.youtube.com/watch?v=X&list=PL1", Mode: "audio",
		EmbedThumbnail: true,
		IsPlaylist:     true, Items: []int{1, 2, 3, 5},
	}
	cmds := BuildCommands(o, "socks5h://host.docker.internal:10808", "")
	if len(cmds) != 1 {
		t.Fatalf("want 1 pass, got %d", len(cmds))
	}
	want := []string{
		"https://www.youtube.com/watch?v=X&list=PL1",
		"-f", "ba", "--remux-video", "webm>opus",
		"--embed-thumbnail",
		"-P", ".", "-o", OutputTemplate,
		"-N", "4", "--retries", "2", "--ignore-errors", "--newline",
		"--progress-template", ProgressTemplate,
		"--remote-components", "ejs:github",
		"--proxy", "socks5h://host.docker.internal:10808",
		"--yes-playlist", "-I", "1:3,5",
	}
	if !reflect.DeepEqual(cmds[0], want) {
		t.Errorf("got:\n%v\nwant:\n%v", cmds[0], want)
	}
}

func TestBuildCommandsMerged1080(t *testing.T) {
	o := Options{URL: "https://youtu.be/X", Mode: "merged", VideoRes: "1080", EmbedMetadata: true}
	cmds := BuildCommands(o, "", "")
	if len(cmds) != 1 {
		t.Fatalf("want 1 pass, got %d", len(cmds))
	}
	want := []string{
		"https://youtu.be/X",
		"-f", "bv+ba", "-S", "res:1080,vcodec", "--merge-output-format", "mkv",
		"--embed-metadata",
		"-P", ".", "-o", OutputTemplate,
		"-N", "4", "--retries", "2", "--ignore-errors", "--newline",
		"--progress-template", ProgressTemplate,
		"--remote-components", "ejs:github",
		"--no-playlist",
	}
	if !reflect.DeepEqual(cmds[0], want) {
		t.Errorf("got:\n%v\nwant:\n%v", cmds[0], want)
	}
}

func TestBuildCommandsTagsParseMetadata(t *testing.T) {
	o := Options{
		URL: "https://www.youtube.com/watch?v=X&list=PL1", Mode: "audio",
		TagURLDate: true, TagPlaylist: true, IsPlaylist: true,
	}
	cmds := BuildCommands(o, "", "")
	if len(cmds) != 1 {
		t.Fatalf("want 1 pass, got %d", len(cmds))
	}
	got := cmds[0]
	want := []string{
		"https://www.youtube.com/watch?v=X&list=PL1",
		"-f", "ba", "--remux-video", "webm>opus",
		"-P", ".", "-o", OutputTemplate,
		"-N", "4", "--retries", "2", "--ignore-errors", "--newline",
		"--progress-template", ProgressTemplate,
		"--remote-components", "ejs:github",
		"--parse-metadata", `%(webpage_url)s:(?P<meta_url>.+)`,
		"--parse-metadata", `%(upload_date>%Y-%m-%d)s:(?P<meta_date>\d{4}-\d{2}-\d{2})`,
		"--parse-metadata", `https\://www.youtube.com/playlist?list=%(playlist_id)s:(?P<meta_playlist_url>.+)`,
		"--embed-metadata",
		"--yes-playlist",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got:\n%v\nwant:\n%v", got, want)
	}
}

func TestBuildCommandsBoth(t *testing.T) {
	o := Options{URL: "u", Mode: "both", VideoRes: "best"}
	cmds := BuildCommands(o, "", "")
	if len(cmds) != 2 {
		t.Fatalf("want 2 passes, got %d", len(cmds))
	}
	if got := PassLabels(o); !reflect.DeepEqual(got, []string{"audio", "video"}) {
		t.Errorf("PassLabels = %v", got)
	}
	wantVideoHead := []string{"u", "-f", "bv", "-S", "res,vcodec", "--remux-video", "mkv"}
	if len(cmds[1]) < len(wantVideoHead) || !reflect.DeepEqual(cmds[1][:len(wantVideoHead)], wantVideoHead) {
		t.Errorf("video pass head = %v, want prefix %v", cmds[1], wantVideoHead)
	}
}

func TestBuildCommandsCookies(t *testing.T) {
	o := Options{URL: "u", Mode: "audio"}
	cmds := BuildCommands(o, "", "/cookies/cookies.txt")
	if len(cmds) != 1 {
		t.Fatalf("want 1 pass, got %d", len(cmds))
	}
	found := false
	for i, a := range cmds[0] {
		if a == "--cookies" && i+1 < len(cmds[0]) && cmds[0][i+1] == "/cookies/cookies.txt" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected --cookies /cookies/cookies.txt in args, got: %v", cmds[0])
	}
}

func TestValidate(t *testing.T) {
	bad := []Options{
		{URL: "u", Mode: "nope"},
		{URL: "u", Mode: "video", VideoRes: "999"},
		{URL: "", Mode: "audio"},
	}
	for i, o := range bad {
		if o.Validate() == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
	ok := Options{URL: "u", Mode: "merged", VideoRes: "best"}
	if err := ok.Validate(); err != nil {
		t.Errorf("unexpected: %v", err)
	}
}

func TestTypeSuffix(t *testing.T) {
	if got := TypeSuffix("audio", ".opus", Options{}); got != "audio opus" {
		t.Errorf("got %q", got)
	}
	if got := TypeSuffix("audio", ".m4a", Options{}); got != "audio aac" {
		t.Errorf("got %q", got)
	}
	if got := TypeSuffix("audio", ".webm", Options{}); got != "audio" {
		t.Errorf("got %q", got)
	}
	if got := TypeSuffix("video", "", Options{VideoRes: "1080"}); got != "video 1080p" {
		t.Errorf("got %q", got)
	}
	if got := TypeSuffix("av", "", Options{VideoRes: "best"}); got != "av best" {
		t.Errorf("got %q", got)
	}
}
