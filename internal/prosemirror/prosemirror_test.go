package prosemirror

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func mustJSON(t *testing.T, n *Node) string {
	t.Helper()
	s, err := n.String()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// normalize round-trips through JSON so int/float attrs compare equal.
func normalize(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func doc(content ...*Node) *Node { return &Node{Type: "doc", Content: content} }
func p(content ...*Node) *Node   { return &Node{Type: "paragraph", Content: content} }
func txt(s string, marks ...Mark) *Node {
	return &Node{Type: "text", Text: s, Marks: marks}
}

var (
	strong = Mark{Type: "strong"}
	em     = Mark{Type: "em"}
	code   = Mark{Type: "code"}
	strike = Mark{Type: "strikethrough"}
)

func link(href string) Mark { return Mark{Type: "link", Attrs: map[string]any{"href": href}} }

func TestFromMarkdown(t *testing.T) {
	tests := []struct {
		name string
		md   string
		want *Node
	}{
		{"paragraph", "hello world", doc(p(txt("hello world")))},
		{"soft break", "a\nb", doc(p(txt("a b")))},
		{"hard break spaces", "a  \nb", doc(p(txt("a"), &Node{Type: "hard_break"}, txt("b")))},
		{"hard break backslash", "a\\\nb", doc(p(txt("a"), &Node{Type: "hard_break"}, txt("b")))},
		{"heading", "## Sub *title*", doc(&Node{Type: "heading", Attrs: map[string]any{"level": 2}, Content: []*Node{txt("Sub "), txt("title", em)}})},
		{"marks", "*a* **b** ***c*** `d` ~~e~~", doc(p(
			txt("a", em), txt(" "), txt("b", strong), txt(" "), txt("c", em, strong), txt(" "),
			txt("d", code), txt(" "), txt("e", strike)))},
		{"nested marks merge", "**a *b* c**", doc(p(txt("a ", strong), txt("b", em, strong), txt(" c", strong)))},
		{"link", "[go *here*](https://x.dev/a)", doc(p(txt("go ", link("https://x.dev/a")), txt("here", link("https://x.dev/a"), em)))},
		{"autolink", "<https://a.b> and https://c.d", doc(p(txt("https://a.b", link("https://a.b")), txt(" and "), txt("https://c.d", link("https://c.d"))))},
		{"escapes and entities", `\*not em\* &amp; &#65;`, doc(p(txt("*not em* & A")))},
		{"code span with backtick", "`` a`b ``", doc(p(txt("a`b", code)))},
		{"blockquote", "> a\n>\n> > b", doc(&Node{Type: "blockquote", Content: []*Node{
			p(txt("a")), {Type: "blockquote", Content: []*Node{p(txt("b"))}}}})},
		{"bullet list", "- a\n- b", doc(&Node{Type: "bullet_list", Content: []*Node{
			{Type: "list_item", Content: []*Node{p(txt("a"))}},
			{Type: "list_item", Content: []*Node{p(txt("b"))}}}})},
		{"ordered list start", "3. a\n4. b", doc(&Node{Type: "ordered_list", Attrs: map[string]any{"start": 3, "order": 3}, Content: []*Node{
			{Type: "list_item", Content: []*Node{p(txt("a"))}},
			{Type: "list_item", Content: []*Node{p(txt("b"))}}}})},
		{"empty list item", "-\n- b", doc(&Node{Type: "bullet_list", Content: []*Node{
			{Type: "list_item", Content: []*Node{{Type: "paragraph"}}},
			{Type: "list_item", Content: []*Node{p(txt("b"))}}}})},
		{"fenced code", "```go\nx := 1\n\n```", doc(&Node{Type: "code_block", Attrs: map[string]any{"language": "go"}, Content: []*Node{txt("x := 1")}})},
		{"fenced code no lang", "```\nx\n```", doc(&Node{Type: "code_block", Content: []*Node{txt("x")}})},
		{"empty fence", "```\n```", doc(&Node{Type: "code_block"})},
		{"indented code", "    x\n    y", doc(&Node{Type: "code_block", Content: []*Node{txt("x\ny")}})},
		{"thematic break", "a\n\n---\n\nb", doc(p(txt("a")), &Node{Type: "horizontal_rule"}, p(txt("b")))},
		{"html dropped", "<div>\nx\n</div>\n\na <b>b</b>", doc(p(txt("a b")))},
		{"table", "| a | b |\n|:--|--:|\n| 1 | 2 |", doc(&Node{Type: "code_block", Content: []*Node{txt("| a | b |\n| :--- | ---: |\n| 1 | 2 |")}})},
		{"empty", "", doc()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := FromMarkdown(context.Background(), tt.md, Options{})
			if err != nil {
				t.Fatal(err)
			}
			if got, want := normalize(t, res.Doc), normalize(t, tt.want); !reflect.DeepEqual(got, want) {
				t.Errorf("got  %s\nwant %s", mustJSON(t, res.Doc), mustJSON(t, tt.want))
			}
		})
	}
}

func TestPaywall(t *testing.T) {
	tests := []struct {
		name string
		md   string
		want bool
	}{
		{"html block", "a\n\n<!-- paywall -->\n\nb", true},
		{"case and spacing", "a\n\n<!--PayWall   -->", true},
		{"inline only html", "<span></span><!-- paywall -->", true},
		{"other comment", "<!-- note -->", false},
		{"in text", "text <!-- paywall --> more", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := FromMarkdown(context.Background(), tt.md, Options{})
			if err != nil {
				t.Fatal(err)
			}
			if res.HasPaywall != tt.want {
				t.Fatalf("HasPaywall = %v, want %v", res.HasPaywall, tt.want)
			}
			found := false
			for _, n := range res.Doc.Content {
				found = found || n.Type == "paywall"
			}
			if found != tt.want {
				t.Fatalf("paywall node present = %v: %s", found, mustJSON(t, res.Doc))
			}
		})
	}
}

