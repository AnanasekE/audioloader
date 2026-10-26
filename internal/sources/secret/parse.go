package secret

import (
	"bytes"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// found is what parsePage reads out of a page.
type found struct {
	title, description string
	// tagged is the audio URL named in og:audio / og:audio:secure_url, else in
	// an <audio> or <source> tag, as written (it may be relative); "" if none
	tagged string
	// texts are the absolute URLs in the page's text, in order, for pages that
	// set their player up in a script
	texts []string
	// links are the page's <a href> links with their text, in order
	links []link
}

type link struct{ href, text string }

const (
	maxTitle       = 200
	maxDescription = 10000
)

// parsePage reads a page. It decides nothing about hosts: Inspect does.
func parsePage(body []byte, base *url.URL) found {
	var f found
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		// html.Parse is tolerant; treat the rare failure as a page without markup
		f.texts = urlsIn(string(body))
		return f
	}

	var ogAudio, ogSecure, ogTitle, ogDesc string
	var tagAudio, tagSource, docTitle, jpTitle, jpDesc string
	var visit func(n *html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "meta":
				content := strings.TrimSpace(attr(n, "content"))
				switch strings.ToLower(attr(n, "property")) {
				case "og:audio":
					ogAudio = firstNonEmpty(ogAudio, content)
				case "og:audio:secure_url":
					ogSecure = firstNonEmpty(ogSecure, content)
				case "og:title":
					ogTitle = firstNonEmpty(ogTitle, content)
				case "og:description":
					ogDesc = firstNonEmpty(ogDesc, content)
				}
			case "audio":
				tagAudio = firstNonEmpty(tagAudio, strings.TrimSpace(attr(n, "src")))
			case "source":
				tagSource = firstNonEmpty(tagSource, strings.TrimSpace(attr(n, "src")))
			case "title":
				if docTitle == "" {
					docTitle = oneLine(text(n))
				}
			case "a":
				if href := strings.TrimSpace(attr(n, "href")); href != "" {
					f.links = append(f.links, link{href, cut(oneLine(text(n)), maxTitle)})
				}
			}
			// jPlayer's markup names its parts by class
			if hasClass(n, "jp-title") && jpTitle == "" {
				jpTitle = oneLine(text(n))
			}
			if hasClass(n, "jp-description") && jpDesc == "" {
				jpDesc = blockText(n)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(doc)

	f.tagged = firstNonEmpty(ogAudio, ogSecure, tagAudio, tagSource)
	f.title = cut(oneLine(firstNonEmpty(ogTitle, jpTitle, docTitle)), maxTitle)
	f.description = cut(firstNonEmpty(ogDesc, jpDesc), maxDescription)
	f.texts = urlsIn(string(body))
	return f
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

func hasClass(n *html.Node, class string) bool {
	for _, c := range strings.Fields(attr(n, "class")) {
		if c == class {
			return true
		}
	}
	return false
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

// text is the plain text under n.
func text(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

// blockText is the text under n with its line breaks kept: <br> and the end
// of a paragraph or block start a new line, all other markup is dropped.
func blockText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			b.WriteString(n.Data)
			return
		case html.ElementNode:
			switch n.Data {
			case "script", "style":
				return
			case "br":
				b.WriteString("\n")
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode {
			switch n.Data {
			case "p", "div", "li", "h1", "h2", "h3", "h4", "h5", "h6":
				b.WriteString("\n")
			}
		}
	}
	walk(n)
	return tidyLines(b.String())
}

// tidyLines trims each line, and keeps at most one empty line in a row.
func tidyLines(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), " ", " ")
	var out []string
	blank := 0
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			blank++
			if blank > 1 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func cut(s string, max int) string {
	if r := []rune(s); len(r) > max {
		return string(r[:max])
	}
	return s
}

var absURL = regexp.MustCompile(`https?://[^\s"'<>\\)\]}]+`)

// urlsIn lists the absolute URLs in raw page text, in order. Scripts often
// escape slashes (https:\/\/host\/file), so those are undone first.
func urlsIn(s string) []string {
	s = strings.ReplaceAll(s, `\/`, `/`)
	s = strings.ReplaceAll(s, `&amp;`, `&`)
	return absURL.FindAllString(s, -1)
}

var bracketTag = regexp.MustCompile(`\[([^\[\]\n]{1,60})\]`)

// BracketTags lists the tags written as [Tag] in texts, in order, without
// repeats (ignoring case). A comma would split a tag in the form's field, so
// it becomes a space.
func BracketTags(texts ...string) []string {
	var out []string
	seen := map[string]bool{}
	for _, t := range texts {
		for _, m := range bracketTag.FindAllStringSubmatch(t, -1) {
			tag := oneLine(strings.ReplaceAll(m[1], ",", " "))
			if tag == "" || seen[strings.ToLower(tag)] {
				continue
			}
			seen[strings.ToLower(tag)] = true
			out = append(out, tag)
		}
	}
	return out
}
