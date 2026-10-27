package prosemirror

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ToMarkdown renders a ProseMirror document as Markdown.
func ToMarkdown(doc *Node) string {
	if doc == nil {
		return ""
	}
	var out string
	if doc.Text != "" && len(doc.Content) == 0 {
		out = renderInline([]*Node{doc}, false)
	} else {
		out = renderBlocks(doc.Content)
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return ""
	}
	return out + "\n"
}

func renderBlocks(nodes []*Node) string {
	return joinBlocks(nodes, "\n\n")
}

func joinBlocks(nodes []*Node, sep string) string {
	var parts []string
	var run []*Node
	flushRun := func() {
		if len(run) > 0 {
			if s := renderInline(run, false); s != "" {
				parts = append(parts, s)
			}
			run = nil
		}
	}
	prevList := ""
	alt := false
	for _, n := range nodes {
		if n == nil {
			continue
		}
		if isInlineNode(n) {
			run = append(run, n)
			continue
		}
		flushRun()
		// Adjacent lists of the same kind would merge; alternate the marker.
		kind := listKind(n.Type)
		if kind != "" && kind == prevList {
			alt = !alt
		} else {
			alt = false
		}
		prevList = kind
		var s string
		if kind != "" {
			s = renderList(n, alt)
		} else {
			s = renderBlock(n)
		}
		if s != "" {
			parts = append(parts, s)
		}
	}
	flushRun()
	return strings.Join(parts, sep)
}

func isInlineNode(n *Node) bool {
	switch n.Type {
	case "text", "hard_break", "hardBreak":
		return true
	}
	return false
}

func listKind(t string) string {
	switch t {
	case "bullet_list", "bulletList":
		return "bullet"
	case "ordered_list", "orderedList":
		return "ordered"
	}
	return ""
}

func renderBlock(n *Node) string {
	switch n.Type {
	case "paragraph":
		return renderInline(n.Content, false)
	case "heading":
		level := min(max(intAttr(n.Attrs, 1, "level"), 1), 6)
		return strings.Repeat("#", level) + " " + renderInline(n.Content, true)
	case "blockquote":
		return prefixLines(renderBlocks(n.Content), "> ", ">")
	case "list_item", "listItem":
		return renderItemBody(n)
	case "code_block", "codeBlock":
		return renderCode(n)
	case "horizontal_rule", "horizontalRule":
		return "---"
	case "captionedImage":
		var img *Node
		caption := ""
		for _, c := range n.Content {
			switch c.Type {
			case "image2", "image":
				if img == nil {
					img = c
				}
			case "caption":
				caption = strings.TrimSpace(PlainText(c))
			}
		}
		if img == nil {
			return ""
		}
		return renderImage(img, caption)
	case "image2", "image":
		return renderImage(n, strAttr(n.Attrs, "title"))
	case "paywall":
		return "<!-- paywall -->"
	case "subscribeWidget":
		return "<!-- subscribe -->"
	case "youtube2":
		if id := strAttr(n.Attrs, "videoId"); id != "" {
			return "https://www.youtube.com/watch?v=" + id
		}
		return ""
	}
	if len(n.Content) == 0 {
		if n.Text != "" {
			return renderInline([]*Node{{Type: "text", Text: n.Text, Marks: n.Marks}}, false)
		}
		return ""
	}
	return renderBlocks(n.Content)
}

func renderList(n *Node, alt bool) string {
	ordered := listKind(n.Type) == "ordered"
	num := intAttr(n.Attrs, 1, "start", "order")
	var items []string
	for i, item := range n.Content {
		var marker string
		switch {
		case ordered && alt:
			marker = strconv.Itoa(num+i) + ") "
		case ordered:
			marker = strconv.Itoa(num+i) + ". "
		case alt:
			marker = "* "
		default:
			marker = "- "
		}
		body := renderItemBody(item)
		if body == "" {
			items = append(items, strings.TrimSpace(marker))
			continue
		}
		indent := strings.Repeat(" ", len(marker))
		lines := strings.Split(body, "\n")
		for j := range lines {
			switch {
			case j == 0:
				lines[j] = marker + lines[j]
			case lines[j] != "":
				lines[j] = indent + lines[j]
			}
		}
		items = append(items, strings.Join(lines, "\n"))
	}
	return strings.Join(items, "\n")
}