func TestDropTitle(t *testing.T) {
	tests := []struct {
		name, md, title string
		blocks          int
	}{
		{"match", "# My Post\n\nbody", "  my post ", 1},
		{"other level matches", "## My Post\n\nbody", "My Post", 1},
		{"different text", "# Other\n\nbody", "My Post", 2},
		{"not first", "body\n\n# My Post", "My Post", 2},
		{"disabled", "# My Post\n\nbody", "", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := FromMarkdown(context.Background(), tt.md, Options{DropTitle: tt.title})
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Doc.Content) != tt.blocks {
				t.Fatalf("got %d blocks: %s", len(res.Doc.Content), mustJSON(t, res.Doc))
			}
		})
	}
}

func TestImages(t *testing.T) {
	var seen []string
	resolver := func(_ context.Context, src string) (Image, error) {
		seen = append(seen, src)
		return Image{URL: "https://cdn/" + strings.TrimPrefix(src, "./"), Width: 800, Height: 600, Bytes: 1234, ContentType: "image/png"}, nil
	}
	md := "before ![Alt *text*](./a.png \"A caption\") after\n\n- item ![b](b.png)\n\n> [![c](c.png)](https://link)"
	res, err := FromMarkdown(context.Background(), md, Options{Images: resolver})
	if err != nil {
		t.Fatal(err)
	}
	if res.Images != 3 || !reflect.DeepEqual(seen, []string{"./a.png", "b.png", "c.png"}) {
		t.Fatalf("Images=%d seen=%v", res.Images, seen)
	}
	c := res.Doc.Content
	if len(c) != 5 || c[0].Type != "paragraph" || c[1].Type != "captionedImage" || c[2].Type != "paragraph" ||
		c[3].Type != "bullet_list" || c[4].Type != "blockquote" {
		t.Fatalf("unexpected structure: %s", mustJSON(t, res.Doc))
	}
	if got := PlainText(c[0]) + "|" + PlainText(c[2]); got != "before|after" {
		t.Errorf("split text = %q", got)
	}
	img := c[1].Content[0]
	want := map[string]any{
		"src": "https://cdn/a.png", "width": 800, "height": 600, "bytes": 1234, "alt": "Alt text",
		"type": "image/png", "title": nil, "href": nil, "belowTheFold": false, "isProcessing": false,
	}
	attrs := normalize(t, img.Attrs).(map[string]any)
	for k, v := range normalize(t, want).(map[string]any) {
		if !reflect.DeepEqual(attrs[k], v) {
			t.Errorf("attr %s = %v, want %v", k, attrs[k], v)
		}
	}
	if img.Type != "image2" || len(c[1].Content) != 2 || c[1].Content[1].Type != "caption" || PlainText(c[1].Content[1]) != "A caption" {
		t.Errorf("caption missing: %s", mustJSON(t, c[1]))
	}
	item := c[3].Content[0].Content
	if len(item) != 2 || item[0].Type != "paragraph" || item[1].Type != "captionedImage" {
		t.Errorf("list item split wrong: %s", mustJSON(t, c[3]))
	}
	quote := c[4].Content
	if len(quote) != 1 || quote[0].Type != "captionedImage" {
		t.Errorf("linked image in quote wrong: %s", mustJSON(t, c[4]))
	}
}

