package prosemirror

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Image is what an ImageResolver returns after hosting an image on Substack's CDN.
type Image struct {
	URL         string
	Width       int
	Height      int
	Bytes       int64
	ContentType string
}

// ImageResolver uploads or otherwise resolves an image source to a hosted image.
type ImageResolver func(ctx context.Context, src string) (Image, error)

// Options controls FromMarkdown.
type Options struct {
	// DropTitle drops the first block when it is a heading whose text equals
	// this value (case-insensitive, trimmed).
	DropTitle string
	// Images resolves image sources. When nil, sources are kept as-is.
	Images ImageResolver
	// Subscribe is the subscribe button inserted for <!-- subscribe -->.
	// When nil, DefaultSubscribe is used.
	Subscribe *Node
}

// DefaultSubscribe is Substack's standard subscribe button.
func DefaultSubscribe() *Node {
	return &Node{
		Type:  "subscribeWidget",
		Attrs: map[string]any{"url": "%%checkout_url%%", "text": "Subscribe", "language": "en"},
		Content: []*Node{{Type: "ctaCaption", Content: []*Node{{Type: "text",
			Text: "Thanks for reading! Subscribe for free to receive new posts and support my work."}}}},
	}
}

// HasSubscribe reports whether md asks for a subscribe button.
func HasSubscribe(md string) bool { return subscribeRe.MatchString(md) }

// Result is the output of FromMarkdown.
type Result struct {
	Doc        *Node
	HasPaywall bool
	Images     int
}

var (
	paywallRe   = regexp.MustCompile(`(?i)<!--\s*paywall\s*-->`)
	subscribeRe = regexp.MustCompile(`(?i)<!--\s*subscribe\s*-->`)
	brRe        = regexp.MustCompile(`(?i)^<br\s*/?>$`)
	// imageRe matches ![alt](dest "title") where dest may contain spaces.
	imageRe = regexp.MustCompile(`!\[([^\]]*)\]\(([^()<>"\n]*?)(\s+"[^"\n]*")?\)`)
)

// bracketImagePaths wraps image destinations that contain spaces in <...>,
// which CommonMark requires. Local paths such as "/My Files/a.png" are common
// and would otherwise be left as literal text. Fenced code is not touched.
func bracketImagePaths(src string) string {
	if !strings.Contains(src, "![") {
		return src
	}
	lines := strings.SplitAfter(src, "\n")
	fence := ""
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if fence == "" && (strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~")) {
			fence = t[:3]
			continue
		}
		if fence != "" {
			if strings.HasPrefix(t, fence) {
				fence = ""
			}
			continue
		}
		lines[i] = imageRe.ReplaceAllStringFunc(line, func(m string) string {
			g := imageRe.FindStringSubmatch(m)
			dest := strings.TrimSpace(g[2])
			if !strings.Contains(dest, " ") {
				return m
			}
			return "![" + g[1] + "](<" + dest + ">" + g[3] + ")"
		})
	}
	return strings.Join(lines, "")
}

var md = goldmark.New(goldmark.WithExtensions(
	extension.Strikethrough,
	extension.Linkify,
	extension.Table,
))

// FromMarkdown converts Markdown into a Substack ProseMirror document.
func FromMarkdown(ctx context.Context, src string, opt Options) (Result, error) {
	source := []byte(bracketImagePaths(src))
	root := md.Parser().Parse(text.NewReader(source))
	c := &converter{ctx: ctx, src: source, opt: opt}
	content := c.blocks(root)
	if c.err != nil {
		return Result{}, c.err
	}
	if t := strings.TrimSpace(opt.DropTitle); t != "" && len(content) > 0 && content[0].Type == "heading" &&
		strings.EqualFold(strings.TrimSpace(PlainText(content[0])), t) {
		content = content[1:]
	}
	return Result{
		Doc:        &Node{Type: "doc", Content: content},
		HasPaywall: c.paywall,
		Images:     c.images,
	}, nil
}

type converter struct {
	ctx     context.Context
	src     []byte
	opt     Options
	err     error
	paywall bool
	images  int
}

func (c *converter) blocks(parent ast.Node) []*Node {
	var out []*Node
	for n := parent.FirstChild(); n != nil && c.err == nil; n = n.NextSibling() {
		out = append(out, c.block(n)...)
	}
	return out
}