// renderItemBody keeps a paragraph followed by a nested list tight.
func renderItemBody(item *Node) string {
	if item.Type != "list_item" && item.Type != "listItem" {
		return renderBlocks([]*Node{item})
	}
	var b strings.Builder
	for i, c := range item.Content {
		s := renderBlocks([]*Node{c})
		if s == "" {
			continue
		}
		if b.Len() > 0 {
			if listKind(c.Type) != "" && item.Content[i-1].Type == "paragraph" {
				b.WriteString("\n")
			} else {
				b.WriteString("\n\n")
			}
		}
		b.WriteString(s)
	}
	return b.String()
}

func prefixLines(s, prefix, empty string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l == "" {
			lines[i] = empty
		} else {
			lines[i] = prefix + l
		}
	}
	return strings.Join(lines, "\n")
}

var fenceRun = regexp.MustCompile("(?m)^ {0,3}(`{3,})")

func renderCode(n *Node) string {
	var b strings.Builder
	for _, c := range n.Content {
		b.WriteString(c.Text)
	}
	code := strings.TrimRight(b.String(), "\n")
	fence := "```"
	for _, m := range fenceRun.FindAllStringSubmatch(code, -1) {
		if len(m[1]) >= len(fence) {
			fence = strings.Repeat("`", len(m[1])+1)
		}
	}
	lang := strAttr(n.Attrs, "language")
	if code == "" {
		return fence + lang + "\n" + fence
	}
	return fence + lang + "\n" + code + "\n" + fence
}

