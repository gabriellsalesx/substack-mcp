// Package prosemirror converts between Markdown and the ProseMirror JSON
// documents Substack uses for post and note bodies.
package prosemirror

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Node is a ProseMirror node as serialized by Substack.
type Node struct {
	Type    string         `json:"type"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Content []*Node        `json:"content,omitempty"`
	Marks   []Mark         `json:"marks,omitempty"`
	Text    string         `json:"text,omitempty"`
}

// Mark is a ProseMirror inline mark (strong, em, link, ...).
type Mark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

// Parse decodes a document from either a JSON object or a JSON string that
// itself contains the encoded document. Empty input or null yields an empty doc.
func Parse(raw []byte) (*Node, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return &Node{Type: "doc"}, nil
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("prosemirror: decode string body: %w", err)
		}
		return Parse([]byte(s))
	}
	var n Node
	if err := json.Unmarshal(raw, &n); err != nil {
		return nil, fmt.Errorf("prosemirror: decode doc: %w", err)
	}
	if n.Type == "" {
		n.Type = "doc"
	}
	return &n, nil
}

// String marshals the node to compact JSON without HTML escaping.
func (n *Node) String() (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(n); err != nil {
		return "", fmt.Errorf("prosemirror: encode doc: %w", err)
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

// NoteDoc builds a Substack Notes body with one paragraph per blank-line
// separated block. Text inside a block is kept verbatim.
func NoteDoc(text string) *Node {
	doc := &Node{Type: "doc", Attrs: map[string]any{"schemaVersion": "v1"}}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	for block := range strings.SplitSeq(text, "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		doc.Content = append(doc.Content, &Node{
			Type:    "paragraph",
			Content: []*Node{{Type: "text", Text: block}},
		})
	}
	return doc
}

// PlainText returns the document text with block boundaries as newlines.
func PlainText(doc *Node) string {
	if doc == nil {
		return ""
	}
	var b strings.Builder
	plainText(&b, doc)
	return strings.TrimSpace(b.String())
}

func plainText(b *strings.Builder, n *Node) {
	switch n.Type {
	case "text":
		b.WriteString(n.Text)
		return
	case "hard_break", "hardBreak":
		b.WriteByte('\n')
		return
	}
	for _, c := range n.Content {
		plainText(b, c)
	}
	if n.Type != "doc" && !isInline(n) && !strings.HasSuffix(b.String(), "\n") {
		b.WriteByte('\n')
	}
}

func isInline(n *Node) bool {
	switch n.Type {
	case "text", "hard_break", "hardBreak", "image":
		return true
	}
	return false
}

// Find returns the first node of the given type, depth first, or nil.
func (n *Node) Find(typ string) *Node {
	if n == nil {
		return nil
	}
	if n.Type == typ {
		return n
	}
	for _, c := range n.Content {
		if f := c.Find(typ); f != nil {
			return f
		}
	}
	return nil
}
