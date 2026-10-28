package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gabriellsalesx/substack-mcp/internal/prosemirror"
	"github.com/gabriellsalesx/substack-mcp/internal/substack"
)

// Input structs avoid pointers and slices: they turn into nullable or array
// JSON Schema types that Gemini and some other clients reject.

type listPostsIn struct {
	Status      string `json:"status,omitempty" jsonschema:"drafts (default), scheduled or published"`
	Limit       int    `json:"limit,omitempty" jsonschema:"How many posts to return, 1-25. Default 10."`
	Offset      int    `json:"offset,omitempty" jsonschema:"How many posts to skip, for paging. Default 0."`
	Publication string `json:"publication,omitempty" jsonschema:"Publication URL, host or subdomain. Optional; defaults to the configured or primary publication."`
}

type postIn struct {
	PostID      string `json:"post_id" jsonschema:"Post id, or the editor/post URL that contains it."`
	Publication string `json:"publication,omitempty" jsonschema:"Publication URL, host or subdomain. Optional; defaults to the configured or primary publication."`
}

type createDraftIn struct {
	Title       string `json:"title" jsonschema:"Post title, at most 280 characters."`
	Content     string `json:"content" jsonschema:"Post body in Markdown. Supports headings, bold, italic, strikethrough, links, lists, quotes, code blocks, horizontal rules and images; local image paths are uploaded. A line with <!-- paywall --> marks where the paywall starts; <!-- subscribe --> inserts the subscribe button."`
	Subtitle    string `json:"subtitle,omitempty" jsonschema:"Optional subtitle, at most 280 characters."`
	Audience    string `json:"audience,omitempty" jsonschema:"everyone (default), only_paid, founding or only_free. A paywall marker sets only_paid when this is empty."`
	SectionID   int64  `json:"section_id,omitempty" jsonschema:"Optional section id from get_publication."`
	Tags        string `json:"tags,omitempty" jsonschema:"Comma-separated tag names. Pick them from list_tags; a name matches an existing tag ignoring case, accents and punctuation. If any name has no match, no tag is applied and the answer lists the missing ones to ask the user about."`
	CreateTags  bool   `json:"create_tags,omitempty" jsonschema:"true creates the tags that do not exist yet. Only set it after the user agreed to create those new tags."`
	Publication string `json:"publication,omitempty" jsonschema:"Publication URL, host or subdomain. Optional; defaults to the configured or primary publication."`
}

type updateDraftIn struct {
	PostID      string `json:"post_id" jsonschema:"Post id, or the editor/post URL that contains it."`
	Title       string `json:"title,omitempty" jsonschema:"New title. Omit to keep."`
	Subtitle    string `json:"subtitle,omitempty" jsonschema:"New subtitle. Omit to keep."`
	Content     string `json:"content,omitempty" jsonschema:"New body in Markdown; replaces the whole body. Omit to keep."`
	Audience    string `json:"audience,omitempty" jsonschema:"everyone, only_paid, founding or only_free. Omit to keep."`
	SectionID   int64  `json:"section_id,omitempty" jsonschema:"Section id from get_publication. Omit to keep."`
	Tags        string `json:"tags,omitempty" jsonschema:"Comma-separated tag names; replaces the post's tags. Pick them from list_tags; a name matches an existing tag ignoring case, accents and punctuation. If any name has no match, the tags are left as they were and the answer lists the missing ones to ask the user about. Omit to keep."`
	CreateTags  bool   `json:"create_tags,omitempty" jsonschema:"true creates the tags that do not exist yet. Only set it after the user agreed to create those new tags."`
	ClearTags   bool   `json:"clear_tags,omitempty" jsonschema:"true removes every tag from the post. The tags stay in the publication."`
	Publication string `json:"publication,omitempty" jsonschema:"Publication URL, host or subdomain. Optional; defaults to the configured or primary publication."`
}

type duplicateIn struct {
	PostID      string `json:"post_id" jsonschema:"Id or URL of the post to copy."`
	NewTitle    string `json:"new_title,omitempty" jsonschema:"Title of the copy. Default: the original title with (copy)."`
	Publication string `json:"publication,omitempty" jsonschema:"Publication URL, host or subdomain. Optional; defaults to the configured or primary publication."`
}

