package jobs

import "testing"

func TestFinalName(t *testing.T) {
	cases := [][3]string{
		{"Chan - Title [abcdefghijk].opus", "audio opus", "Chan - Title - audio opus.ogg"},
		{"Chan - Title [abcdefghijk].mp3", "audio mp3", "Chan - Title - audio mp3.mp3"},
		{"C - T [abcdefghijk].mp4", "video 1080p", "C - T - video 1080p.mp4"},
		{"No Id Here.mkv", "av best", "No Id Here - av best.mkv"},
	}
	for _, c := range cases {
		if got := finalName(c[0], c[1]); got != c[2] {
			t.Errorf("finalName(%q,%q) = %q, want %q", c[0], c[1], got, c[2])
		}
	}
}

func TestEscapeFFMetadataValue(t *testing.T) {
	got := escapeFFMetadataValue("a=b;c#d\\e\nf")
	want := "a\\=b\\;c\\#d\\\\e\\\nf"
	if got != want {
		t.Errorf("escapeFFMetadataValue(...) = %q, want %q", got, want)
	}
}
