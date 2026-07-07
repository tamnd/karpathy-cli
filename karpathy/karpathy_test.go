package karpathy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const sampleFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Andrej Karpathy blog</title>
    <link>http://karpathy.github.io/</link>
    <item>
      <title>microgpt</title>
      <link>http://karpathy.github.io/2026/02/12/microgpt/</link>
      <pubDate>Thu, 12 Feb 2026 00:00:00 +0000</pubDate>
      <description>&lt;p&gt;This is a brief guide to my new art project.&lt;/p&gt;</description>
    </item>
    <item>
      <title>Deep Neural Nets: 33 years ago</title>
      <link>http://karpathy.github.io/2022/03/14/lecun1989/</link>
      <pubDate>Mon, 14 Mar 2022 00:00:00 +0000</pubDate>
      <description>&lt;p&gt;LeCun 1989 is the earliest real-world application.&lt;/p&gt;</description>
    </item>
    <item>
      <title>Bitcoin in Python</title>
      <link>http://karpathy.github.io/2021/06/21/blockchain/</link>
      <pubDate>Mon, 21 Jun 2021 00:00:00 +0000</pubDate>
      <description>&lt;p&gt;We create a Bitcoin transaction in pure Python.&lt;/p&gt;</description>
    </item>
  </channel>
</rss>`

const samplePostHTML = `<!DOCTYPE html>
<html>
<head><title>microgpt - Andrej Karpathy blog</title></head>
<body>
<div class="post">
  <h1 class="post-title">microgpt</h1>
  <p class="post-meta">Feb 12, 2026</p>
  <div class="entry">
    <p>This is a brief guide to my new art project microgpt.</p>
    <h2 id="dataset">Dataset</h2>
    <p>The fuel of large language models is data.</p>
    <h2 id="tokenizer">Tokenizer</h2>
    <p>Neural networks work with numbers, not characters.</p>
  </div>
</div>
</body>
</html>`

func newTestServer(t *testing.T) (*httptest.Server, *Client) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/feed.xml"):
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(sampleFeed))
		case strings.Contains(r.URL.Path, "microgpt"):
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(samplePostHTML))
		default:
			http.NotFound(w, r)
		}
	}))
	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	return srv, NewClient(cfg)
}

func TestPostsAll(t *testing.T) {
	srv, c := newTestServer(t)
	defer srv.Close()

	posts, err := c.Posts(context.Background(), 0)
	if err != nil {
		t.Fatalf("Posts: %v", err)
	}
	if len(posts) != 3 {
		t.Errorf("want 3 posts, got %d", len(posts))
	}
}

func TestPostsLimit(t *testing.T) {
	srv, c := newTestServer(t)
	defer srv.Close()

	posts, err := c.Posts(context.Background(), 2)
	if err != nil {
		t.Fatalf("Posts: %v", err)
	}
	if len(posts) != 2 {
		t.Errorf("want 2 posts, got %d", len(posts))
	}
}

func TestPostsOrder(t *testing.T) {
	srv, c := newTestServer(t)
	defer srv.Close()

	posts, err := c.Posts(context.Background(), 0)
	if err != nil {
		t.Fatalf("Posts: %v", err)
	}
	if posts[0].Rank != 1 {
		t.Errorf("first post rank = %d, want 1", posts[0].Rank)
	}
	if posts[0].Slug != "microgpt" {
		t.Errorf("first post slug = %q, want microgpt", posts[0].Slug)
	}
}

func TestPostDate(t *testing.T) {
	srv, c := newTestServer(t)
	defer srv.Close()

	posts, err := c.Posts(context.Background(), 1)
	if err != nil {
		t.Fatalf("Posts: %v", err)
	}
	if posts[0].Date != "2026-02-12" {
		t.Errorf("date = %q, want 2026-02-12", posts[0].Date)
	}
}

func TestStats(t *testing.T) {
	srv, c := newTestServer(t)
	defer srv.Close()

	info, err := c.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if info.TotalPosts != 3 {
		t.Errorf("total_posts = %d, want 3", info.TotalPosts)
	}
	if info.LatestPost != "2026-02-12" {
		t.Errorf("latest_post = %q, want 2026-02-12", info.LatestPost)
	}
	if info.FirstPost != "2021-06-21" {
		t.Errorf("first_post = %q, want 2021-06-21", info.FirstPost)
	}
}

func TestPostBySlug(t *testing.T) {
	srv, c := newTestServer(t)
	defer srv.Close()

	detail, err := c.Post(context.Background(), "microgpt")
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if detail.Slug != "microgpt" {
		t.Errorf("slug = %q, want microgpt", detail.Slug)
	}
}

func TestPostSections(t *testing.T) {
	srv, c := newTestServer(t)
	defer srv.Close()

	detail, err := c.Post(context.Background(), "microgpt")
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if !strings.Contains(detail.Sections, "Dataset") {
		t.Errorf("sections = %q, want to contain 'Dataset'", detail.Sections)
	}
	if !strings.Contains(detail.Sections, "Tokenizer") {
		t.Errorf("sections = %q, want to contain 'Tokenizer'", detail.Sections)
	}
}

func TestPostNotFound(t *testing.T) {
	srv, c := newTestServer(t)
	defer srv.Close()

	_, err := c.Post(context.Background(), "nonexistent-post-slug")
	if err == nil {
		t.Error("expected error for unknown slug, got nil")
	}
}

func TestSlugFromURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"http://karpathy.github.io/2026/02/12/microgpt/", "microgpt"},
		{"https://karpathy.github.io/2022/03/14/lecun1989/", "lecun1989"},
		{"http://karpathy.github.io/", ""},
	}
	for _, tc := range cases {
		got := slugFromURL(tc.in)
		if got != tc.want {
			t.Errorf("slugFromURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseDate(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Thu, 12 Feb 2026 00:00:00 +0000", "2026-02-12"},
		{"Mon, 14 Mar 2022 00:00:00 +0000", "2022-03-14"},
		{"Mon, 21 Jun 2021 00:00:00 +0000", "2021-06-21"},
		{"", ""},
	}
	for _, tc := range cases {
		got := parseDate(tc.in)
		if got != tc.want {
			t.Errorf("parseDate(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestStripHTML(t *testing.T) {
	in := "<p>Hello &amp; <b>world</b>!</p>"
	got := cleanSummary(in)
	if !strings.Contains(got, "Hello & world !") && !strings.Contains(got, "Hello & world!") {
		t.Errorf("cleanSummary(%q) = %q, want HTML stripped", in, got)
	}
}

func TestExtractSections(t *testing.T) {
	html := "<h2>Section One</h2><p>Text</p><h2>Section Two</h2>"
	got := extractSections(html)
	if got != "Section One;Section Two" {
		t.Errorf("extractSections = %q, want %q", got, "Section One;Section Two")
	}
}
