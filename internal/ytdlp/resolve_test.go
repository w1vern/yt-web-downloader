package ytdlp

import "testing"

const videoJSON = `{"id":"abc","title":"My Video","channel":"My Channel","uploader":"fallback","duration":123.4}`
const playlistJSON = `{"_type":"playlist","id":"PL1","title":"My List","entries":[
  {"id":"v1","title":"One","uploader":"Ch1","duration":10},
  {"id":"v2","title":"Two","channel":"Ch2","duration":20}]}`

func TestParseInfoVideo(t *testing.T) {
	info, err := parseInfo([]byte(videoJSON), false)
	if err != nil {
		t.Fatal(err)
	}
	if info.Type != "video" || info.ID != "abc" || info.Channel != "My Channel" || info.Duration != 123.4 {
		t.Errorf("bad: %+v", info)
	}
}

func TestParseInfoPlaylist(t *testing.T) {
	info, err := parseInfo([]byte(playlistJSON), true)
	if err != nil {
		t.Fatal(err)
	}
	if info.Type != "playlist" || len(info.Entries) != 2 {
		t.Fatalf("bad: %+v", info)
	}
	if info.Entries[0].Index != 1 || info.Entries[0].Channel != "Ch1" || info.Entries[1].Channel != "Ch2" {
		t.Errorf("bad entries: %+v", info.Entries)
	}
}

func TestIsPlaylistURL(t *testing.T) {
	if !IsPlaylistURL("https://www.youtube.com/watch?v=X&list=PL1") {
		t.Error("want true")
	}
	if IsPlaylistURL("https://www.youtube.com/watch?v=X") {
		t.Error("want false")
	}
	if PlaylistID("https://www.youtube.com/watch?v=X&list=PL1") != "PL1" {
		t.Error("bad PlaylistID")
	}
}
