// Package karpathy is the library behind the karpathy command line:
// the HTTP client, RSS feed parsing, and typed data models for
// Andrej Karpathy's blog at karpathy.github.io.
//
// The Client is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public site throws under load.
package karpathy

import (
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// DefaultUserAgent identifies the client to the blog server.
const DefaultUserAgent = "karpathy/dev (+https://github.com/tamnd/karpathy-cli)"

// Host is the site this client talks to.
const Host = "karpathy.github.io"

// BaseURL is the root every request is built from.
const BaseURL = "https://" + Host

// Config holds tuneable parameters for a Client.
type Config struct {
	BaseURL   string
	FeedPath  string
	UserAgent string
	Rate      time.Duration
	Timeout   time.Duration
	Retries   int
}

// DefaultConfig returns a Config with polite defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL:   BaseURL,
		FeedPath:  "/feed.xml",
		UserAgent: DefaultUserAgent,
		Rate:      300 * time.Millisecond,
		Timeout:   30 * time.Second,
		Retries:   3,
	}
}

// Client talks to karpathy.github.io over HTTP.
type Client struct {
	cfg  Config
	http *http.Client
	mu   sync.Mutex
	last time.Time
}

// NewClient returns a Client configured from cfg.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

// Post is a single blog post entry.
type Post struct {
	Rank    int    `json:"rank"`
	Title   string `json:"title"`
	Date    string `json:"date"`
	Slug    string `json:"slug"`
	Summary string `json:"summary"`
	URL     string `json:"url"`
}

// PostDetail augments Post with sections and links from the post page.
type PostDetail struct {
	Title    string `json:"title"`
	Date     string `json:"date"`
	Slug     string `json:"slug"`
	Summary  string `json:"summary"`
	Sections string `json:"sections"`
	Links    string `json:"links"`
	URL      string `json:"url"`
}

// Info holds aggregate blog statistics.
type Info struct {
	TotalPosts int    `json:"total_posts"`
	FirstPost  string `json:"first_post"`
	LatestPost string `json:"latest_post"`
	FeedURL    string `json:"feed_url"`
	SiteURL    string `json:"site_url"`
}

// rss2Feed is the RSS 2.0 wire format.
type rss2Feed struct {
	Channel struct {
		Items []rss2Item `xml:"item"`
	} `xml:"channel"`
}

type rss2Item struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	PubDate     string `xml:"pubDate"`
	Description string `xml:"description"`
}

// Posts fetches the RSS feed and returns up to limit posts, newest first.
// limit <= 0 returns all posts in the feed.
func (c *Client) Posts(ctx context.Context, limit int) ([]Post, error) {
	feedURL := c.cfg.BaseURL + c.cfg.FeedPath
	body, err := c.get(ctx, feedURL)
	if err != nil {
		return nil, fmt.Errorf("fetch feed: %w", err)
	}
	return parsePosts(body, limit)
}

// Post fetches a single blog post by slug or URL.
func (c *Client) Post(ctx context.Context, slug string) (*PostDetail, error) {
	// Resolve slug to URL via the feed
	postURL, err := c.resolveSlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	body, err := c.get(ctx, postURL)
	if err != nil {
		return nil, err
	}
	return parsePostPage(body, postURL)
}

// Stats returns aggregate statistics from the RSS feed.
func (c *Client) Stats(ctx context.Context) (*Info, error) {
	posts, err := c.Posts(ctx, 0)
	if err != nil {
		return nil, err
	}
	info := &Info{
		TotalPosts: len(posts),
		FeedURL:    c.cfg.BaseURL + c.cfg.FeedPath,
		SiteURL:    c.cfg.BaseURL,
	}
	if len(posts) > 0 {
		info.LatestPost = posts[0].Date
		info.FirstPost = posts[len(posts)-1].Date
	}
	return info, nil
}

// resolveSlug finds the full URL for a slug/path/URL in the RSS feed.
func (c *Client) resolveSlug(ctx context.Context, input string) (string, error) {
	// If it looks like a full URL, use it directly
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		return input, nil
	}
	posts, err := c.Posts(ctx, 0)
	if err != nil {
		return "", err
	}
	// Try exact slug match
	for _, p := range posts {
		if p.Slug == input {
			return p.URL, nil
		}
	}
	// Try suffix match (partial path)
	for _, p := range posts {
		if strings.HasSuffix(strings.TrimSuffix(p.URL, "/"), "/"+input) {
			return p.URL, nil
		}
	}
	return "", fmt.Errorf("post not found: %s", input)
}