type publishIn struct {
	PostID       string `json:"post_id" jsonschema:"Id or URL of the draft to publish."`
	SendEmail    bool   `json:"send_email,omitempty" jsonschema:"Also email the post to subscribers. Default false."`
	ScheduledFor string `json:"scheduled_for,omitempty" jsonschema:"Schedule instead of publishing now. RFC 3339, e.g. 2026-11-03T09:00:00-03:00."`
	Confirm      bool   `json:"confirm,omitempty" jsonschema:"false returns a preview; true publishes. Only set true after the user approved the preview."`
	Publication  string `json:"publication,omitempty" jsonschema:"Publication URL, host or subdomain. Optional; defaults to the configured or primary publication."`
}

type deleteDraftIn struct {
	PostID      string `json:"post_id" jsonschema:"Id or URL of the draft to delete."`
	Confirm     bool   `json:"confirm,omitempty" jsonschema:"false returns a preview; true deletes. Only set true after the user agreed."`
	Publication string `json:"publication,omitempty" jsonschema:"Publication URL, host or subdomain. Optional; defaults to the configured or primary publication."`
}

type uploadImageIn struct {
	Path        string `json:"path" jsonschema:"Absolute path of a JPEG, PNG, GIF or WebP file, or an image URL to re-host."`
	Publication string `json:"publication,omitempty" jsonschema:"Publication URL, host or subdomain. Optional; defaults to the configured or primary publication."`
}

type pubIn struct {
	Publication string `json:"publication,omitempty" jsonschema:"Publication URL, host or subdomain. Optional; defaults to the configured or primary publication."`
}

type createNoteIn struct {
	Content      string `json:"content" jsonschema:"Note text. Blank lines separate paragraphs."`
	LinkURL      string `json:"link_url,omitempty" jsonschema:"Optional URL to attach as a link card."`
	ScheduledFor string `json:"scheduled_for,omitempty" jsonschema:"Schedule instead of posting now. RFC 3339, e.g. 2026-11-03T09:00:00-03:00."`
	Confirm      bool   `json:"confirm,omitempty" jsonschema:"false returns a preview; true posts. Only set true after the user approved the preview."`
	Publication  string `json:"publication,omitempty" jsonschema:"Publication URL, host or subdomain. Optional; defaults to the configured or primary publication."`
}

type listNotesIn struct {
	Limit  int    `json:"limit,omitempty" jsonschema:"How many scheduled notes to return, 1-50. Default 20."`
	Cursor string `json:"cursor,omitempty" jsonschema:"next_cursor from a previous call."`
}

type deleteNoteIn struct {
	NoteID  string `json:"note_id" jsonschema:"Id of the note or scheduled note."`
	Confirm bool   `json:"confirm,omitempty" jsonschema:"false returns a preview; true deletes. Only set true after the user agreed."`
}

type communityIn struct {
	Content     string `json:"content" jsonschema:"Message text."`
	Audience    string `json:"audience,omitempty" jsonschema:"Who can see it. Default all_subscribers."`
	LinkURL     string `json:"link_url,omitempty" jsonschema:"Optional URL to share."`
	SendEmail   bool   `json:"send_email,omitempty" jsonschema:"Email the message to subscribers. Default false."`
	SendPush    bool   `json:"send_push,omitempty" jsonschema:"Send a push notification. Default false."`
	Confirm     bool   `json:"confirm,omitempty" jsonschema:"false returns a preview; true posts. Only set true after the user approved the preview."`
	Publication string `json:"publication,omitempty" jsonschema:"Publication URL, host or subdomain. Optional; defaults to the configured or primary publication."`
}

// Outputs.

type postSummary struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
	Status   string `json:"status"`
	Audience string `json:"audience,omitempty"`
	PostDate string `json:"post_date,omitempty"`
	URL      string `json:"url,omitempty"`
	EditURL  string `json:"edit_url"`
}

type preview struct {
	ConfirmationRequired bool   `json:"confirmation_required"`
	Action               string `json:"action"`
	Details              any    `json:"details,omitempty"`
	Next                 string `json:"next"`
}

func confirmPreview(action string, details any) preview {
	return preview{
		ConfirmationRequired: true,
		Action:               action,
		Details:              details,
		Next:                 "Nothing was changed. Show this to the user and call the tool again with confirm=true only if they approve.",
	}
}

