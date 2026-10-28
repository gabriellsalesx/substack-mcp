package substack

import (
	"context"
	"strconv"
	"strings"
	"unicode"
)

// Tag is a post tag of a publication. Ids are UUIDs.
type Tag struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Slug   string `json:"slug"`
	Hidden bool   `json:"hidden"`
}

// Tags lists the publication's tags.
func (c *Client) Tags(ctx context.Context, pub Publication) ([]Tag, error) {
	var tags []Tag
	err := c.do(ctx, request{Method: "GET", URL: c.pubAPI(pub) + "/publication/post-tag"}, &tags)
	return tags, err
}

// CreateTag adds a tag to the publication.
func (c *Client) CreateTag(ctx context.Context, pub Publication, name string) (Tag, error) {
	var t Tag
	err := c.do(ctx, request{Method: "POST", URL: c.pubAPI(pub) + "/publication/post-tag", Body: map[string]any{"name": name}}, &t)
	return t, err
}

// PostTagIDs returns the ids of the tags on a post or draft.
func (c *Client) PostTagIDs(ctx context.Context, pub Publication, postID int64) ([]string, error) {
	var links []struct {
		TagID string `json:"post_tag_id"`
	}
	if err := c.do(ctx, request{Method: "GET", URL: c.postTagURL(pub, postID, "")}, &links); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(links))
	for _, l := range links {
		ids = append(ids, l.TagID)
	}
	return ids, nil
}

// AddPostTag puts a tag on a post or draft.
func (c *Client) AddPostTag(ctx context.Context, pub Publication, postID int64, tagID string) error {
	return c.do(ctx, request{Method: "POST", URL: c.postTagURL(pub, postID, tagID)}, nil)
}

// RemovePostTag takes a tag off a post or draft. The tag itself stays.
func (c *Client) RemovePostTag(ctx context.Context, pub Publication, postID int64, tagID string) error {
	return c.do(ctx, request{Method: "DELETE", URL: c.postTagURL(pub, postID, tagID)}, nil)
}

func (c *Client) postTagURL(pub Publication, postID int64, tagID string) string {
	u := c.pubAPI(pub) + "/post/" + strconv.FormatInt(postID, 10) + "/tag"
	if tagID != "" {
		u += "/" + tagID
	}
	return u
}

// TagKey folds a tag name for matching: case, accents, spaces and
// punctuation are ignored, so "IA e Prompts" matches "ia-e-prompts".
func TagKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if f, ok := accents[r]; ok {
			r = f
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var accents = map[rune]rune{
	'á': 'a', 'à': 'a', 'â': 'a', 'ã': 'a', 'ä': 'a',
	'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e',
	'í': 'i', 'ì': 'i', 'î': 'i', 'ï': 'i',
	'ó': 'o', 'ò': 'o', 'ô': 'o', 'õ': 'o', 'ö': 'o',
	'ú': 'u', 'ù': 'u', 'û': 'u', 'ü': 'u',
	'ç': 'c', 'ñ': 'n',
}
