package ytapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// NewHTTPClient returns a client going through proxyURL (socks5/socks5h/http) or direct if empty.
func NewHTTPClient(proxyURL string) *http.Client {
	c := &http.Client{Timeout: 30 * time.Second}
	if proxyURL == "" {
		return c
	}
	fixed := strings.Replace(proxyURL, "socks5h://", "socks5://", 1)
	if u, err := url.Parse(fixed); err == nil {
		c.Transport = &http.Transport{Proxy: http.ProxyURL(u)}
	}
	return c
}

const apiURL = "https://www.googleapis.com/youtube/v3/playlistItems"

type apiResp struct {
	NextPageToken string `json:"nextPageToken"`
	Items         []struct {
		Snippet struct {
			PublishedAt string `json:"publishedAt"`
			ResourceID  struct {
				VideoID string `json:"videoId"`
			} `json:"resourceId"`
		} `json:"snippet"`
	} `json:"items"`
}

// FetchPlaylistDates returns {videoID: date-added} for every playlist item.
func FetchPlaylistDates(ctx context.Context, apiKey, playlistID, proxyURL string) (map[string]string, error) {
	client := NewHTTPClient(proxyURL)
	dates := map[string]string{}
	pageToken := ""
	for {
		q := url.Values{
			"part":       {"snippet"},
			"playlistId": {playlistID},
			"maxResults": {"50"},
			"key":        {apiKey},
		}
		if pageToken != "" {
			q.Set("pageToken", pageToken)
		}
		req, err := http.NewRequestWithContext(ctx, "GET", apiURL+"?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("youtube api: %w", err)
		}
		var body apiResp
		err = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("youtube api status %d", resp.StatusCode)
		}
		if err != nil {
			return nil, fmt.Errorf("youtube api decode: %w", err)
		}
		for _, it := range body.Items {
			dates[it.Snippet.ResourceID.VideoID] = it.Snippet.PublishedAt
		}
		if body.NextPageToken == "" {
			return dates, nil
		}
		pageToken = body.NextPageToken
	}
}