func summarize(pub substack.Publication, p *substack.Post) postSummary {
	s := postSummary{
		ID:       p.ID,
		Title:    p.CurrentTitle(),
		Subtitle: p.CurrentSubtitle(),
		Status:   p.Status(),
		Audience: p.Audience,
		EditURL:  substack.EditURL(pub, p.ID),
	}
	if at := p.ScheduledFor(); !at.IsZero() {
		s.PostDate = at.Format(time.RFC3339)
	} else if !p.PostDate.IsZero() {
		s.PostDate = p.PostDate.Format(time.RFC3339)
	}
	if s.Status == "published" {
		s.URL = substack.PostURL(pub, p)
	}
	return s
}

func ann(title string, readOnly, destructive, idempotent bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		Title:           title,
		ReadOnlyHint:    readOnly,
		DestructiveHint: &destructive,
		IdempotentHint:  idempotent,
		OpenWorldHint:   new(true),
	}
}

func (s *Server) register(ms *mcp.Server) {
	mcp.AddTool(ms, &mcp.Tool{
		Name:        "list_posts",
		Description: "List drafts, scheduled or published posts of a Substack publication, with ids and links.",
		Annotations: ann("List posts", true, false, true),
	}, s.listPosts)

	mcp.AddTool(ms, &mcp.Tool{
		Name:        "get_post",
		Description: "Read one post or draft: title, subtitle, status, audience, tags, links and the full body as Markdown.",
		Annotations: ann("Read post", true, false, true),
	}, s.getPost)

	mcp.AddTool(ms, &mcp.Tool{
		Name:        "create_draft",
		Description: "Create a private draft from Markdown. Local image paths in the Markdown are uploaded to Substack. Tags must already exist in the publication unless create_tags is set with the user's consent. Returns the draft id and an editor link where the user can preview it. Does not publish.",
		Annotations: ann("Create draft", false, false, false),
	}, s.createDraft)

	mcp.AddTool(ms, &mcp.Tool{
		Name:        "update_draft",
		Description: "Change the title, subtitle, body, audience, section or tags of a draft. Only the fields you pass change; content replaces the whole body. For an already published post this edits its pending version; publish_post makes the edit live.",
		Annotations: ann("Update draft", false, true, true),
	}, s.updateDraft)

	mcp.AddTool(ms, &mcp.Tool{
		Name:        "duplicate_post",
		Description: "Copy a post or draft, formatting and tags included, into a new private draft.",
		Annotations: ann("Duplicate post", false, false, false),
	}, s.duplicatePost)

	mcp.AddTool(ms, &mcp.Tool{
		Name:        "publish_post",
		Description: "Publish a draft now or schedule it. On an already published post, pushes its pending edits live, and with send_email=true emails it to subscribers. Public, so call without confirm first, show the preview to the user, and repeat with confirm=true once they approve.",
		Annotations: ann("Publish post", false, true, false),
	}, s.publishPost)

	mcp.AddTool(ms, &mcp.Tool{
		Name:        "delete_draft",
		Description: "Delete a draft, or cancel a scheduled post and delete it. Live posts are refused. Call without confirm first, then with confirm=true once the user agrees.",
		Annotations: ann("Delete draft", false, true, true),
	}, s.deleteDraft)

	mcp.AddTool(ms, &mcp.Tool{
		Name:        "upload_image",
		Description: "Upload an image to Substack's CDN and get its URL plus a Markdown snippet. Not needed for create_draft, which uploads images by itself.",
		Annotations: ann("Upload image", false, false, false),
	}, s.uploadImage)

	mcp.AddTool(ms, &mcp.Tool{
		Name:        "get_publication",
		Description: "Show the publication (name, URL, role, subscriber count, sections with ids) and the other publications on this account.",
		Annotations: ann("Publication info", true, false, true),
	}, s.getPublication)

	mcp.AddTool(ms, &mcp.Tool{
		Name:        "list_tags",
		Description: "List the publication's post tags. Call it before tagging a post and choose the existing tags that fit it; propose a new tag only when none fits, and create it only if the user agrees.",
		Annotations: ann("List tags", true, false, true),
	}, s.listTags)

	mcp.AddTool(ms, &mcp.Tool{
		Name:        "create_note",
		Description: "Post a Substack Note (short post on the Notes feed) now or schedule it. Public: call without confirm first, then with confirm=true once the user approves.",
		Annotations: ann("Create note", false, false, false),
	}, s.createNote)

	mcp.AddTool(ms, &mcp.Tool{
		Name:        "list_scheduled_notes",
		Description: "List scheduled Notes that have not been posted yet.",
		Annotations: ann("List scheduled notes", true, false, true),
	}, s.listNotes)

	mcp.AddTool(ms, &mcp.Tool{
		Name:        "delete_note",
		Description: "Cancel a scheduled Note or delete a posted one. Call without confirm first, then with confirm=true once the user agrees.",
		Annotations: ann("Delete note", false, true, true),
	}, s.deleteNote)

	mcp.AddTool(ms, &mcp.Tool{
		Name:        "create_community_post",
		Description: "Start a thread in the publication's subscriber chat. Visible to subscribers: call without confirm first, then with confirm=true once the user approves.",
		Annotations: ann("Post to subscriber chat", false, false, false),
	}, s.createCommunityPost)
}

