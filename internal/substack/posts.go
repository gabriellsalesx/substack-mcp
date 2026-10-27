package substack

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Post is a draft or published post as returned by /drafts/{id}.
type Post struct {
	ID            int64           `json:"id"`
	Title         string          `json:"title"`
	Subtitle      string          `json:"subtitle"`
	DraftTitle    string          `json:"draft_title"`
	DraftSubtitle string          `json:"draft_subtitle"`
	Slug          string          `json:"slug"`
	Audience      string          `json:"audience"`
	Type          string          `json:"type"`
	PostDate      Time            `json:"post_date"`
	IsPublished   bool            `json:"is_published"`
	SectionID     *int64          `json:"section_id"`
	DraftSection  *int64          `json:"draft_section_id"`
	Body          json.RawMessage `json:"body"`
	DraftBody     json.RawMessage `json:"draft_body"`
	DraftUpdated  Time            `json:"draft_updated_at"`
	WordCount     int             `json:"word_count"`
	Schedules     []struct {
		TriggerAt     Time   `json:"trigger_at"`
		EmailAudience string `json:"email_audience"`
	} `json:"postSchedules"`
	CanonicalURL string `json:"canonical_url"`
}

// Status is "draft", "scheduled" or "published".
func (p *Post) Status() string {
	switch {
	case p.IsPublished:
		return "published"
	case !p.ScheduledFor().IsZero():
		return "scheduled"
	case p.PostDate.IsZero():
		return "draft"
	default:
		return "published"
	}
}

// ScheduledFor is when a scheduled post goes out, or zero.
func (p *Post) ScheduledFor() time.Time {
	for _, s := range p.Schedules {
		if s.TriggerAt.After(time.Now()) {
			return s.TriggerAt.Time
		}
	}
	return time.Time{}
}

// CurrentTitle prefers the draft fields, which hold unpublished edits.
func (p *Post) CurrentTitle() string    { return firstNonEmpty(p.DraftTitle, p.Title) }
func (p *Post) CurrentSubtitle() string { return firstNonEmpty(p.DraftSubtitle, p.Subtitle) }

// CurrentBody returns the raw doc, draft first.
func (p *Post) CurrentBody() json.RawMessage {
	if s := strings.TrimSpace(string(p.DraftBody)); s != "" && s != "null" && s != `""` {
		return p.DraftBody
	}
	return p.Body
}

// DraftInput is the writable part of a draft. Empty strings are not sent on update.
type DraftInput struct {
	Title     string
	Subtitle  string
	Body      string // ProseMirror doc encoded as a JSON string
	Audience  string
	SectionID int64
}

// GetPost fetches one post (draft or published).
func (c *Client) GetPost(ctx context.Context, pub Publication, id int64) (*Post, error) {
	p := new(Post)
	err := c.do(ctx, request{Method: "GET", URL: c.pubAPI(pub) + "/drafts/" + strconv.FormatInt(id, 10)}, p)
	return p, err
}

// ListDrafts lists unpublished posts, newest first. It also returns how many
// rows it scanned, which is what the next offset must advance by.
func (c *Client) ListDrafts(ctx context.Context, pub Publication, limit, offset int) ([]Post, int, error) {
	q := url.Values{
		"limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)},
		"order_by": {"draft_updated_at"}, "order_direction": {"desc"},
	}
	posts, err := c.listPosts(ctx, c.pubAPI(pub)+"/post_management/drafts", q)
	if err != nil {
		return nil, 0, err
	}
	scanned := len(posts)
	// Defensive: the older /drafts endpoint mixed in published posts.
	out := posts[:0]
	for _, p := range posts {
		if p.Status() != "published" {
			out = append(out, p)
		}
	}
	return out, scanned, nil
}

// ListPublished lists published posts, newest first.
func (c *Client) ListPublished(ctx context.Context, pub Publication, limit, offset int) ([]Post, error) {
	q := url.Values{
		"limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)},
		"order_by": {"post_date"}, "order_direction": {"desc"},
	}
	return c.listPosts(ctx, c.pubAPI(pub)+"/post_management/published", q)
}

// ListScheduled lists posts scheduled to publish, soonest first.
func (c *Client) ListScheduled(ctx context.Context, pub Publication, limit, offset int) ([]Post, error) {
	q := url.Values{
		"limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)},
		"order_by": {"trigger_at"}, "order_direction": {"asc"},
	}
	return c.listPosts(ctx, c.pubAPI(pub)+"/post_management/scheduled", q)
}