func (c *converter) block(n ast.Node) []*Node {
	switch n := n.(type) {
	case *ast.Heading:
		level := n.Level
		return c.inlineBlocks(n, func(in []*Node) *Node {
			return &Node{Type: "heading", Attrs: map[string]any{"level": level}, Content: in}
		})
	case *ast.Paragraph, *ast.TextBlock:
		if html, ok := c.onlyRawHTML(n); ok {
			if paywallRe.MatchString(html) {
				return c.paywallNode(n)
			}
			if subscribeRe.MatchString(html) {
				return []*Node{c.subscribe()}
			}
			return nil
		}
		return c.inlineBlocks(n, func(in []*Node) *Node {
			return &Node{Type: "paragraph", Content: in}
		})
	case *ast.Blockquote:
		content := c.blocks(n)
		if len(content) == 0 {
			content = []*Node{{Type: "paragraph"}}
		}
		return []*Node{{Type: "blockquote", Content: content}}
	case *ast.List:
		list := &Node{Type: "bullet_list"}
		if n.IsOrdered() {
			list.Type = "ordered_list"
			list.Attrs = map[string]any{"start": n.Start, "order": n.Start}
		}
		for item := n.FirstChild(); item != nil; item = item.NextSibling() {
			content := c.blocks(item)
			if len(content) == 0 {
				content = []*Node{{Type: "paragraph"}}
			}
			list.Content = append(list.Content, &Node{Type: "list_item", Content: content})
		}
		return []*Node{list}
	case *ast.FencedCodeBlock:
		var attrs map[string]any
		if n.Info != nil {
			if lang := n.Language(c.src); len(lang) > 0 {
				attrs = map[string]any{"language": string(lang)}
			}
		}
		return []*Node{codeBlock(c.lines(n.Lines()), attrs)}
	case *ast.CodeBlock:
		return []*Node{codeBlock(c.lines(n.Lines()), nil)}
	case *ast.ThematicBreak:
		return []*Node{{Type: "horizontal_rule"}}
	case *ast.HTMLBlock:
		html := c.lines(n.Lines())
		if n.HasClosure() {
			html += string(n.ClosureLine.Value(c.src))
		}
		if paywallRe.MatchString(html) {
			return c.paywallNode(n)
		}
		if subscribeRe.MatchString(html) {
			return []*Node{c.subscribe()}
		}
		return nil
	case *east.Table:
		return []*Node{codeBlock(c.tableSource(n), nil)}
	default:
		return c.blocks(n)
	}
}

// paywallNode places the paywall. Substack supports a single one, at the top
// level of the document.
func (c *converter) paywallNode(n ast.Node) []*Node {
	if n.Parent() == nil || n.Parent().Kind() != ast.KindDocument {
		c.err = fmt.Errorf("prosemirror: the paywall marker must be on its own line, outside lists and quotes")
		return nil
	}
	if c.paywall {
		return nil
	}
	c.paywall = true
	return []*Node{{Type: "paywall"}}
}

// subscribe returns a fresh copy of the subscribe button template.
func (c *converter) subscribe() *Node {
	tpl := c.opt.Subscribe
	if tpl == nil {
		tpl = DefaultSubscribe()
	}
	b, _ := json.Marshal(tpl)
	var n Node
	_ = json.Unmarshal(b, &n)
	return &n
}

func codeBlock(code string, attrs map[string]any) *Node {
	n := &Node{Type: "code_block", Attrs: attrs}
	code = strings.TrimRight(code, "\n")
	if code != "" {
		n.Content = []*Node{{Type: "text", Text: code}}
	}
	return n
}

func (c *converter) lines(lines *text.Segments) string {
	var b bytes.Buffer
	for i := range lines.Len() {
		seg := lines.At(i)
		b.Write(seg.Value(c.src))
	}
	return b.String()
}

// tableSource rebuilds the table's Markdown from its row lines; the delimiter
// row is regenerated from the parsed alignments since goldmark drops it.
func (c *converter) tableSource(t *east.Table) string {
	var rows []string
	for r := t.FirstChild(); r != nil; r = r.NextSibling() {
		rows = append(rows, c.lineAt(r.Pos()))
		if _, ok := r.(*east.TableHeader); ok {
			cells := make([]string, len(t.Alignments))
			for i, a := range t.Alignments {
				switch a {
				case east.AlignLeft:
					cells[i] = ":---"
				case east.AlignRight:
					cells[i] = "---:"
				case east.AlignCenter:
					cells[i] = ":---:"
				default:
					cells[i] = "---"
				}
			}
			rows = append(rows, "| "+strings.Join(cells, " | ")+" |")
		}
	}
	return strings.Join(rows, "\n")
}

func (c *converter) lineAt(pos int) string {
	if pos < 0 || pos > len(c.src) {
		return ""
	}
	line := c.src[pos:]
	if i := bytes.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	return strings.TrimSpace(string(line))
}