func (s *Server) listPosts(ctx context.Context, _ *mcp.CallToolRequest, in listPostsIn) (*mcp.CallToolResult, any, error) {
	c, pub, err := s.resolve(ctx, in.Publication)
	if err != nil {
		return nil, nil, err
	}
	limit := clamp(in.Limit, 10, 1, 25)
	offset := max(in.Offset, 0)
	var posts []substack.Post
	scanned := 0
	switch strings.ToLower(firstNonEmpty(in.Status, "drafts")) {
	case "drafts", "draft":
		posts, scanned, err = c.ListDrafts(ctx, pub, limit, offset)
	case "scheduled":
		posts, err = c.ListScheduled(ctx, pub, limit, offset)
		scanned = len(posts)
	case "published":
		posts, err = c.ListPublished(ctx, pub, limit, offset)
		scanned = len(posts)
	default:
		return nil, nil, fmt.Errorf("status %q is invalid; use drafts, scheduled or published", in.Status)
	}
	if err != nil {
		return nil, nil, err
	}
	out := struct {
		Publication string        `json:"publication"`
		Posts       []postSummary `json:"posts"`
		NextOffset  int           `json:"next_offset,omitempty"`
	}{Publication: pub.PublicURL(), Posts: []postSummary{}}
	for i := range posts {
		sum := summarize(pub, &posts[i])
		if in.Status == "scheduled" {
			// This listing omits postSchedules, so the status cannot be derived.
			sum.Status = "scheduled"
		}
		out.Posts = append(out.Posts, sum)
	}
	if scanned >= limit {
		out.NextOffset = offset + scanned
	}
	return nil, out, nil
}

func (s *Server) getPost(ctx context.Context, _ *mcp.CallToolRequest, in postIn) (*mcp.CallToolResult, any, error) {
	id, err := parseID(in.PostID, "post_id")
	if err != nil {
		return nil, nil, err
	}
	c, pub, err := s.resolve(ctx, in.Publication)
	if err != nil {
		return nil, nil, err
	}
	p, err := c.GetPost(ctx, pub, id)
	if err != nil {
		return nil, nil, err
	}
	doc, err := prosemirror.Parse(p.CurrentBody())
	if err != nil {
		return nil, nil, fmt.Errorf("post %d has a body this server cannot read: %w", id, err)
	}
	tags, err := postTags(ctx, c, pub, id)
	if err != nil {
		return nil, nil, fmt.Errorf("read the tags of post %d: %w", id, err)
	}
	md, cut := truncate(prosemirror.ToMarkdown(doc), 40000)
	out := struct {
		postSummary
		Tags      []string `json:"tags"`
		WordCount int      `json:"word_count"`
		Markdown  string   `json:"markdown"`
		Truncated bool     `json:"truncated,omitempty"`
	}{summarize(pub, p), tags, len(strings.Fields(prosemirror.PlainText(doc))), md, cut}
	return nil, out, nil
}

// subscribeTemplate copies the subscribe button from the newest published
// post that has one, so new posts get the same wording and language.
func subscribeTemplate(ctx context.Context, c *substack.Client, pub substack.Publication) *prosemirror.Node {
	posts, err := c.ListPublished(ctx, pub, 5, 0)
	if err != nil {
		return nil
	}
	for _, p := range posts {
		full, err := c.GetPost(ctx, pub, p.ID)
		if err != nil {
			continue
		}
		if doc, err := prosemirror.Parse(full.CurrentBody()); err == nil {
			if w := doc.Find("subscribeWidget"); w != nil {
				return w
			}
		}
	}
	return nil
}

