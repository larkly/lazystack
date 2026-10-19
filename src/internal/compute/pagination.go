package compute

import (
	"context"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/larkly/lazystack/internal/shared"
)

// maxListPages caps how many pages eachNovaPage follows, so a server that
// keeps returning a next link cannot keep a listing running forever.
const maxListPages = 100

// novaLinkedPage wraps one of gophercloud's single-page results and follows
// the rel=next entry in the body's linksKey array. Nova paginates keypairs
// (from microversion 2.35, "keypairs_links") and instance actions (from
// 2.58, "links"), but gophercloud's pagers stop after the first page.
type novaLinkedPage struct {
	pagination.Page // gophercloud's page, for IsEmpty and the Extract helpers
	linksKey        string
}

// NextPageURL implements pagination.Page.
func (p novaLinkedPage) NextPageURL() (string, error) {
	body, _ := p.GetBody().(map[string]any)
	links, _ := body[p.linksKey].([]any)
	for _, l := range links {
		link, _ := l.(map[string]any)
		if rel, _ := link["rel"].(string); rel != "next" {
			continue
		}
		href, _ := link["href"].(string)
		return href, nil
	}
	return "", nil
}

// eachNovaPage GETs url and every following page, up to maxListPages, and
// passes each gophercloud page (as built by wrap) to handle.
func eachNovaPage(ctx context.Context, client *gophercloud.ServiceClient, url, linksKey string,
	wrap func(pagination.PageResult) pagination.Page, handle func(pagination.Page) error) error {
	pages := 0
	pager := pagination.NewPager(client, url, func(r pagination.PageResult) pagination.Page {
		return novaLinkedPage{Page: wrap(r), linksKey: linksKey}
	})
	return pager.EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		if err := handle(page.(novaLinkedPage).Page); err != nil {
			return false, err
		}
		pages++
		if pages >= maxListPages {
			shared.Debugf("[compute] %s: stopped after %d pages", url, pages)
			return false, nil
		}
		return true, nil
	})
}
