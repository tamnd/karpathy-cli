package karpathy

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// These tests are offline: they exercise the URI driver's pure string functions
// and the host wiring, which need no network.
// HTTP behaviour is covered in karpathy_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "karpathy" {
		t.Errorf("Scheme = %q, want karpathy", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "karpathy" {
		t.Errorf("Identity.Binary = %q, want karpathy", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct{ in, typ, id string }{
		{"microgpt", "post", "microgpt"},
		{"lecun1989", "post", "lecun1989"},
		{"https://" + Host + "/2026/02/12/microgpt/", "post", "microgpt"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil || typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q, %v), want (%q, %q, nil)",
				tc.in, typ, id, err, tc.typ, tc.id)
		}
	}
}

func TestLocate(t *testing.T) {
	got, err := Domain{}.Locate("post", "microgpt")
	want := "https://" + Host + "/microgpt"
	if err != nil || got != want {
		t.Errorf("Locate = (%q, %v), want (%q, nil)", got, err, want)
	}
}

func TestLocateUnknownType(t *testing.T) {
	_, err := Domain{}.Locate("page", "microgpt")
	if err == nil {
		t.Error("expected error for unknown type, got nil")
	}
}

// TestHostWiring mounts the driver in a kit Host and checks the round-trip.
func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}

	got, err := h.ResolveOn("karpathy", "microgpt")
	if err != nil {
		t.Fatalf("ResolveOn: %v", err)
	}
	if got.String() != "karpathy://post/microgpt" {
		t.Errorf("ResolveOn = %q, want karpathy://post/microgpt", got.String())
	}
}