// buildBody converts Markdown, uploading images to the publication.
func buildBody(ctx context.Context, c *substack.Client, pub substack.Publication, md, title string) (prosemirror.Result, string, error) {
	var sub *prosemirror.Node
	if prosemirror.HasSubscribe(md) {
		sub = subscribeTemplate(ctx, c, pub)
	}
	res, err := prosemirror.FromMarkdown(ctx, md, prosemirror.Options{
		DropTitle: title,
		Subscribe: sub,
		Images: func(ctx context.Context, src string) (prosemirror.Image, error) {
			img, err := c.UploadImage(ctx, pub, src)
			if err != nil {
				return prosemirror.Image{}, err
			}
			return prosemirror.Image{URL: img.URL, Width: img.Width, Height: img.Height, Bytes: img.Bytes, ContentType: img.ContentType}, nil
		},
	})
	if err != nil {
		return res, "", err
	}
	if len(res.Doc.Content) == 0 {
		return res, "", fmt.Errorf("the content has nothing Substack can show; raw HTML is not supported, use Markdown")
	}
	body, err := res.Doc.String()
	return res, body, err
}

func checkLen(field, v string) error {
	if n := len([]rune(v)); n > 280 {
		return fmt.Errorf("%s has %d characters; Substack allows 280", field, n)
	}
	return nil
}

func (s *Server) createDraft(ctx context.Context, _ *mcp.CallToolRequest, in createDraftIn) (*mcp.CallToolResult, any, error) {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || strings.TrimSpace(in.Content) == "" {
		return nil, nil, fmt.Errorf("title and content are required")
	}
	for _, err := range []error{checkLen("title", in.Title), checkLen("subtitle", in.Subtitle), checkAudience(in.Audience)} {
		if err != nil {
			return nil, nil, err
		}
	}
	c, pub, err := s.resolve(ctx, in.Publication)
	if err != nil {
		return nil, nil, err
	}
	res, body, err := buildBody(ctx, c, pub, in.Content, in.Title)
	if err != nil {
		return nil, nil, err
	}
	audience := in.Audience
	if audience == "" && res.HasPaywall {
		audience = "only_paid"
	}
	p, err := c.CreateDraft(ctx, pub, substack.DraftInput{Title: in.Title, Subtitle: in.Subtitle, Body: body, Audience: audience, SectionID: in.SectionID})
	if err != nil {
		return nil, nil, err
	}
	out := struct {
		postSummary
		ImagesUploaded int        `json:"images_uploaded"`
		Paywall        bool       `json:"paywall"`
		Tags           *tagResult `json:"tags,omitempty"`
		TagsError      string     `json:"tags_error,omitempty"`
	}{postSummary: summarize(pub, p), ImagesUploaded: res.Images, Paywall: res.HasPaywall}
	out.Title, out.Status = in.Title, "draft"
	if names := splitTags(in.Tags); len(names) > 0 {
		// The draft exists by now, so a tag failure is reported, not returned.
		tr, err := setTags(ctx, c, pub, p.ID, names, in.CreateTags)
		if err != nil {
			out.TagsError = "The draft was created, but tagging failed: " + err.Error() + ". Retry with update_draft."
		} else {
			out.Tags = &tr
		}
	}
	return nil, out, nil
}

func (s *Server) updateDraft(ctx context.Context, _ *mcp.CallToolRequest, in updateDraftIn) (*mcp.CallToolResult, any, error) {
	id, err := parseID(in.PostID, "post_id")
	if err != nil {
		return nil, nil, err
	}
	for _, err := range []error{checkLen("title", in.Title), checkLen("subtitle", in.Subtitle), checkAudience(in.Audience)} {
		if err != nil {
			return nil, nil, err
		}
	}
	c, pub, err := s.resolve(ctx, in.Publication)
	if err != nil {
		return nil, nil, err
	}
	cur, err := c.GetPost(ctx, pub, id)
	if err != nil {
		return nil, nil, err
	}
	names := splitTags(in.Tags)
	tagging := len(names) > 0 || in.ClearTags
	if len(names) > 0 && in.ClearTags {
		return nil, nil, fmt.Errorf("pass tags or clear_tags, not both")
	}
	upd := substack.DraftInput{Title: strings.TrimSpace(in.Title), Subtitle: in.Subtitle, Audience: in.Audience, SectionID: in.SectionID}
	images := 0
	if strings.TrimSpace(in.Content) != "" {
		res, body, err := buildBody(ctx, c, pub, in.Content, firstNonEmpty(upd.Title, cur.CurrentTitle()))
		if err != nil {
			return nil, nil, err
		}
		upd.Body, images = body, res.Images
	}
	p := cur
	if upd != (substack.DraftInput{}) || !tagging {
		if p, err = c.UpdateDraft(ctx, pub, id, upd); err != nil {
			return nil, nil, err
		}
		if p.ID == 0 {
			p = cur
		}
	}
	out := struct {
		postSummary
		ImagesUploaded int        `json:"images_uploaded"`
		Tags           *tagResult `json:"tags,omitempty"`
		Note           string     `json:"note,omitempty"`
	}{postSummary: summarize(pub, p), ImagesUploaded: images}
	if tagging {
		tr, err := setTags(ctx, c, pub, id, names, in.CreateTags)
		if err != nil {
			return nil, nil, err
		}
		out.Tags = &tr
	}
	if cur.Status() == "published" {
		out.Note = "This post is live. The edit is saved as pending; call publish_post to make it public (send_email=true also emails it to subscribers)."
	}
	return nil, out, nil
}