// listPosts accepts both a bare array and {"posts": [...]}.
func (c *Client) listPosts(ctx context.Context, u string, q url.Values) ([]Post, error) {
	var raw json.RawMessage
	if err := c.do(ctx, request{Method: "GET", URL: u, Query: q}, &raw); err != nil {
		return nil, err
	}
	var posts []Post
	if json.Unmarshal(raw, &posts) == nil {
		return posts, nil
	}
	var wrapped struct {
		Posts []Post `json:"posts"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil, fmt.Errorf("unexpected post list: %w", err)
	}
	return wrapped.Posts, nil
}

// CreateDraft creates a draft bylined to the logged-in user.
func (c *Client) CreateDraft(ctx context.Context, pub Publication, in DraftInput) (*Post, error) {
	prof, err := c.Profile(ctx)
	if err != nil {
		return nil, err
	}
	audience := firstNonEmpty(in.Audience, "everyone")
	body := map[string]any{
		"draft_title":      in.Title,
		"draft_subtitle":   in.Subtitle,
		"draft_body":       in.Body,
		"draft_bylines":    []map[string]any{{"id": prof.ID, "is_guest": false}},
		"audience":         audience,
		"draft_section_id": nil,
		"section_chosen":   in.SectionID != 0,
	}
	if in.SectionID != 0 {
		body["draft_section_id"] = in.SectionID
	}
	p := new(Post)
	err = c.do(ctx, request{Method: "POST", URL: c.pubAPI(pub) + "/drafts", Body: body}, p)
	return p, err
}

// UpdateDraft changes the given fields of a draft. For a published post this
// edits its draft copy; publishing again pushes the change live.
func (c *Client) UpdateDraft(ctx context.Context, pub Publication, id int64, in DraftInput) (*Post, error) {
	body := map[string]any{}
	if in.Title != "" {
		body["draft_title"] = in.Title
	}
	if in.Subtitle != "" {
		body["draft_subtitle"] = in.Subtitle
	}
	if in.Body != "" {
		body["draft_body"] = in.Body
	}
	if in.Audience != "" {
		body["audience"] = in.Audience
	}
	if in.SectionID != 0 {
		body["draft_section_id"] = in.SectionID
		body["section_chosen"] = true
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("nothing to update: pass title, subtitle, content, audience or section_id")
	}
	p := new(Post)
	err := c.do(ctx, request{Method: "PUT", URL: c.pubAPI(pub) + "/drafts/" + strconv.FormatInt(id, 10), Body: body}, p)
	return p, err
}

// Publish publishes a draft now, emailing it to subscribers when sendEmail is set.
func (c *Client) Publish(ctx context.Context, pub Publication, id int64, sendEmail bool) (*Post, error) {
	body := map[string]any{"send": sendEmail}
	if !sendEmail {
		body["saved_segment_id"] = nil
	}
	p := new(Post)
	err := c.do(ctx, request{Method: "POST", URL: c.pubAPI(pub) + "/drafts/" + strconv.FormatInt(id, 10) + "/publish", Body: body}, p)
	return p, err
}

// Prepublish returns the problems the Substack editor would flag before publishing.
func (c *Client) Prepublish(ctx context.Context, pub Publication, id int64) (errs, suggestions []any, err error) {
	var resp struct {
		Errors      []any `json:"errors"`
		Suggestions []any `json:"suggestions"`
	}
	err = c.do(ctx, request{Method: "GET", URL: c.pubAPI(pub) + "/drafts/" + strconv.FormatInt(id, 10) + "/prepublish"}, &resp)
	return resp.Errors, resp.Suggestions, err
}

// Schedule publishes a draft at a later time, like the editor's "Schedule".
func (c *Client) Schedule(ctx context.Context, pub Publication, id int64, at time.Time, audience string, sendEmail bool) error {
	audience = firstNonEmpty(audience, "everyone")
	body := map[string]any{"trigger_at": at.UTC().Format(time.RFC3339), "post_audience": audience}
	if sendEmail {
		body["email_audience"] = audience
	} else {
		body["saved_segment_id"] = nil
	}
	return c.do(ctx, request{Method: "POST", URL: c.pubAPI(pub) + "/drafts/" + strconv.FormatInt(id, 10) + "/scheduled_release", Body: body}, nil)
}

// Unschedule turns a scheduled post back into a draft.
func (c *Client) Unschedule(ctx context.Context, pub Publication, id int64) error {
	return c.do(ctx, request{Method: "DELETE", URL: c.pubAPI(pub) + "/drafts/" + strconv.FormatInt(id, 10) + "/scheduled_release"}, nil)
}

// DeleteDraft deletes a draft.
func (c *Client) DeleteDraft(ctx context.Context, pub Publication, id int64) error {
	return c.do(ctx, request{Method: "DELETE", URL: c.pubAPI(pub) + "/drafts/" + strconv.FormatInt(id, 10)}, nil)
}

// EditURL is the author-only editor and preview link.
func EditURL(pub Publication, id int64) string {
	return pub.PublicURL() + "/publish/post/" + strconv.FormatInt(id, 10)
}

// PostURL is the public link of a published post.
func PostURL(pub Publication, p *Post) string {
	if p.CanonicalURL != "" {
		return p.CanonicalURL
	}
	if p.Slug == "" {
		return ""
	}
	return pub.PublicURL() + "/p/" + p.Slug
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

// Time decodes Substack timestamps, treating null, "" and odd formats as zero.
type Time struct{ time.Time }

func (t *Time) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) != nil || s == "" {
		t.Time = time.Time{}
		return nil
	}
	t.Time, _ = time.Parse(time.RFC3339Nano, s)
	return nil
}

func (t Time) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(t.Time.UTC().Format(time.RFC3339))
}
