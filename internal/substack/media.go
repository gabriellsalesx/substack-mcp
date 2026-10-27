package substack

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// MaxImageBytes caps local uploads; Substack itself rejects very large files.
const MaxImageBytes = 15 << 20

// UploadedImage is Substack's answer to an image upload.
type UploadedImage struct {
	URL         string `json:"url"`
	ContentType string `json:"contentType"`
	Bytes       int64  `json:"bytes"`
	Width       int    `json:"imageWidth"`
	Height      int    `json:"imageHeight"`
}

var imageTypes = map[string]bool{
	"image/jpeg": true, "image/png": true, "image/gif": true, "image/webp": true,
}

// UploadImage hosts an image on Substack's CDN. src is a local path (absolute,
// relative to the working directory, ~/..., or file://) or an http(s) URL,
// which Substack fetches and re-hosts.
func (c *Client) UploadImage(ctx context.Context, pub Publication, src string) (*UploadedImage, error) {
	value := src
	if !strings.HasPrefix(src, "http://") && !strings.HasPrefix(src, "https://") {
		data, mime, err := readImage(src)
		if err != nil {
			return nil, err
		}
		value = "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
	}
	img := new(UploadedImage)
	r := request{Method: "POST", URL: c.pubAPI(pub) + "/image", Form: url.Values{"image": {value}}, Timeout: 3 * time.Minute}
	if err := c.do(ctx, r, img); err != nil {
		return nil, err
	}
	if img.URL == "" {
		return nil, fmt.Errorf("upload of %s returned no URL", src)
	}
	return img, nil
}

func readImage(src string) ([]byte, string, error) {
	path := strings.TrimPrefix(src, "file://")
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, "", err
		}
		path = filepath.Join(home, rest)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, "", fmt.Errorf("open image %q: %w (use an absolute path)", src, err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, "", err
	}
	if !fi.Mode().IsRegular() {
		return nil, "", fmt.Errorf("%q is not a regular file", src)
	}
	if fi.Size() > MaxImageBytes {
		return nil, "", fmt.Errorf("%q is %d MB; the limit is %d MB", src, fi.Size()>>20, MaxImageBytes>>20)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, "", err
	}
	mime := http.DetectContentType(data)
	if !imageTypes[mime] {
		return nil, "", fmt.Errorf("%q is %s; only JPEG, PNG, GIF and WebP are supported", src, mime)
	}
	return data, mime, nil
}

// Note is a Substack Note (a short post on the Notes feed) or a scheduled draft of one.
type Note struct {
	ID        ID     `json:"id"`
	Status    string `json:"status,omitempty"`
	Type      string `json:"type,omitempty"`
	Body      string `json:"body,omitempty"`
	Date      Time   `json:"date"`
	TriggerAt Time   `json:"trigger_at"`
	UpdatedAt Time   `json:"updated_at"`
}

// NoteInput is a note to post or schedule. BodyJSON is a ProseMirror doc.
type NoteInput struct {
	BodyJSON any
	LinkURL  string
	At       time.Time // zero = post now
}

// CreateNote posts a Note now, or schedules it when in.At is set.
func (c *Client) CreateNote(ctx context.Context, pub Publication, in NoteInput) (*Note, error) {
	body := map[string]any{
		"bodyJson":         in.BodyJSON,
		"tabId":            "for-you",
		"surface":          "feed",
		"replyMinimumRole": "everyone",
	}
	if in.LinkURL != "" {
		var att struct {
			ID any `json:"id"`
		}
		if err := c.do(ctx, request{Method: "POST", URL: c.Root + "/comment/attachment", Body: map[string]any{"url": in.LinkURL, "type": "link"}}, &att); err != nil {
			return nil, fmt.Errorf("attach link: %w", err)
		}
		if att.ID == nil {
			return nil, fmt.Errorf("attach link: Substack returned no attachment id for %s", in.LinkURL)
		}
		body["attachmentIds"] = []any{att.ID}
	}
	n := new(Note)
	if in.At.IsZero() {
		// The trailing slash avoids a redirect that would turn the POST into a GET.
		err := c.do(ctx, request{Method: "POST", URL: c.Root + "/comment/feed/", Body: body}, n)
		return n, err
	}
	body["parent_id"] = nil
	body["publication_id"] = pub.ID
	body["trigger_at"] = in.At.UTC().Format("2006-01-02T15:04:05Z")
	err := c.do(ctx, request{Method: "POST", URL: c.Root + "/comment/draft", Body: body}, n)
	return n, err
}

// ListNoteDrafts lists scheduled notes. cursor comes from a previous call.
func (c *Client) ListNoteDrafts(ctx context.Context, limit int, cursor string) ([]Note, string, error) {
	q := url.Values{"limit": {strconv.Itoa(limit)}}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	var resp struct {
		Drafts     []Note `json:"drafts"`
		NextCursor string `json:"nextCursor"`
	}
	if err := c.do(ctx, request{Method: "GET", URL: c.Root + "/feed/drafts", Query: q}, &resp); err != nil {
		return nil, "", err
	}
	out := resp.Drafts[:0]
	for _, n := range resp.Drafts {
		if n.Type == "" || n.Type == "feed" {
			out = append(out, n)
		}
	}
	return out, resp.NextCursor, nil
}

// DeleteNote deletes a note or cancels a scheduled one.
func (c *Client) DeleteNote(ctx context.Context, id string) error {
	return c.do(ctx, request{Method: "DELETE", URL: c.Root + "/comment/" + url.PathEscape(id)}, nil)
}

// ID is an identifier Substack sends either as a number or as a string.
type ID string

func (id *ID) UnmarshalJSON(b []byte) error {
	*id = ID(strings.Trim(string(b), `"`))
	if *id == "null" {
		*id = ""
	}
	return nil
}

// CommunityPost is a thread in the publication's subscriber chat.
type CommunityPost struct {
	ID       ID     `json:"id"`
	Status   string `json:"status,omitempty"`
	Audience string `json:"audience,omitempty"`
	Body     string `json:"body,omitempty"`
}

// CommunityInput is a subscriber chat thread to start.
type CommunityInput struct {
	Body      string
	Audience  string
	LinkURL   string
	SendEmail bool
	SendPush  bool
}

// CreateCommunityPost starts a thread in the publication's subscriber chat.
func (c *Client) CreateCommunityPost(ctx context.Context, pub Publication, in CommunityInput) (*CommunityPost, error) {
	body := map[string]any{
		"id":         uuid4(),
		"body":       in.Body,
		"audience":   firstNonEmpty(in.Audience, "all_subscribers"),
		"type":       "text",
		"send_email": in.SendEmail,
		"send_push":  in.SendPush,
	}
	if in.LinkURL != "" {
		body["type"] = "link"
		body["link_url"] = in.LinkURL
	}
	var resp struct {
		CommunityPost CommunityPost `json:"communityPost"`
	}
	err := c.do(ctx, request{Method: "POST", URL: c.pubAPI(pub) + "/community/publications/" + strconv.FormatInt(pub.ID, 10) + "/posts", Body: body}, &resp)
	return &resp.CommunityPost, err
}

func uuid4() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
