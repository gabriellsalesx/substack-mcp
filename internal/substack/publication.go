package substack

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// Profile is the subset of /user/profile/self the server uses.
type Profile struct {
	ID                 int64             `json:"id"`
	Name               string            `json:"name"`
	Handle             string            `json:"handle"`
	Email              string            `json:"email"`
	PrimaryPublication *Publication      `json:"primaryPublication"`
	PublicationUsers   []PublicationUser `json:"publicationUsers"`
}

// PublicationUser links the user to a publication with a role.
type PublicationUser struct {
	Role        string      `json:"role"`
	IsPrimary   bool        `json:"is_primary"`
	Publication Publication `json:"publication"`
}

// Publication is a Substack newsletter the user belongs to.
type Publication struct {
	ID                   int64  `json:"id"`
	Name                 string `json:"name"`
	Subdomain            string `json:"subdomain"`
	CustomDomain         string `json:"custom_domain"`
	CustomDomainOptional bool   `json:"custom_domain_optional"`
	Role                 string `json:"-"`
}

// PublicURL is the address readers use.
func (p Publication) PublicURL() string {
	if p.CustomDomain != "" && !p.CustomDomainOptional {
		return "https://" + p.CustomDomain
	}
	return "https://" + p.Subdomain + ".substack.com"
}

// Profile returns the logged-in user, cached for the life of the client.
func (c *Client) Profile(ctx context.Context) (*Profile, error) {
	c.mu.Lock()
	p := c.profile
	c.mu.Unlock()
	if p != nil {
		return p, nil
	}
	p = new(Profile)
	if err := c.do(ctx, request{Method: "GET", URL: c.Root + "/user/profile/self"}, p); err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.profile = p
	c.mu.Unlock()
	return p, nil
}

// Publications lists the publications the user can work with, primary first.
func (c *Client) Publications(ctx context.Context) ([]Publication, error) {
	p, err := c.Profile(ctx)
	if err != nil {
		return nil, err
	}
	var out []Publication
	for _, pu := range p.PublicationUsers {
		pub := pu.Publication
		if pub.ID == 0 {
			continue
		}
		pub.Role = pu.Role
		if pu.IsPrimary || (p.PrimaryPublication != nil && pub.ID == p.PrimaryPublication.ID) {
			out = append([]Publication{pub}, out...)
		} else {
			out = append(out, pub)
		}
	}
	if len(out) == 0 && p.PrimaryPublication != nil {
		out = append(out, *p.PrimaryPublication)
	}
	return out, nil
}

// Publication resolves a selector (URL, host, custom domain or subdomain) to
// one of the user's publications. An empty selector picks the primary one.
func (c *Client) Publication(ctx context.Context, selector string) (Publication, error) {
	pubs, err := c.Publications(ctx)
	if err != nil {
		return Publication{}, err
	}
	if len(pubs) == 0 {
		return Publication{}, fmt.Errorf("this Substack account has no publication")
	}
	want := normalizeHost(selector)
	if want == "" {
		return pubs[0], nil
	}
	for _, p := range pubs {
		if want == strings.ToLower(p.Subdomain) || want == strings.ToLower(p.Subdomain)+".substack.com" ||
			(p.CustomDomain != "" && strings.TrimPrefix(want, "www.") == strings.TrimPrefix(strings.ToLower(p.CustomDomain), "www.")) {
			return p, nil
		}
	}
	names := make([]string, 0, len(pubs))
	for _, p := range pubs {
		names = append(names, p.PublicURL())
	}
	return Publication{}, fmt.Errorf("publication %q is not on this account; available: %s", selector, strings.Join(names, ", "))
}

// pubAPI is the API base for a publication. The *.substack.com host is used
// even for custom domains, because the session cookie belongs to substack.com.
func (c *Client) pubAPI(p Publication) string {
	if c.PubBase != "" {
		return c.PubBase
	}
	return "https://" + p.Subdomain + ".substack.com/api/v1"
}

func normalizeHost(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return s
	}
	return u.Hostname()
}

// Section is a publication section (Substack's "categories").
type Section struct {
	ID                  int64  `json:"id"`
	Name                string `json:"name"`
	Slug                string `json:"slug,omitempty"`
	Description         string `json:"description,omitempty"`
	SubscriberCount     int64  `json:"subscriber_count,omitempty"`
	FreeSubscriberCount int64  `json:"free_subscriber_count,omitempty"`
	PaidSubscriberCount int64  `json:"paid_subscriber_count,omitempty"`
}

// Sections returns the publication's sections.
func (c *Client) Sections(ctx context.Context, p Publication) ([]Section, error) {
	var out []Section
	err := c.do(ctx, request{Method: "GET", URL: c.pubAPI(p) + "/publication/sections"}, &out)
	return out, err
}

// Stats are the dashboard totals for the last 30 days.
type Stats struct {
	Subscribers     int64 `json:"subscribers"`
	PaidSubscribers int64 `json:"paid_subscribers"`
	Views30Days     int64 `json:"views_30_days"`
}

// Stats reads the publisher dashboard summary.
func (c *Client) Stats(ctx context.Context, p Publication) (Stats, error) {
	var resp struct {
		Total int64 `json:"totalSubscribersEnd"`
		Paid  int64 `json:"paidSubscribersEnd"`
		Views int64 `json:"totalViewsEnd"`
	}
	err := c.do(ctx, request{Method: "GET", URL: c.pubAPI(p) + "/publish-dashboard/summary-v2", Query: url.Values{"range": {"30"}}}, &resp)
	return Stats{resp.Total, resp.Paid, resp.Views}, err
}
