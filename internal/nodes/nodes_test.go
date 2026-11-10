package nodes

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ahmettsezis/flui/internal/store"
)

func TestNormalizeMonitorURL(t *testing.T) {
	ok := map[string]string{
		"http://10.0.0.21:24220":     "http://10.0.0.21:24220",
		"10.0.0.21":                  "http://10.0.0.21:24220",
		" fluentd-01.lan:24230/ ":    "http://fluentd-01.lan:24230",
		"https://[fd00::1]:24220":    "https://[fd00::1]:24220",
		"HTTP://Fluentd.Example.com": "http://Fluentd.Example.com:24220",
	}
	for in, want := range ok {
		got, err := NormalizeMonitorURL(in)
		if err != nil || got != want {
			t.Errorf("%q: got %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "ftp://x", "http://user:pw@x:1", "http://x:1/api", "http://x:1?a=b", "http://x:99999", "file:///etc/passwd", "http://:24220"} {
		if _, err := NormalizeMonitorURL(in); err == nil {
			t.Errorf("%q accepted", in)
		}
	}
}

func TestSnippet(t *testing.T) {
	s := Snippet(Node{Name: "web-1", MonitorURL: "http://10.0.0.21:24220"})
	for _, want := range []string{"bind 10.0.0.21", "port 24220", "include_config false", "TCP 24220"} {
		if !strings.Contains(s, want) {
			t.Errorf("snippet missing %q:\n%s", want, s)
		}
	}
}

func TestRepoCRUD(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	r := NewRepo(st.DB)

	n, err := r.Create(ctx, Input{Name: "web-1", MonitorURL: "10.0.0.21"})
	if err != nil || n.ID == 0 || n.MonitorURL != "http://10.0.0.21:24220" {
		t.Fatalf("create: %+v %v", n, err)
	}
	if _, err := r.Create(ctx, Input{Name: "WEB-1", MonitorURL: "10.0.0.22"}); !errors.Is(err, ErrDuplicate) {
		t.Errorf("duplicate: %v", err)
	}
	var verr *ValidationError
	if _, err := r.Create(ctx, Input{Name: "<script>", MonitorURL: "10.0.0.22"}); !errors.As(err, &verr) || verr.Field != "name" {
		t.Errorf("invalid name: %v", err)
	}
	n, err = r.Update(ctx, n.ID, Input{Name: "web-01", MonitorURL: "http://10.0.0.21:24230"})
	if err != nil || n.Name != "web-01" || n.MonitorURL != "http://10.0.0.21:24230" {
		t.Fatalf("update: %+v %v", n, err)
	}
	if _, err := r.Update(ctx, 999, Input{Name: "x", MonitorURL: "x"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("update missing: %v", err)
	}
	list, err := r.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %v", list, err)
	}
	if err := r.Delete(ctx, n.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Get(ctx, n.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("get deleted: %v", err)
	}
}
