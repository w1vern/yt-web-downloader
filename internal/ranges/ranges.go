package ranges

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var tokenRe = regexp.MustCompile(`^(\d+)(?:\s*-\s*(\d+))?$`)

// Parse parses "1, 2-5, 7-10" into sorted unique 1-based indices.
// Empty input returns (nil, nil) meaning "all items". max > 0 caps indices.
func Parse(s string, max int) ([]int, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	seen := map[int]bool{}
	for _, tok := range strings.Split(s, ",") {
		tok = strings.TrimSpace(tok)
		m := tokenRe.FindStringSubmatch(tok)
		if m == nil {
			return nil, fmt.Errorf("bad token %q", tok)
		}
		a, _ := strconv.Atoi(m[1])
		b := a
		if m[2] != "" {
			b, _ = strconv.Atoi(m[2])
		}
		if a < 1 || b < a {
			return nil, fmt.Errorf("bad range %q", tok)
		}
		if max > 0 && b > max {
			return nil, fmt.Errorf("index %d out of range (max %d)", b, max)
		}
		for i := a; i <= b; i++ {
			seen[i] = true
		}
	}
	out := make([]int, 0, len(seen))
	for i := range seen {
		out = append(out, i)
	}
	sort.Ints(out)
	return out, nil
}

// runs splits sorted unique indices into consecutive [start, end] pairs.
func runs(items []int) [][2]int {
	var out [][2]int
	for _, v := range items {
		if n := len(out); n > 0 && out[n-1][1] == v-1 {
			out[n-1][1] = v
		} else {
			out = append(out, [2]int{v, v})
		}
	}
	return out
}

func join(items []int, sep, dash string) string {
	var parts []string
	for _, r := range runs(items) {
		if r[0] == r[1] {
			parts = append(parts, strconv.Itoa(r[0]))
		} else {
			parts = append(parts, strconv.Itoa(r[0])+dash+strconv.Itoa(r[1]))
		}
	}
	return strings.Join(parts, sep)
}

// Format renders indices for humans: "1-3, 5".
func Format(items []int) string { return join(items, ", ", "-") }

// ToYtdlp renders indices as a yt-dlp -I spec: "1:3,5".
func ToYtdlp(items []int) string { return join(items, ",", ":") }
