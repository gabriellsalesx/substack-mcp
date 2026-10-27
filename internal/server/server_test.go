package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gabriellsalesx/substack-mcp/internal/auth"
	"github.com/gabriellsalesx/substack-mcp/internal/substack"
)

// fakeSubstack records requests and answers like the parts of Substack we use.
type fakeSubstack struct {
	mu        sync.Mutex
	calls     []string
	bodies    map[string]map[string]any
	publishes []map[string]any
	live      bool
}

func (f *fakeSubstack) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	if r.Body != nil && r.Header.Get("Content-Type") == "application/json" {
		var m map[string]any
		_ = json.NewDecoder(r.Body).Decode(&m)
		f.bodies[r.Method+" "+r.URL.Path] = m
		if strings.HasSuffix(r.URL.Path, "/publish") {
			f.publishes = append(f.publishes, m)
		}
	}
	live := f.live
	f.mu.Unlock()
	if ck, err := r.Cookie("substack.sid"); err != nil || ck.Value != "s%3Atest" {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"errors":[{"msg":"Please sign in"}]}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch r.Method + " " + r.URL.Path {
	case "GET /root/user/profile/self":
		_, _ = w.Write([]byte(`{"id":7,"name":"Ana","handle":"ana","publicationUsers":[
			{"role":"admin","is_primary":true,"publication":{"id":1,"name":"Main","subdomain":"main"}},
			{"role":"admin","publication":{"id":2,"name":"Side","subdomain":"side","custom_domain":"side.dev"}}]}`))
	case "POST /pub/drafts":
		_, _ = w.Write([]byte(`{"id":101,"draft_title":"Hello","post_date":null}`))
	case "GET /pub/drafts/101":
		if live {
			_, _ = w.Write([]byte(`{"id":101,"title":"Hello","slug":"hello","is_published":true,"post_date":"2026-10-26T10:00:00Z","draft_body":""}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":101,"draft_title":"Hello","audience":"everyone","post_date":null,
			"draft_body":"{\"type\":\"doc\",\"content\":[{\"type\":\"paragraph\",\"content\":[{\"type\":\"text\",\"text\":\"Hi \"},{\"type\":\"text\",\"text\":\"there\",\"marks\":[{\"type\":\"strong\"}]}]}]}"}`))
	case "GET /pub/post_management/drafts":
		_, _ = w.Write([]byte(`[{"id":101,"draft_title":"Hello","post_date":null},{"id":99,"title":"Old","post_date":"2025-01-01T10:00:00.000Z","is_published":true}]`))
	case "POST /pub/drafts/101/publish":
		_, _ = w.Write([]byte(`{"id":101,"title":"Hello","slug":"hello","post_date":"2026-10-26T10:00:00Z","is_published":true}`))
	default:
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"error":"no route"}`))
	}
}

func connect(t *testing.T) (*mcp.ClientSession, *fakeSubstack) {
	t.Helper()
	fake := &fakeSubstack{bodies: map[string]map[string]any{}}
	ts := httptest.NewServer(fake)
	t.Cleanup(ts.Close)

	srv := New(Config{Version: "test", NewClient: func() (*substack.Client, error) {
		c := substack.New(substack.Options{Cookies: []auth.Cookie{{Name: "substack.sid", Value: "s%3Atest"}}})
		c.Root, c.PubBase = ts.URL+"/root", ts.URL+"/pub"
		return c, nil
	}})
	ct, st := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := srv.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs, fake
}

func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (map[string]any, bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	var out map[string]any
	if !res.IsError {
		if err := json.Unmarshal([]byte(text), &out); err != nil {
			t.Fatalf("%s: output is not JSON: %s", name, text)
		}
	} else {
		out = map[string]any{"error": text}
	}
	return out, res.IsError
}

// TestSchemasPortable guards against JSON Schema features that Gemini, Codex
// and other clients reject: type arrays (nullable), $ref/$defs and anyOf.
func TestSchemasPortable(t *testing.T) {
	cs, _ := connect(t)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tools) != 13 {
		t.Errorf("got %d tools, want 13", len(res.Tools))
	}
	for _, tool := range res.Tools {
		raw, _ := json.Marshal(tool.InputSchema)
		s := string(raw)
		for _, bad := range []string{`"type":[`, `"$ref"`, `"$defs"`, `"anyOf"`, `"oneOf"`} {
			if strings.Contains(s, bad) {
				t.Errorf("%s input schema contains %s: %s", tool.Name, bad, s)
			}
		}
		if tool.Annotations == nil || tool.Annotations.DestructiveHint == nil {
			t.Errorf("%s has no explicit destructiveHint", tool.Name)
		}
		if tool.OutputSchema != nil {
			t.Errorf("%s has an output schema; keep outputs schema-less for portability", tool.Name)
		}
	}
}

func TestCreateAndReadDraft(t *testing.T) {
	cs, fake := connect(t)
	out, isErr := call(t, cs, "create_draft", map[string]any{
		"title":   "Hello",
		"content": "# Hello\n\nSome **bold** text.\n\n<!-- paywall -->\n\nPaid part.",
	})
	if isErr {
		t.Fatal(out)
	}
	if out["id"].(float64) != 101 || out["paywall"] != true {
		t.Errorf("unexpected output: %v", out)
	}
	body := fake.bodies["POST /pub/drafts"]
	if body["audience"] != "only_paid" {
		t.Errorf("paywall should set only_paid, got %v", body["audience"])
	}
	doc := body["draft_body"].(string)
	if strings.Contains(doc, `"heading"`) {
		t.Errorf("heading equal to the title should be dropped: %s", doc)
	}
	if !strings.Contains(doc, `"strong"`) || !strings.Contains(doc, `"paywall"`) {
		t.Errorf("formatting lost: %s", doc)
	}

	out, isErr = call(t, cs, "get_post", map[string]any{"post_id": "https://main.substack.com/publish/post/101"})
	if isErr {
		t.Fatal(out)
	}
	if md := out["markdown"].(string); !strings.Contains(md, "Hi **there**") {
		t.Errorf("markdown = %q", md)
	}
}

func TestListDraftsFiltersPublished(t *testing.T) {
	cs, _ := connect(t)
	out, isErr := call(t, cs, "list_posts", map[string]any{})
	if isErr {
		t.Fatal(out)
	}
	if posts := out["posts"].([]any); len(posts) != 1 {
		t.Errorf("want only the draft, got %v", posts)
	}
}

func TestPublishNeedsConfirm(t *testing.T) {
	cs, fake := connect(t)
	out, isErr := call(t, cs, "publish_post", map[string]any{"post_id": "101"})
	if isErr || out["confirmation_required"] != true {
		t.Fatalf("want a preview, got %v", out)
	}
	for _, c := range fake.calls {
		if strings.HasSuffix(c, "/publish") {
			t.Fatal("published without confirm")
		}
	}
	out, isErr = call(t, cs, "publish_post", map[string]any{"post_id": "101", "confirm": true})
	if isErr || out["status"] != "published" || !strings.HasSuffix(out["url"].(string), "/p/hello") {
		t.Fatalf("publish failed: %v", out)
	}
	if fake.bodies["POST /pub/drafts/101/publish"]["send"] != false {
		t.Error("send_email must default to false")
	}
}

func TestPublicationSelection(t *testing.T) {
	cs, _ := connect(t)
	out, isErr := call(t, cs, "list_posts", map[string]any{"publication": "https://nope.substack.com"})
	if !isErr || !strings.Contains(out["error"].(string), "main.substack.com") {
		t.Errorf("want an error listing available publications, got %v", out)
	}
}

func TestNumericIDAccepted(t *testing.T) {
	cs, _ := connect(t)
	out, isErr := call(t, cs, "get_post", map[string]any{"post_id": 101})
	if isErr || out["id"].(float64) != 101 {
		t.Errorf("numeric post_id should work, got %v", out)
	}
}

func TestParseID(t *testing.T) {
	ok := map[string]int64{
		"123":   123,
		" 123 ": 123,
		"c-555": 555,
		"https://x.substack.com/publish/post/123456?section=2": 123456,
		"https://substack.com/@ana/note/c-555?utm_source=2":    555,
	}
	for in, want := range ok {
		if got, err := parseID(in, "id"); err != nil || got != want {
			t.Errorf("parseID(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", "abc", "https://x.substack.com/p/top-10-tips", "https://x.substack.com/?r=2"} {
		if got, err := parseID(in, "id"); err == nil {
			t.Errorf("parseID(%q) = %d, want error", in, got)
		}
	}
}

func TestResendPublishedPost(t *testing.T) {
	cs, fake := connect(t)
	fake.mu.Lock()
	fake.live = true
	fake.mu.Unlock()
	out, isErr := call(t, cs, "publish_post", map[string]any{"post_id": "101", "send_email": true})
	if isErr || !strings.Contains(out["action"].(string), "email it to subscribers") {
		t.Fatalf("preview = %v", out)
	}
	if _, isErr = call(t, cs, "publish_post", map[string]any{"post_id": "101", "send_email": true, "confirm": true}); isErr {
		t.Fatal("publish failed")
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	sends := fake.publishes
	if len(sends) != 2 || sends[0]["send"] != false || sends[1]["only_send"] != true {
		t.Errorf("want an update without email, then only_send; got %v", sends)
	}
}