func (s *Server) duplicatePost(ctx context.Context, _ *mcp.CallToolRequest, in duplicateIn) (*mcp.CallToolResult, any, error) {
	id, err := parseID(in.PostID, "post_id")
	if err != nil {
		return nil, nil, err
	}
	if err := checkLen("new_title", in.NewTitle); err != nil {
		return nil, nil, err
	}
	c, pub, err := s.resolve(ctx, in.Publication)
	if err != nil {
		return nil, nil, err
	}
	src, err := c.GetPost(ctx, pub, id)
	if err != nil {
		return nil, nil, err
	}
	doc, err := prosemirror.Parse(src.CurrentBody())
	if err != nil {
		return nil, nil, err
	}
	body, err := doc.String()
	if err != nil {
		return nil, nil, err
	}
	title := firstNonEmpty(strings.TrimSpace(in.NewTitle), src.CurrentTitle()+" (copy)")
	p, err := c.CreateDraft(ctx, pub, substack.DraftInput{Title: title, Subtitle: src.CurrentSubtitle(), Body: body, Audience: src.Audience})
	if err != nil {
		return nil, nil, err
	}
	out := struct {
		postSummary
		Tags      []string `json:"tags,omitempty"`
		TagsError string   `json:"tags_error,omitempty"`
	}{postSummary: summarize(pub, p)}
	out.Title, out.Status = title, "draft"
	if tags, err := postTags(ctx, c, pub, id); err != nil {
		out.TagsError = "The copy was created without tags: " + err.Error()
	} else if len(tags) > 0 {
		if tr, err := setTags(ctx, c, pub, p.ID, tags, false); err != nil {
			out.TagsError = "The copy was created, but copying the tags failed: " + err.Error()
		} else {
			out.Tags = tr.Tags
		}
	}
	return nil, out, nil
}

func (s *Server) publishPost(ctx context.Context, _ *mcp.CallToolRequest, in publishIn) (*mcp.CallToolResult, any, error) {
	id, err := parseID(in.PostID, "post_id")
	if err != nil {
		return nil, nil, err
	}
	var at time.Time
	if in.ScheduledFor != "" {
		if at, err = parseTime(in.ScheduledFor); err != nil {
			return nil, nil, err
		}
	}
	c, pub, err := s.resolve(ctx, in.Publication)
	if err != nil {
		return nil, nil, err
	}
	p, err := c.GetPost(ctx, pub, id)
	if err != nil {
		return nil, nil, err
	}
	live := p.Status() == "published"
	if live && !at.IsZero() {
		return nil, nil, fmt.Errorf("post %d is already published; it cannot be scheduled", id)
	}
	if !in.Confirm {
		d := map[string]any{
			"post":        summarize(pub, p),
			"publication": pub.PublicURL(),
			"send_email":  in.SendEmail,
		}
		if errs, sugg, err := c.Prepublish(ctx, pub, id); err == nil {
			if len(errs) > 0 {
				d["substack_errors"] = errs
			}
			if len(sugg) > 0 {
				d["substack_suggestions"] = sugg
			}
		}
		action := "Publish now"
		switch {
		case !at.IsZero():
			action = "Schedule for " + at.Format(time.RFC3339)
		case live && in.SendEmail:
			action = "Update the live post with its pending edits and email it to subscribers"
		case live:
			action = "Update the live post with its pending edits (no email)"
		}
		if in.SendEmail {
			if st, err := c.Stats(ctx, pub); err == nil {
				d["subscribers_emailed"] = st.Subscribers
			}
		}
		return nil, confirmPreview(fmt.Sprintf("%s: %q on %s", action, p.CurrentTitle(), pub.PublicURL()), d), nil
	}
	if !at.IsZero() {
		if err := c.Schedule(ctx, pub, id, at, p.Audience, in.SendEmail); err != nil {
			return nil, nil, err
		}
		out := summarize(pub, p)
		out.Status, out.PostDate = "scheduled", at.Format(time.RFC3339)
		return nil, out, nil
	}
	// A live post is updated without email first; the email, if asked, is a
	// separate "send to subscribers" so it never goes out twice.
	pp, err := c.Publish(ctx, pub, id, in.SendEmail && !live)
	if err != nil {
		return nil, nil, err
	}
	if live && in.SendEmail {
		if err := c.SendPublished(ctx, pub, id); err != nil {
			return nil, nil, fmt.Errorf("post updated, but sending the email failed: %w", err)
		}
	}
	if pp.ID == 0 {
		pp = p
	}
	out := summarize(pub, pp)
	out.Status = "published"
	out.URL = substack.PostURL(pub, pp)
	return nil, out, nil
}