// parsePosts parses an RSS 2.0 body into Post records.
func parsePosts(body []byte, limit int) ([]Post, error) {
	var feed rss2Feed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("parse feed: %w", err)
	}
	items := feed.Channel.Items
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	posts := make([]Post, 0, len(items))
	for i, it := range items {
		link := strings.TrimSpace(it.Link)
		posts = append(posts, Post{
			Rank:    i + 1,
			Title:   cleanText(it.Title),
			Date:    parseDate(it.PubDate),
			Slug:    slugFromURL(link),
			Summary: cleanSummary(it.Description),
			URL:     link,
		})
	}
	return posts, nil
}

// parsePostPage extracts PostDetail from a post HTML page.
func parsePostPage(body []byte, postURL string) (*PostDetail, error) {
	s := string(body)
	slug := slugFromURL(postURL)
	// Extract title from <h1>
	title := ""
	if m := reH1.FindStringSubmatch(s); m != nil {
		title = strings.TrimSpace(cleanText(m[1]))
	}
	// Extract date
	date := ""
	if m := reDateMeta.FindStringSubmatch(s); m != nil {
		date = parseDate(m[1])
	}
	// Extract first paragraph as summary
	summary := ""
	if m := reFirstP.FindStringSubmatch(s); m != nil {
		summary = cleanSummary(m[1])
		if len(summary) > 500 {
			summary = summary[:499] + "..."
		}
	}
	return &PostDetail{
		Title:    title,
		Date:     date,
		Slug:     slug,
		Summary:  summary,
		Sections: extractSections(s),
		Links:    extractLinks(s),
		URL:      postURL,
	}, nil
}

var (
	reTag     = regexp.MustCompile(`<[^>]+>`)
	reH1      = regexp.MustCompile(`(?i)<h1[^>]*>(.*?)</h1>`)
	reH2      = regexp.MustCompile(`(?i)<h2[^>]*>(.*?)</h2>`)
	reFirstP  = regexp.MustCompile(`(?s)<p[^>]*>(.*?)</p>`)
	reDateMeta = regexp.MustCompile(`(?i)<meta[^>]+property="article:published_time"[^>]+content="([^"]+)"`)
	reLinkHref = regexp.MustCompile(`href="(https?://[^"]+)"`)
)

// extractSections extracts h2 headings from HTML.
func extractSections(s string) string {
	var secs []string
	for _, m := range reH2.FindAllStringSubmatch(s, -1) {
		t := strings.TrimSpace(cleanText(m[1]))
		if t != "" {
			secs = append(secs, t)
		}
	}
	return strings.Join(secs, ";")
}

// extractLinks extracts external links from HTML.
func extractLinks(s string) string {
	seen := map[string]bool{}
	var links []string
	for _, m := range reLinkHref.FindAllStringSubmatch(s, -1) {
		href := m[1]
		if !seen[href] {
			seen[href] = true
			links = append(links, href)
		}
	}
	return strings.Join(links, ";")
}

// slugFromURL extracts the last path segment from a URL.
func slugFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

// cleanSummary strips HTML and truncates.
func cleanSummary(s string) string {
	s = reTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 200 {
		s = s[:199] + "..."
	}
	return s
}

// cleanText decodes HTML entities and trims space.
func cleanText(s string) string {
	s = reTag.ReplaceAllString(s, "")
	return strings.TrimSpace(html.UnescapeString(s))
}

// parseDate parses an RSS date string to YYYY-MM-DD.
func parseDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	for _, layout := range []string{
		time.RFC1123Z,
		time.RFC1123,
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"Mon, 02 Jan 2006 15:04:05 -0700",
		"Mon, 2 Jan 2006 15:04:05 +0000",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

// get fetches a URL with pacing and retries.
func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

func (c *Client) pace() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cfg.Rate > 0 {
		if wait := c.cfg.Rate - time.Since(c.last); wait > 0 {
			time.Sleep(wait)
		}
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}
