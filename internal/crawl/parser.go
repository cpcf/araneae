package crawl

import (
	"io"
	"strings"

	"golang.org/x/net/html"
)

type LinkOccurrence struct {
	SourceURL string
	LinkURL   string
	FetchURL  string
	Fragment  string
	Text      string
}

type ParseResult struct {
	Links       []LinkOccurrence
	FragmentIDs map[string]struct{}
}

type Parser interface {
	Parse(pageURL string, r io.Reader) (ParseResult, error)
}

type HTMLParser struct{}

func (p HTMLParser) Parse(pageURL string, r io.Reader) (ParseResult, error) {
	node, err := html.Parse(r)
	if err != nil {
		return ParseResult{}, err
	}

	links := make([]LinkOccurrence, 0)
	fragmentIDs := map[string]struct{}{}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n == nil {
			return
		}

		if n.Type == html.ElementNode {
			for _, attr := range n.Attr {
				switch {
				case attr.Key == "id":
					if attr.Val != "" {
						fragmentIDs[attr.Val] = struct{}{}
					}
				case n.Data == "a" && (attr.Key == "name"):
					if attr.Val != "" {
						fragmentIDs[attr.Val] = struct{}{}
					}
				}
			}

			if n.Data == "a" {
				var href string
				for _, attr := range n.Attr {
					if attr.Key == "href" {
						href = attr.Val
						break
					}
				}
				if href != "" {
					norm, err := NormalizeLink(href, pageURL)
					if err == nil {
						fragment := ""
						if idx := strings.IndexByte(norm.LinkURL, '#'); idx != -1 && idx+1 < len(norm.LinkURL) {
							fragment = norm.LinkURL[idx+1:]
						}
						links = append(links, LinkOccurrence{
							SourceURL: pageURL,
							LinkURL:   norm.LinkURL,
							FetchURL:  norm.FetchURL,
							Fragment:  fragment,
							Text:      linkText(n),
						})
					}
				}
			}
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)

	return ParseResult{
		Links:       links,
		FragmentIDs: fragmentIDs,
	}, nil
}

func linkText(n *html.Node) string {
	var b strings.Builder

	var walkText func(*html.Node)
	walkText = func(node *html.Node) {
		if node == nil {
			return
		}
		if node.Type == html.TextNode {
			b.WriteString(node.Data)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walkText(child)
		}
	}
	walkText(n)

	trimmed := strings.Join(strings.Fields(strings.TrimSpace(b.String())), " ")
	if len(trimmed) <= 80 {
		return trimmed
	}
	return trimmed[:80]
}