func (s *Server) deleteDraft(ctx context.Context, _ *mcp.CallToolRequest, in deleteDraftIn) (*mcp.CallToolResult, any, error) {
	id, err := parseID(in.PostID, "post_id")
	if err != nil {
		return nil, nil, err
	}
	c, pub, err := s.resolve(ctx, in.Publication)
	if err != nil {
		return nil, nil, err
	}
	p, err := c.GetPost(ctx, pub, id)
	if err != nil {
		return nil, nil, err
	}
	if p.Status() == "published" {
		return nil, nil, fmt.Errorf("post %d is published; only drafts and scheduled posts can be deleted here, use the Substack dashboard for live posts", id)
	}
	if !in.Confirm {
		action := fmt.Sprintf("Delete draft %q permanently", p.CurrentTitle())
		if p.Status() == "scheduled" {
			action = fmt.Sprintf("Cancel the scheduled publication of %q and delete it permanently", p.CurrentTitle())
		}
		return nil, confirmPreview(action, summarize(pub, p)), nil
	}
	if p.Status() == "scheduled" {
		if err := c.Unschedule(ctx, pub, id); err != nil {
			return nil, nil, fmt.Errorf("cancel schedule: %w", err)
		}
	}
	if err := c.DeleteDraft(ctx, pub, id); err != nil {
		return nil, nil, err
	}
	return nil, map[string]any{"deleted": true, "id": id, "title": p.CurrentTitle()}, nil
}

func (s *Server) uploadImage(ctx context.Context, _ *mcp.CallToolRequest, in uploadImageIn) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.Path) == "" {
		return nil, nil, fmt.Errorf("path is required")
	}
	c, pub, err := s.resolve(ctx, in.Publication)
	if err != nil {
		return nil, nil, err
	}
	img, err := c.UploadImage(ctx, pub, strings.TrimSpace(in.Path))
	if err != nil {
		return nil, nil, err
	}
	return nil, map[string]any{
		"url": img.URL, "width": img.Width, "height": img.Height, "bytes": img.Bytes,
		"markdown": "![](" + img.URL + ")",
	}, nil
}

func (s *Server) getPublication(ctx context.Context, _ *mcp.CallToolRequest, in pubIn) (*mcp.CallToolResult, any, error) {
	c, pub, err := s.resolve(ctx, in.Publication)
	if err != nil {
		return nil, nil, err
	}
	prof, err := c.Profile(ctx)
	if err != nil {
		return nil, nil, err
	}
	sections, err := c.Sections(ctx, pub)
	if err != nil {
		return nil, nil, err
	}
	pubs, _ := c.Publications(ctx)
	others := []string{}
	for _, p := range pubs {
		if p.ID != pub.ID {
			others = append(others, p.PublicURL())
		}
	}
	out := map[string]any{
		"id":                 pub.ID,
		"name":               pub.Name,
		"url":                pub.PublicURL(),
		"subdomain":          pub.Subdomain,
		"role":               pub.Role,
		"sections":           sections,
		"other_publications": others,
		"user":               map[string]any{"id": prof.ID, "name": prof.Name, "handle": prof.Handle},
	}
	if out["sections"] == nil {
		out["sections"] = []substack.Section{}
	}
	if st, err := c.Stats(ctx, pub); err == nil {
		out["stats"] = st
	}
	return nil, out, nil
}

