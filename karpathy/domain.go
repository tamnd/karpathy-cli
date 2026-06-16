package karpathy

import (
	"context"
	"net/url"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes karpathy as a kit Domain so a multi-domain host (ant)
// can enable it with a single blank import:
//
//	import _ "github.com/tamnd/karpathy-cli/karpathy"
//
// The init below registers it; the same Domain builds the standalone karpathy
// binary (see cli.NewApp), so the binary and a host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the karpathy driver. It carries no state.
type Domain struct{}

// Info describes the scheme, accepted hostnames, and the binary identity.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "karpathy",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "karpathy",
			Short:  "Browse Andrej Karpathy's blog from the command line.",
			Long: `karpathy reads Andrej Karpathy's blog (karpathy.github.io) and
returns structured records as table, JSON, JSONL, CSV, TSV, or URLs.

Quick start:
  karpathy list -n 5           list the five most recent posts
  karpathy post microgpt        show post details
  karpathy export               export all posts as JSONL
  karpathy info                 blog statistics`,
			Site: Host,
			Repo: "https://github.com/tamnd/karpathy-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	kit.Handle(app, kit.OpMeta{Name: "list", Group: "read", List: true,
		Summary: "List all blog posts"}, listPosts)

	kit.Handle(app, kit.OpMeta{Name: "post", Group: "read", Single: true,
		Summary: "Show details for a single post",
		Args:    []kit.Arg{{Name: "slug", Help: "post slug, partial path, or full URL"}}}, getPost)

	kit.Handle(app, kit.OpMeta{Name: "export", Group: "read", List: true,
		Summary: "Export all posts as JSONL"}, exportPosts)

	kit.Handle(app, kit.OpMeta{Name: "info", Group: "read", Single: true,
		Summary: "Show blog statistics"}, getBlogInfo)
}

// newClient builds a Client from kit.Config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := DefaultConfig()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.Timeout = cfg.Timeout
	}
	return NewClient(c), nil
}

// --- inputs ---

type listIn struct {
	Limit  int     `kit:"flag,inherit" help:"max results (0 = all)"`
	Client *Client `kit:"inject"`
}

type postIn struct {
	Slug   string  `kit:"arg" help:"post slug, partial path, or full URL"`
	Client *Client `kit:"inject"`
}

type exportIn struct {
	Limit  int     `kit:"flag,inherit" help:"max results (0 = all)"`
	Client *Client `kit:"inject"`
}

type infoIn struct {
	Client *Client `kit:"inject"`
}

// --- handlers ---

func listPosts(ctx context.Context, in listIn, emit func(*Post) error) error {
	posts, err := in.Client.Posts(ctx, in.Limit)
	if err != nil {
		return mapErr(err)
	}
	for i := range posts {
		if err := emit(&posts[i]); err != nil {
			return err
		}
	}
	return nil
}

func getPost(ctx context.Context, in postIn, emit func(*PostDetail) error) error {
	detail, err := in.Client.Post(ctx, in.Slug)
	if err != nil {
		return mapErr(err)
	}
	return emit(detail)
}

func exportPosts(ctx context.Context, in exportIn, emit func(*Post) error) error {
	posts, err := in.Client.Posts(ctx, 0)
	if err != nil {
		return mapErr(err)
	}
	for i := range posts {
		if err := emit(&posts[i]); err != nil {
			return err
		}
	}
	return nil
}

func getBlogInfo(ctx context.Context, in infoIn, emit func(*Info) error) error {
	info, err := in.Client.Stats(ctx)
	if err != nil {
		return mapErr(err)
	}
	return emit(info)
}

// --- Resolver ---

// Classify turns a slug or URL into (type, id).
func (Domain) Classify(input string) (uriType, id string, err error) {
	input = strings.TrimSpace(input)
	if u, parseErr := url.Parse(input); parseErr == nil &&
		(u.Scheme == "http" || u.Scheme == "https") {
		slug := slugFromURL(input)
		if slug == "" {
			return "", "", errs.Usage("unrecognized karpathy URL: %q", input)
		}
		return "post", slug, nil
	}
	if input != "" {
		return "post", input, nil
	}
	return "", "", errs.Usage("unrecognized karpathy reference: %q", input)
}

// Locate returns the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	if uriType != "post" {
		return "", errs.Usage("karpathy has no resource type %q", uriType)
	}
	return BaseURL + "/" + strings.Trim(id, "/"), nil
}

// mapErr converts library errors into kit error kinds.
func mapErr(err error) error {
	return err
}
