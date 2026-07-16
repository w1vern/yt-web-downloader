package jobs

import (
	"testing"
	"time"
)

func TestShouldDelete(t *testing.T) {
	now := time.Now()
	ttl := time.Hour
	cases := []struct {
		known bool
		st    State
		fin   time.Time
		want  bool
	}{
		{false, "", time.Time{}, true},                     // orphan dir
		{true, Running, time.Time{}, false},                // running — never
		{true, Queued, time.Time{}, false},                 // queued — never
		{true, Done, now.Add(-2 * time.Hour), true},        // expired
		{true, Done, now.Add(-10 * time.Minute), false},    // fresh
		{true, Failed, now.Add(-2 * time.Hour), true},      // failed expired
	}
	for i, c := range cases {
		if got := shouldDelete(c.known, c.st, c.fin, ttl, now); got != c.want {
			t.Errorf("case %d: got %v want %v", i, got, c.want)
		}
	}
}