// onlyRawHTML reports whether a paragraph consists solely of inline raw HTML
// (plus whitespace) and returns that HTML.
func (c *converter) onlyRawHTML(n ast.Node) (string, bool) {
	var b strings.Builder
	found := false
	for ch := n.FirstChild(); ch != nil; ch = ch.NextSibling() {
		switch ch := ch.(type) {
		case *ast.RawHTML:
			found = true
			for i := range ch.Segments.Len() {
				seg := ch.Segments.At(i)
				b.Write(seg.Value(c.src))
			}
		case *ast.Text:
			if !util.IsBlank(ch.Value(c.src)) {
				return "", false
			}
		default:
			return "", false
		}
	}
	return b.String(), found
}

// inlineCollector gathers inline nodes for a block, splitting it around images.
type inlineCollector struct {
	wrap  func([]*Node) *Node
	out   []*Node
	cur   []*Node
	marks []Mark
}

func (ic *inlineCollector) flush() {
	cur := trimInline(ic.cur)
	ic.cur = nil
	if len(cur) > 0 {
		ic.out = append(ic.out, ic.wrap(cur))
	}
}

func (ic *inlineCollector) text(s string) {
	if s == "" {
		return
	}
	marks := sortedMarks(ic.marks)
	if k := len(ic.cur); k > 0 && ic.cur[k-1].Type == "text" && marksEqual(ic.cur[k-1].Marks, marks) {
		ic.cur[k-1].Text += s
		return
	}
	ic.cur = append(ic.cur, &Node{Type: "text", Text: s, Marks: marks})
}

func (ic *inlineCollector) push(m Mark) { ic.marks = append(ic.marks, m) }
func (ic *inlineCollector) pop()        { ic.marks = ic.marks[:len(ic.marks)-1] }

func (c *converter) inlineBlocks(n ast.Node, wrap func([]*Node) *Node) []*Node {
	ic := &inlineCollector{wrap: wrap}
	c.inlines(ic, n)
	ic.flush()
	return ic.out
}

func (c *converter) inlines(ic *inlineCollector, parent ast.Node) {
	for n := parent.FirstChild(); n != nil && c.err == nil; n = n.NextSibling() {
		c.inline(ic, n)
	}
}

func (c *converter) inline(ic *inlineCollector, n ast.Node) {
	switch n := n.(type) {
	case *ast.Text:
		v := n.Value(c.src)
		if !n.IsRaw() {
			v = unescape(v)
		}
		ic.text(string(v))
		switch {
		case n.HardLineBreak():
			ic.cur = append(ic.cur, &Node{Type: "hard_break"})
		case n.SoftLineBreak():
			ic.text(" ")
		}
	case *ast.String:
		v := n.Value
		if !n.IsRaw() && !n.IsCode() {
			v = unescape(v)
		}
		ic.text(string(v))
	case *ast.CodeSpan:
		var b bytes.Buffer
		for ch := n.FirstChild(); ch != nil; ch = ch.NextSibling() {
			switch t := ch.(type) {
			case *ast.Text:
				v := t.Value(c.src)
				if bytes.HasSuffix(v, []byte("\n")) {
					v = append(v[:len(v)-1:len(v)-1], ' ')
				}
				b.Write(v)
			case *ast.String:
				b.Write(t.Value)
			}
		}
		ic.push(Mark{Type: "code"})
		ic.text(b.String())
		ic.pop()
	case *ast.Emphasis:
		typ := "em"
		if n.Level >= 2 {
			typ = "strong"
		}
		ic.push(Mark{Type: typ})
		c.inlines(ic, n)
		ic.pop()
	case *east.Strikethrough:
		ic.push(Mark{Type: "strikethrough"})
		c.inlines(ic, n)
		ic.pop()
	case *ast.Link:
		ic.push(Mark{Type: "link", Attrs: map[string]any{"href": string(unescape(n.Destination))}})
		c.inlines(ic, n)
		ic.pop()
	case *ast.AutoLink:
		href := string(n.URL(c.src))
		if n.AutoLinkType == ast.AutoLinkEmail && !strings.HasPrefix(strings.ToLower(href), "mailto:") {
			href = "mailto:" + href
		}
		ic.push(Mark{Type: "link", Attrs: map[string]any{"href": href}})
		ic.text(string(n.Label(c.src)))
		ic.pop()
	case *ast.Image:
		ic.flush()
		img, err := c.image(n)
		if err != nil {
			c.err = err
			return
		}
		ic.out = append(ic.out, img)
	case *ast.RawHTML:
		// Inline HTML has no Substack equivalent, except line breaks.
		var raw strings.Builder
		for i := 0; i < n.Segments.Len(); i++ {
			seg := n.Segments.At(i)
			raw.Write(seg.Value(c.src))
		}
		if brRe.MatchString(raw.String()) {
			ic.cur = append(ic.cur, &Node{Type: "hard_break"})
		}
	default:
		c.inlines(ic, n)
	}
}

