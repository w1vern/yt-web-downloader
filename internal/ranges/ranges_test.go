package ranges

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		max  int
		want []int
		err  bool
	}{
		{"1, 2-5, 7-10", 20, []int{1, 2, 3, 4, 5, 7, 8, 9, 10}, false},
		{"1-3,2-4", 10, []int{1, 2, 3, 4}, false}, // overlap dedup
		{"5", 10, []int{5}, false},
		{"", 10, nil, false},        // empty = all
		{"  ", 10, nil, false},
		{"3-1", 10, nil, true},      // reversed
		{"0-2", 10, nil, true},      // below 1
		{"5", 3, nil, true},         // above max
		{"a-b", 10, nil, true},
		{"1,,2", 10, nil, true},
	}
	for _, c := range cases {
		got, err := Parse(c.in, c.max)
		if (err != nil) != c.err {
			t.Errorf("Parse(%q): err=%v, want err=%v", c.in, err, c.err)
			continue
		}
		if !c.err && !reflect.DeepEqual(got, c.want) {
			t.Errorf("Parse(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestFormat(t *testing.T) {
	if got := Format([]int{1, 2, 3, 5}); got != "1-3, 5" {
		t.Errorf("Format = %q", got)
	}
	if got := Format(nil); got != "" {
		t.Errorf("Format(nil) = %q", got)
	}
}

func TestToYtdlp(t *testing.T) {
	if got := ToYtdlp([]int{1, 2, 3, 5}); got != "1:3,5" {
		t.Errorf("ToYtdlp = %q", got)
	}
}
