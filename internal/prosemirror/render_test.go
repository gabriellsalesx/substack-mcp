package prosemirror

import (
	"context"
	"reflect"
	"testing"
)

func TestToMarkdown(t *testing.T) {
	item := func(c ...*Node) *Node { return &Node{Type: "list_item", Content: c} }
	tests := []struct {
		name string
		doc  *Node
		want string
	}{
		{"nil", nil, ""},
		{"empty", doc(), ""},
		{"paragraphs", doc(p(txt("a")), p(txt("b"))), "a\n\nb\n"},
		{"heading", doc(&Node{Type: "heading", Attrs: map[string]any{"level": 3.0}, Content: []*Node{txt("Hi")}}), "### Hi\n"},
		{"marks", doc(p(txt("a", strong), txt(" "), txt("b", em), txt(" "), txt("c", code), txt(" "), txt("d", strike))), "**a** *b* `c` ~~d~~\n"},
		{"continuous marks", doc(p(txt("a ", strong), txt("b", strong, em), txt(" c", strong))), "**a *b* c**\n"},
		{"whitespace outside delimiters", doc(p(txt("x"), txt(" bold ", strong), txt("y"))), "x **bold** y\n"},
		{"link", doc(p(txt("see "), txt("docs", link("https://x.dev/a b")))), "see [docs](<https://x.dev/a b>)\n"},
		{"code with backticks", doc(p(txt("a`b", code))), "``a`b``\n"},
		{"escaping", doc(p(txt("# 1. *a* _b_ [c] `d` <tag> &amp; R&D"))), "\\# 1. \\*a\\* \\_b\\_ \\[c\\] \\`d\\` \\<tag> \\&amp; R&D\n"},
		{"line start list", doc(p(txt("- x")), p(txt("2. y")), p(txt("> z"))), "\\- x\n\n2\\. y\n\n\\> z\n"},
		{"hard break", doc(p(txt("a"), &Node{Type: "hard_break"}, txt("- b"))), "a\\\n\\- b\n"},
		{"blockquote nested", doc(&Node{Type: "blockquote", Content: []*Node{p(txt("a")), {Type: "blockquote", Content: []*Node{p(txt("b"))}}}}), "> a\n>\n> > b\n"},
		{"bullet nested", doc(&Node{Type: "bullet_list", Content: []*Node{
			item(p(txt("a")), &Node{Type: "bullet_list", Content: []*Node{item(p(txt("b")))}}), item(p(txt("c")))}}),
			"- a\n  - b\n- c\n"},
		{"ordered start", doc(&Node{Type: "ordered_list", Attrs: map[string]any{"order": 9.0}, Content: []*Node{item(p(txt("a"))), item(p(txt("b")))}}),
			"9. a\n10. b\n"},
		{"ordered default", doc(&Node{Type: "ordered_list", Content: []*Node{item(p(txt("a")), p(txt("b")))}}), "1. a\n\n   b\n"},
		{"code block", doc(&Node{Type: "code_block", Attrs: map[string]any{"language": "go"}, Content: []*Node{txt("x\n```\ny")}}), "````go\nx\n```\ny\n````\n"},
		{"codeBlock camel", doc(&Node{Type: "codeBlock", Content: []*Node{txt("*x*")}}), "```\n*x*\n```\n"},
		{"rule and paywall", doc(&Node{Type: "horizontal_rule"}, &Node{Type: "paywall"}), "---\n\n<!-- paywall -->\n"},
		{"captioned image", doc(&Node{Type: "captionedImage", Content: []*Node{
			{Type: "image2", Attrs: map[string]any{"src": "https://c/x.png", "alt": "A [b]"}},
			{Type: "caption", Content: []*Node{txt(`Say "hi"`)}}}}), "![A \\[b\\]](https://c/x.png \"Say \\\"hi\\\"\")\n"},
		{"image without alt", doc(&Node{Type: "image2", Attrs: map[string]any{"src": "x.png", "alt": nil}}), "![](x.png)\n"},
		{"youtube", doc(&Node{Type: "youtube2", Attrs: map[string]any{"videoId": "abc"}}), "https://www.youtube.com/watch?v=abc\n"},
		{"unknown block", doc(&Node{Type: "pullquote", Content: []*Node{p(txt("q"))}}), "q\n"},
		{"unknown inline container", doc(&Node{Type: "footnote", Content: []*Node{txt("f")}}), "f\n"},
		{"adjacent lists", doc(&Node{Type: "bullet_list", Content: []*Node{item(p(txt("a")))}}, &Node{Type: "bullet_list", Content: []*Node{item(p(txt("b")))}}),
			"- a\n\n* b\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ToMarkdown(tt.doc); got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	src := `## Section *one*

Intro with **bold**, *em*, ***both***, ` + "`code`" + `, ~~gone~~ and a [link](https://example.com/x?a=1&b=2).
Soft wrapped line with a hard break  
right here. Special chars: \*stars\*, snake\_case, \[brackets\], 3 < 4 & 5 > 2.

![Diagram](https://cdn.example.com/d.png "How it works")

> Quoted text
>
> - inside quote
> - second

1. first
2. second
   - nested **bold**
   - nested two
3. third

5. starts at five

- loose item

  with two paragraphs

` + "```bash\necho \"hi\"\n```" + `

<!-- paywall -->

---

| col | val |
|-----|-----|
| a   | 1   |

\# not a heading and 1\. not a list
`
	ctx := context.Background()
	first, err := FromMarkdown(ctx, src, Options{})
	if err != nil {
		t.Fatal(err)
	}
	md := ToMarkdown(first.Doc)
	second, err := FromMarkdown(ctx, md, Options{})
	if err != nil {
		t.Fatal(err)
	}
	a, b := mustJSON(t, first.Doc), mustJSON(t, second.Doc)
	if a != b {
		t.Fatalf("round trip mismatch\nmarkdown:\n%s\nfirst:  %s\nsecond: %s", md, a, b)
	}
	if !first.HasPaywall || !second.HasPaywall || first.Images != 1 || second.Images != 1 {
		t.Fatalf("metadata lost: %+v %+v", first, second)
	}
	// A doc decoded from JSON (float attrs) renders the same Markdown.
	parsed, err := Parse([]byte(a))
	if err != nil {
		t.Fatal(err)
	}
	if got := ToMarkdown(parsed); got != md {
		t.Fatalf("parsed doc renders differently:\n%s\nvs\n%s", got, md)
	}
	if !reflect.DeepEqual(normalize(t, parsed), normalize(t, first.Doc)) {
		t.Fatal("Parse(String()) changed the doc")
	}
}