func (c *converter) image(n *ast.Image) (*Node, error) {
	src := string(unescape(n.Destination))
	img := Image{URL: src}
	if c.opt.Images != nil {
		var err error
		img, err = c.opt.Images(c.ctx, src)
		if err != nil {
			return nil, fmt.Errorf("prosemirror: resolve image %q: %w", src, err)
		}
		if img.URL == "" {
			img.URL = src
		}
	}
	c.images++
	alt := strings.TrimSpace(inlineText(n, c.src))
	attrs := map[string]any{
		"src":              img.URL,
		"srcNoWatermark":   nil,
		"fullscreen":       nil,
		"imageSize":        nil,
		"height":           nullInt(int64(img.Height)),
		"width":            nullInt(int64(img.Width)),
		"resizeWidth":      nil,
		"bytes":            nullInt(img.Bytes),
		"alt":              nullStr(alt),
		"title":            nil,
		"type":             nullStr(img.ContentType),
		"href":             nil,
		"belowTheFold":     false,
		"topImage":         false,
		"internalRedirect": nil,
		"isProcessing":     false,
		"align":            nil,
		"offset":           false,
	}
	node := &Node{Type: "captionedImage", Content: []*Node{{Type: "image2", Attrs: attrs}}}
	if caption := strings.TrimSpace(string(unescape(n.Title))); caption != "" {
		node.Content = append(node.Content, &Node{Type: "caption", Content: []*Node{{Type: "text", Text: caption}}})
	}
	return node, nil
}

func nullInt(v int64) any {
	if v <= 0 {
		return nil
	}
	return v
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// inlineText flattens the text content of an inline subtree (used for alt text).
func inlineText(n ast.Node, src []byte) string {
	var b strings.Builder
	_ = ast.Walk(n, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := n.(type) {
		case *ast.Text:
			b.Write(unescape(t.Value(src)))
			if t.SoftLineBreak() || t.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(t.Value)
		case *ast.AutoLink:
			b.Write(t.Label(src))
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

func unescape(v []byte) []byte {
	return util.ResolveEntityNames(util.ResolveNumericReferences(util.UnescapePunctuations(v)))
}

// trimInline removes leading and trailing whitespace-only content and hard
// breaks at the edges of a block.
func trimInline(in []*Node) []*Node {
	for len(in) > 0 && in[0].Type == "hard_break" {
		in = in[1:]
	}
	for len(in) > 0 && in[len(in)-1].Type == "hard_break" {
		in = in[:len(in)-1]
	}
	if len(in) == 0 {
		return nil
	}
	if first := in[0]; first.Type == "text" {
		first.Text = strings.TrimLeft(first.Text, " \t\n")
		if first.Text == "" {
			return trimInline(in[1:])
		}
	}
	if last := in[len(in)-1]; last.Type == "text" {
		last.Text = strings.TrimRight(last.Text, " \t\n")
		if last.Text == "" {
			return trimInline(in[:len(in)-1])
		}
	}
	return in
}

var markRank = map[string]int{"link": 0, "em": 1, "strong": 2, "strikethrough": 3, "code": 4}

// sortedMarks returns a copy of marks in schema order, deduplicated by type.
func sortedMarks(marks []Mark) []Mark {
	if len(marks) == 0 {
		return nil
	}
	out := make([]Mark, 0, len(marks))
	for _, m := range marks {
		if i := slices.IndexFunc(out, func(o Mark) bool { return o.Type == m.Type }); i >= 0 {
			out[i] = m // innermost link wins
			continue
		}
		out = append(out, m)
	}
	slices.SortStableFunc(out, func(a, b Mark) int { return rank(a.Type) - rank(b.Type) })
	return out
}

func rank(t string) int {
	if r, ok := markRank[t]; ok {
		return r
	}
	return len(markRank)
}

func marksEqual(a, b []Mark) bool {
	return slices.EqualFunc(a, b, markEqual)
}

func markEqual(a, b Mark) bool {
	if a.Type != b.Type || len(a.Attrs) != len(b.Attrs) {
		return false
	}
	for k, v := range a.Attrs {
		if fmt.Sprint(b.Attrs[k]) != fmt.Sprint(v) {
			return false
		}
	}
	return true
}