func (s *Server) createNote(ctx context.Context, _ *mcp.CallToolRequest, in createNoteIn) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.Content) == "" {
		return nil, nil, fmt.Errorf("content is required")
	}
	var at time.Time
	var err error
	if in.ScheduledFor != "" {
		if at, err = parseTime(in.ScheduledFor); err != nil {
			return nil, nil, err
		}
	}
	if !in.Confirm {
		action := "Post a public Note now"
		if !at.IsZero() {
			action = "Schedule a public Note for " + at.Format(time.RFC3339)
		}
		return nil, confirmPreview(action, map[string]any{"content": in.Content, "link_url": in.LinkURL}), nil
	}
	c, pub, err := s.resolve(ctx, in.Publication)
	if err != nil {
		return nil, nil, err
	}
	n, err := c.CreateNote(ctx, pub, substack.NoteInput{BodyJSON: prosemirror.NoteDoc(in.Content), LinkURL: in.LinkURL, At: at})
	if err != nil {
		return nil, nil, err
	}
	out := map[string]any{"id": n.ID, "status": firstNonEmpty(n.Status, "published")}
	if !at.IsZero() {
		out["status"] = "scheduled"
		out["scheduled_for"] = at.Format(time.RFC3339)
	} else if prof, err := c.Profile(ctx); err == nil && prof.Handle != "" && n.ID != "" {
		out["url"] = "https://substack.com/@" + prof.Handle + "/note/c-" + string(n.ID)
	}
	return nil, out, nil
}

func (s *Server) listNotes(ctx context.Context, _ *mcp.CallToolRequest, in listNotesIn) (*mcp.CallToolResult, any, error) {
	c, _, err := s.session()
	if err != nil {
		return nil, nil, err
	}
	notes, next, err := c.ListNoteDrafts(ctx, clamp(in.Limit, 20, 1, 50), in.Cursor)
	if err != nil {
		return nil, nil, err
	}
	type item struct {
		ID           substack.ID `json:"id"`
		Status       string      `json:"status,omitempty"`
		ScheduledFor string      `json:"scheduled_for,omitempty"`
		Text         string      `json:"text"`
	}
	out := struct {
		Notes      []item `json:"notes"`
		NextCursor string `json:"next_cursor,omitempty"`
	}{Notes: []item{}, NextCursor: next}
	for _, n := range notes {
		it := item{ID: n.ID, Status: n.Status, Text: n.Body}
		if !n.TriggerAt.IsZero() {
			it.ScheduledFor = n.TriggerAt.Format(time.RFC3339)
		}
		out.Notes = append(out.Notes, it)
	}
	return nil, out, nil
}

func (s *Server) deleteNote(ctx context.Context, _ *mcp.CallToolRequest, in deleteNoteIn) (*mcp.CallToolResult, any, error) {
	id, err := parseID(in.NoteID, "note_id")
	if err != nil {
		return nil, nil, err
	}
	if !in.Confirm {
		return nil, confirmPreview(fmt.Sprintf("Delete note %d permanently", id), nil), nil
	}
	c, _, err := s.session()
	if err != nil {
		return nil, nil, err
	}
	if err := c.DeleteNote(ctx, fmt.Sprint(id)); err != nil {
		return nil, nil, err
	}
	return nil, map[string]any{"deleted": true, "id": id}, nil
}

func (s *Server) createCommunityPost(ctx context.Context, _ *mcp.CallToolRequest, in communityIn) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.Content) == "" {
		return nil, nil, fmt.Errorf("content is required")
	}
	c, pub, err := s.resolve(ctx, in.Publication)
	if err != nil {
		return nil, nil, err
	}
	if !in.Confirm {
		return nil, confirmPreview("Post to the subscriber chat of "+pub.PublicURL(), map[string]any{
			"content": in.Content, "audience": firstNonEmpty(in.Audience, "all_subscribers"),
			"link_url": in.LinkURL, "send_email": in.SendEmail, "send_push": in.SendPush,
		}), nil
	}
	p, err := c.CreateCommunityPost(ctx, pub, substack.CommunityInput{
		Body: in.Content, Audience: in.Audience, LinkURL: in.LinkURL, SendEmail: in.SendEmail, SendPush: in.SendPush,
	})
	if err != nil {
		return nil, nil, err
	}
	return nil, p, nil
}