func renderImage(n *Node, caption string) string {
	src := strAttr(n.Attrs, "src")
	if src == "" {
		return ""
	}
	alt := strings.NewReplacer(`\`, `\\`, "[", `\[`, "]", `\]`).Replace(strAttr(n.Attrs, "alt"))
	s := "![" + alt + "](" + linkDest(src)
	if caption != "" {
		s += ` "` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(caption) + `"`
	}
	return s + ")"
}

func linkDest(u string) string {
	if strings.ContainsAny(u, " ()<>") {
		return "<" + strings.NewReplacer("<", "%3C", ">", "%3E").Replace(u) + ">"
	}
	return u
}

func intAttr(attrs map[string]any, def int, keys ...string) int {
	for _, k := range keys {
		switch v := attrs[k].(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		case string:
			if i, err := strconv.Atoi(v); err == nil {
				return i
			}
		}
	}
	return def
}

func strAttr(attrs map[string]any, key string) string {
	switch v := attrs[key].(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

// inlineWriter renders inline content, opening and closing mark delimiters
// so that marks spanning several text nodes stay continuous.
type inlineWriter struct {
	b         strings.Builder
	open      []Mark
	pendingWS string
	lineStart bool
	heading   bool
}

func renderInline(nodes []*Node, heading bool) string {
	w := &inlineWriter{lineStart: true, heading: heading}
	w.nodes(nodes)
	w.closeTo(0)
	return strings.TrimSpace(w.b.String())
}

func (w *inlineWriter) nodes(nodes []*Node) {
	for _, n := range nodes {
		if n != nil {
			w.node(n)
		}
	}
}

func (w *inlineWriter) node(n *Node) {
	switch n.Type {
	case "text":
		w.text(n)
	case "hard_break", "hardBreak":
		w.pendingWS = ""
		if w.heading {
			w.pendingWS = " "
			return
		}
		w.b.WriteString("\\\n")
		w.lineStart = true
	case "image", "image2":
		w.closeTo(0)
		w.flushWS()
		w.b.WriteString(renderImage(n, strAttr(n.Attrs, "title")))
		w.lineStart = false
	default:
		if n.Text != "" {
			w.text(&Node{Type: "text", Text: n.Text, Marks: n.Marks})
		}
		w.nodes(n.Content)
	}
}

func (w *inlineWriter) flushWS() {
	w.b.WriteString(w.pendingWS)
	w.pendingWS = ""
}

func (w *inlineWriter) closeTo(keep int) {
	for i := len(w.open) - 1; i >= keep; i-- {
		w.b.WriteString(closeDelim(w.open[i]))
	}
	w.open = w.open[:keep]
}

func (w *inlineWriter) text(n *Node) {
	var marks []Mark
	code := false
	for _, m := range sortedMarks(n.Marks) {
		if m.Type == "code" {
			code = true
			continue
		}
		if openDelim(m) == "" {
			continue
		}
		marks = append(marks, m)
	}

	if !code && strings.TrimSpace(n.Text) == "" {
		w.pendingWS += strings.ReplaceAll(n.Text, "\n", " ")
		return
	}

	keep := 0
	for keep < len(w.open) && containsMark(marks, w.open[keep]) {
		keep++
	}
	w.closeTo(keep)
	w.flushWS()

	txt := n.Text
	trail := ""
	if !code {
		core := strings.TrimLeft(txt, " \t")
		w.writePlain(txt[:len(txt)-len(core)])
		txt = strings.TrimRight(core, " \t")
		trail = core[len(txt):]
	}
	for _, m := range marks {
		if !containsMark(w.open, m) {
			w.b.WriteString(openDelim(m))
			w.open = append(w.open, m)
		}
	}
	if code {
		w.b.WriteString(codeSpan(strings.ReplaceAll(txt, "\n", " ")))
		w.lineStart = false
	} else {
		w.writeEscaped(txt)
	}
	w.pendingWS = trail
}

func (w *inlineWriter) writePlain(s string) { w.b.WriteString(s) }

func (w *inlineWriter) writeEscaped(s string) {
	for i, line := range strings.Split(s, "\n") {
		if i > 0 {
			if w.heading {
				w.b.WriteString(" ")
			} else {
				w.b.WriteString("\\\n")
				w.lineStart = true
			}
		}
		w.b.WriteString(escapeText(line, w.lineStart))
		if strings.TrimSpace(line) != "" {
			w.lineStart = false
		}
	}
}

func containsMark(set []Mark, m Mark) bool {
	for _, o := range set {
		if markEqual(o, m) {
			return true
		}
	}
	return false
}

func openDelim(m Mark) string {
	switch m.Type {
	case "strong":
		return "**"
	case "em":
		return "*"
	case "strikethrough":
		return "~~"
	case "link":
		return "["
	}
	return ""
}

func closeDelim(m Mark) string {
	if m.Type == "link" {
		return "](" + linkDest(strAttr(m.Attrs, "href")) + ")"
	}
	return openDelim(m)
}

func codeSpan(s string) string {
	longest, run := 0, 0
	for _, r := range s {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", longest+1)
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") ||
		(strings.HasPrefix(s, " ") && strings.HasSuffix(s, " ") && strings.TrimSpace(s) != "") {
		s = " " + s + " "
	}
	return fence + s + fence
}

var (
	entityLike    = regexp.MustCompile(`^&(#[0-9]{1,8}|#[xX][0-9a-fA-F]{1,7}|[A-Za-z][A-Za-z0-9]*);`)
	orderedPrefix = regexp.MustCompile(`^([0-9]{1,9})([.)])`)
)

// escapeText escapes characters that would otherwise turn plain text into
// Markdown formatting.
func escapeText(s string, lineStart bool) string {
	var b strings.Builder
	if lineStart {
		trimmed := strings.TrimLeft(s, " ")
		b.WriteString(s[:len(s)-len(trimmed)])
		s = trimmed
		if s != "" {
			switch s[0] {
			case '#', '>', '-', '+', '=':
				b.WriteByte('\\')
				b.WriteByte(s[0])
				s = s[1:]
			default:
				if m := orderedPrefix.FindStringSubmatch(s); m != nil {
					b.WriteString(m[1] + `\` + m[2])
					s = s[len(m[0]):]
				}
			}
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\', '*', '_', '`', '[', ']', '~':
			b.WriteByte('\\')
		case '<':
			if i+1 < len(s) && (isLetter(s[i+1]) || strings.ContainsRune("/!?", rune(s[i+1]))) {
				b.WriteByte('\\')
			}
		case '&':
			if entityLike.MatchString(s[i:]) {
				b.WriteByte('\\')
			}
		}
		b.WriteByte(c)
	}
	return b.String()
}

func isLetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
