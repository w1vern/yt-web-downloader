package ytdlp

import "testing"

func TestParseLineProgress(t *testing.T) {
	ev := ParseLine("PRG|dQw4w9WgXcQ| 42.3%|1.2MiB/s|00:12")
	if ev.Type != "progress" || ev.VideoID != "dQw4w9WgXcQ" || ev.Percent != "42.3%" || ev.Speed != "1.2MiB/s" || ev.ETA != "00:12" {
		t.Errorf("bad parse: %+v", ev)
	}
}

func TestParseLineLog(t *testing.T) {
	ev := ParseLine("[download] Destination: foo.webm")
	if ev.Type != "log" || ev.Line != "[download] Destination: foo.webm" {
		t.Errorf("bad parse: %+v", ev)
	}
}

func TestParseLineMalformedPrg(t *testing.T) {
	if ev := ParseLine("PRG|only|two"); ev.Type != "log" {
		t.Errorf("malformed PRG must fall back to log, got %+v", ev)
	}
}
