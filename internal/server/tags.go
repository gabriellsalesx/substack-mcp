package server

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gabriellsalesx/substack-mcp/internal/substack"
)

// tagResult says what setTags did. When names have no matching tag and
// creating was not allowed, nothing changes and Missing, Available and Next
// tell the agent what to ask the user.
type tagResult struct {
	Tags      []string `json:"tags"`
	Created   []string `json:"created,omitempty"`
	Removed   []string `json:"removed,omitempty"`
	Missing   []string `json:"missing,omitempty"`
	Available []string `json:"available,omitempty"`
	Next      string   `json:"next,omitempty"`
}

// splitTags reads a comma-separated list, dropping blanks and repeats.
func splitTags(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, name := range strings.Split(s, ",") {
		name = strings.Join(strings.Fields(name), " ")
		k := substack.TagKey(name)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, name)
	}
	return out
}

// setTags makes names the post's tags. Each name reuses the publication tag
// it matches; tags not in names are taken off the post. A name with no match
// is created only when create is set: the tags belong to the user, so new
// ones need their consent.
func setTags(ctx context.Context, c *substack.Client, pub substack.Publication, postID int64, names []string, create bool) (tagResult, error) {
	res := tagResult{Tags: []string{}}
	all, err := c.Tags(ctx, pub)
	if err != nil {
		return res, fmt.Errorf("list tags: %w", err)
	}
	byKey := map[string]substack.Tag{}
	byID := map[string]substack.Tag{}
	for _, t := range all {
		byID[t.ID] = t
		for _, k := range []string{substack.TagKey(t.Name), substack.TagKey(t.Slug)} {
			if _, dup := byKey[k]; !dup && k != "" {
				byKey[k] = t
			}
		}
	}
	if !create {
		for _, name := range names {
			if _, ok := byKey[substack.TagKey(name)]; !ok {
				res.Missing = append(res.Missing, name)
			}
		}
		if len(res.Missing) > 0 {
			res.Available = []string{}
			for _, t := range all {
				res.Available = append(res.Available, t.Name)
			}
			res.Next = "No tag was changed. These tags do not exist in the publication yet. Ask the user whether to create them or to use existing tags from available that fit the post, then call update_draft with the chosen tags (and create_tags=true only for names they agreed to create)."
			return res, nil
		}
	}
	have, err := c.PostTagIDs(ctx, pub, postID)
	if err != nil {
		return res, fmt.Errorf("read the post's tags: %w", err)
	}

	var want []string
	for _, name := range names {
		t, ok := byKey[substack.TagKey(name)]
		if !ok {
			if t, err = c.CreateTag(ctx, pub, name); err != nil {
				return res, fmt.Errorf("create tag %q: %w", name, err)
			}
			byKey[substack.TagKey(name)], byID[t.ID] = t, t
			res.Created = append(res.Created, t.Name)
		}
		if !slices.Contains(want, t.ID) {
			want = append(want, t.ID)
		}
		res.Tags = append(res.Tags, t.Name)
	}
	for _, id := range want {
		if slices.Contains(have, id) {
			continue
		}
		if err := c.AddPostTag(ctx, pub, postID, id); err != nil {
			return res, fmt.Errorf("add tag %q: %w", byID[id].Name, err)
		}
	}
	for _, id := range have {
		if slices.Contains(want, id) {
			continue
		}
		if err := c.RemovePostTag(ctx, pub, postID, id); err != nil {
			return res, fmt.Errorf("remove tag %q: %w", byID[id].Name, err)
		}
		res.Removed = append(res.Removed, firstNonEmpty(byID[id].Name, id))
	}
	return res, nil
}

// postTags names the tags on a post.
func postTags(ctx context.Context, c *substack.Client, pub substack.Publication, postID int64) ([]string, error) {
	ids, err := c.PostTagIDs(ctx, pub, postID)
	if err != nil || len(ids) == 0 {
		return []string{}, err
	}
	all, err := c.Tags(ctx, pub)
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, t := range all {
		if slices.Contains(ids, t.ID) {
			names = append(names, t.Name)
		}
	}
	return names, nil
}

func (s *Server) listTags(ctx context.Context, _ *mcp.CallToolRequest, in pubIn) (*mcp.CallToolResult, any, error) {
	c, pub, err := s.resolve(ctx, in.Publication)
	if err != nil {
		return nil, nil, err
	}
	tags, err := c.Tags(ctx, pub)
	if err != nil {
		return nil, nil, err
	}
	type item struct {
		Name   string `json:"name"`
		Slug   string `json:"slug"`
		Hidden bool   `json:"hidden,omitempty"`
	}
	out := struct {
		Publication string `json:"publication"`
		Tags        []item `json:"tags"`
	}{Publication: pub.PublicURL(), Tags: []item{}}
	for _, t := range tags {
		out.Tags = append(out.Tags, item{t.Name, t.Slug, t.Hidden})
	}
	return nil, out, nil
}