func TestImagesWithoutResolver(t *testing.T) {
	res, err := FromMarkdown(context.Background(), "![](x.png)", Options{})
	if err != nil {
		t.Fatal(err)
	}
	attrs := res.Doc.Content[0].Content[0].Attrs
	if attrs["src"] != "x.png" || attrs["width"] != nil || attrs["height"] != nil || attrs["bytes"] != nil || attrs["alt"] != nil || attrs["type"] != nil {
		t.Fatalf("attrs = %v", attrs)
	}
}

func TestImageResolverError(t *testing.T) {
	boom := errors.New("boom")
	_, err := FromMarkdown(context.Background(), "text\n\n![a](bad.png)", Options{
		Images: func(context.Context, string) (Image, error) { return Image{}, boom },
	})
	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "bad.png") {
		t.Fatalf("err = %v", err)
	}
}

func TestParse(t *testing.T) {
	obj := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"hi <b>"}]}]}`
	strEnc, _ := json.Marshal(obj)
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"object", obj, obj},
		{"string encoded", string(strEnc), obj},
		{"empty", "", `{"type":"doc"}`},
		{"null", " null ", `{"type":"doc"}`},
		{"empty string", `""`, `{"type":"doc"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, err := Parse([]byte(tt.raw))
			if err != nil {
				t.Fatal(err)
			}
			if got := mustJSON(t, n); got != tt.want {
				t.Fatalf("got %s want %s", got, tt.want)
			}
		})
	}
	if _, err := Parse([]byte("{bad")); err == nil {
		t.Fatal("expected error")
	}
}

func TestNoteDoc(t *testing.T) {
	got := mustJSON(t, NoteDoc("first line\nsame para\r\n\r\n\n\n  second  \n\n"))
	want := `{"type":"doc","attrs":{"schemaVersion":"v1"},"content":[{"type":"paragraph","content":[{"type":"text","text":"first line\nsame para"}]},{"type":"paragraph","content":[{"type":"text","text":"second"}]}]}`
	if got != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}
	if got := mustJSON(t, NoteDoc("  \n\n ")); got != `{"type":"doc","attrs":{"schemaVersion":"v1"}}` {
		t.Fatalf("empty note: %s", got)
	}
}

func TestPlainText(t *testing.T) {
	res, err := FromMarkdown(context.Background(), "# T\n\nOne **two**\n\n- a\n- b", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := PlainText(res.Doc); got != "T\nOne two\na\nb" {
		t.Fatalf("got %q", got)
	}
	if PlainText(nil) != "" {
		t.Fatal("nil doc")
	}
}

func TestReviewEdgeCases(t *testing.T) {
	ctx := context.Background()
	if _, err := FromMarkdown(ctx, "> <!-- paywall -->", Options{}); err == nil {
		t.Error("nested paywall should be rejected")
	}
	res, err := FromMarkdown(ctx, "a\n\n<!-- paywall -->\n\nb\n\n<!-- paywall -->\n\nc", Options{})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, b := range res.Doc.Content {
		if b.Type == "paywall" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("want one paywall, got %d", n)
	}
	res, _ = FromMarkdown(ctx, ">", Options{})
	if q := res.Doc.Content[0]; q.Type != "blockquote" || len(q.Content) != 1 {
		t.Errorf("empty quote should hold a paragraph: %+v", q)
	}
	res, _ = FromMarkdown(ctx, "Hello<br>world", Options{})
	if p := res.Doc.Content[0]; len(p.Content) != 3 || p.Content[1].Type != "hard_break" {
		t.Errorf("<br> should be a hard_break: %+v", p.Content)
	}
}

func TestImagePathWithSpaces(t *testing.T) {
	var got []string
	res, err := FromMarkdown(context.Background(), "![a](/My Files/x.png \"cap\")\n\n```\n![b](/not touched.png)\n```", Options{
		Images: func(_ context.Context, src string) (Image, error) {
			got = append(got, src)
			return Image{URL: "https://cdn/x.png"}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "/My Files/x.png" {
		t.Errorf("resolver got %q", got)
	}
	if res.Doc.Content[0].Type != "captionedImage" || res.Images != 1 {
		t.Errorf("first block = %+v", res.Doc.Content[0])
	}
}

func TestSubscribeMarker(t *testing.T) {
	res, err := FromMarkdown(context.Background(), "a\n\n<!-- subscribe -->\n\nb", Options{})
	if err != nil {
		t.Fatal(err)
	}
	w := res.Doc.Content[1]
	if w.Type != "subscribeWidget" || w.Content[0].Type != "ctaCaption" {
		t.Fatalf("got %+v", w)
	}
	if md := ToMarkdown(res.Doc); !strings.Contains(md, "<!-- subscribe -->") {
		t.Errorf("round trip lost the marker: %q", md)
	}
}
