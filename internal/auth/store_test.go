package auth

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestParseCookies(t *testing.T) {
	tests := []struct {
		in   string
		want []Cookie
	}{
		{"s%3Aabc", []Cookie{{"substack.sid", "s%3Aabc"}, {"connect.sid", "s%3Aabc"}}},
		{"substack.sid=s%3Aabc", []Cookie{{"substack.sid", "s%3Aabc"}}},
		{"Cookie: ab=1; connect.sid=s%3Axyz; other=2", []Cookie{{"connect.sid", "s%3Axyz"}}},
		{"  \"s%3Aq\"\n", []Cookie{{"substack.sid", "s%3Aq"}, {"connect.sid", "s%3Aq"}}},
		{"foo=bar", nil},
		{"substack.sid=s%3Aabc; foo=bar baz", []Cookie{{"substack.sid", "s%3Aabc"}}},
		{"", nil},
	}
	for _, tt := range tests {
		got := ParseCookies(tt.in)
		if len(got) != len(tt.want) {
			t.Errorf("ParseCookies(%q) = %v, want %v", tt.in, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("ParseCookies(%q) = %v, want %v", tt.in, got, tt.want)
			}
		}
	}
}

func TestSaveLoad(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SUBSTACK_MCP_CONFIG_DIR", dir)
	t.Setenv("SUBSTACK_SID", "")
	t.Setenv("SUBSTACK_SESSION_TOKEN", "")
	t.Setenv("SUBSTACK_COOKIE", "")
	if _, err := Load(); err != ErrNoCredentials {
		t.Fatalf("Load on empty dir = %v", err)
	}
	if err := Save(&Credentials{Cookies: []Cookie{{"substack.sid", "v"}}, Publication: "https://a.substack.com"}); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		fi, _ := os.Stat(filepath.Join(dir, "credentials.json"))
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("mode = %v, want 0600", fi.Mode().Perm())
		}
	}
	c, err := Load()
	if err != nil || c.Cookies[0].Value != "v" || c.Publication != "https://a.substack.com" {
		t.Fatalf("Load = %+v, %v", c, err)
	}
	t.Setenv("SUBSTACK_SID", "env")
	if c, _ := Load(); c.Cookies[0].Value != "env" {
		t.Errorf("env should win, got %+v", c)
	}
	if err := Delete(); err != nil {
		t.Fatal(err)
	}
}
