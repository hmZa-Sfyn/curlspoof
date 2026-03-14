package main

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// ─── Public entry point ───────────────────────────────────────────────────────

// extract parses body HTML according to mode and pretty-prints results.
// baseURL is used to resolve relative hrefs.
func extract(mode, body, baseURL string) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "links":
		extractLinks(body, baseURL, false)
	case "links-text", "linktext", "links_text":
		extractLinks(body, baseURL, true)
	case "images", "imgs":
		extractImages(body, baseURL)
	case "headings", "headers":
		extractHeadings(body)
	case "title":
		extractTitle(body)
	case "text":
		extractText(body)
	case "forms":
		extractForms(body)
	case "scripts":
		extractScripts(body, baseURL)
	case "meta":
		extractMeta(body)
	default:
		fmt.Printf("%s✗ unknown extract mode: %s%s\n", red, mode, reset)
		fmt.Printf("%s  valid modes: links links-text images headings title text forms scripts meta%s\n", dim, reset)
	}
}

// ─── Links ────────────────────────────────────────────────────────────────────

func extractLinks(body, baseURL string, withText bool) {
	type link struct {
		href string
		text string
	}

	var links []link
	seen := map[string]bool{}

	base, _ := url.Parse(baseURL)

	forEachTag(body, "a", func(attrs map[string]string, inner string) {
		href := attrs["href"]
		if href == "" || strings.HasPrefix(href, "javascript:") || href == "#" {
			return
		}
		// resolve relative URLs
		if base != nil {
			if ref, err := url.Parse(href); err == nil {
				href = base.ResolveReference(ref).String()
			}
		}
		if seen[href] {
			return
		}
		seen[href] = true
		text := ""
		if withText {
			text = collapseWS(stripTags(inner))
		}
		links = append(links, link{href, text})
	})

	if len(links) == 0 {
		fmt.Printf("%s(no links found)%s\n", dim, reset)
		return
	}

	// sort: same-host first, then external, both groups alphabetically
	sort.Slice(links, func(i, j int) bool {
		hi := isSameHost(links[i].href, baseURL)
		hj := isSameHost(links[j].href, baseURL)
		if hi != hj {
			return hi // same-host first
		}
		return links[i].href < links[j].href
	})

	fmt.Printf("\n%s%s Links (%d)%s\n", bold, cyan, len(links), reset)
	fmt.Println(strings.Repeat("─", 72))

	for i, l := range links {
		num := fmt.Sprintf("%s%3d%s", dim, i+1, reset)
		if withText && l.text != "" {
			txt := truncate(l.text, 38)
			href := truncate(l.href, 72)
			fmt.Printf("%s  %s%-40s%s  %s%s%s\n",
				num, yellow, txt, reset, dim, href, reset)
		} else {
			fmt.Printf("%s  %s%s%s\n", num, green, truncate(l.href, 90), reset)
		}
	}
	fmt.Printf("\n%s%d links total%s\n\n", gray, len(links), reset)
}

// ─── Images ───────────────────────────────────────────────────────────────────

func extractImages(body, baseURL string) {
	base, _ := url.Parse(baseURL)
	var srcs []string
	seen := map[string]bool{}

	forEachTag(body, "img", func(attrs map[string]string, _ string) {
		src := attrs["src"]
		if src == "" {
			// try data-src (lazy load)
			src = attrs["data-src"]
		}
		if src == "" || strings.HasPrefix(src, "data:") {
			return
		}
		if base != nil {
			if ref, err := url.Parse(src); err == nil {
				src = base.ResolveReference(ref).String()
			}
		}
		if !seen[src] {
			seen[src] = true
			srcs = append(srcs, src)
		}
	})

	if len(srcs) == 0 {
		fmt.Printf("%s(no images found)%s\n", dim, reset)
		return
	}

	fmt.Printf("\n%s%s Images (%d)%s\n", bold, cyan, len(srcs), reset)
	fmt.Println(strings.Repeat("─", 72))
	for i, src := range srcs {
		fmt.Printf("%s%3d%s  %s%s%s\n", dim, i+1, reset, blue, src, reset)
	}
	fmt.Printf("\n%s%d images total%s\n\n", gray, len(srcs), reset)
}

// ─── Headings ─────────────────────────────────────────────────────────────────

func extractHeadings(body string) {
	type heading struct {
		level string
		text  string
	}

	heads := headingsInOrder(body)

	if len(heads) == 0 {
		fmt.Printf("%s(no headings found)%s\n", dim, reset)
		return
	}

	levelColor := map[string]string{
		"h1": bold + yellow,
		"h2": cyan,
		"h3": green,
		"h4": blue,
		"h5": purple,
		"h6": gray,
	}
	fmt.Printf("\n%s%s Headings (%d)%s\n", bold, cyan, len(heads), reset)
	fmt.Println(strings.Repeat("─", 72))
	for _, hd := range heads {
		indent := strings.Repeat("  ", headLevel(hd.level)-1)
		col := levelColor[hd.level]
		fmt.Printf("%s%s%s%s  %s%s%s\n",
			indent, col, strings.ToUpper(hd.level), reset,
			dim, truncate(hd.text, 80), reset)
	}
	fmt.Println()
}

type headingItem struct{ level, text string }

func headingsInOrder(body string) []headingItem {
	var out []headingItem
	lower := strings.ToLower(body)
	pos := 0
	for pos < len(lower) {
		// find the next opening heading tag
		best := -1
		bestTag := ""
		for _, tag := range []string{"h1", "h2", "h3", "h4", "h5", "h6"} {
			idx := strings.Index(lower[pos:], "<"+tag)
			if idx >= 0 {
				abs := pos + idx
				if best < 0 || abs < best {
					best = abs
					bestTag = tag
				}
			}
		}
		if best < 0 {
			break
		}
		// find close of opening tag
		closeOpen := strings.Index(lower[best:], ">")
		if closeOpen < 0 {
			break
		}
		contentStart := best + closeOpen + 1
		// find closing tag
		closeTag := strings.Index(lower[contentStart:], "</"+bestTag)
		if closeTag < 0 {
			pos = contentStart
			continue
		}
		inner := body[contentStart : contentStart+closeTag]
		text := collapseWS(stripTags(inner))
		if text != "" {
			out = append(out, headingItem{bestTag, text})
		}
		pos = contentStart + closeTag + len("</"+bestTag+">")
	}
	return out
}

func headLevel(tag string) int {
	if len(tag) == 2 && tag[0] == 'h' && tag[1] >= '1' && tag[1] <= '6' {
		return int(tag[1] - '0')
	}
	return 1
}

// ─── Title ────────────────────────────────────────────────────────────────────

func extractTitle(body string) {
	lower := strings.ToLower(body)
	start := strings.Index(lower, "<title")
	if start < 0 {
		fmt.Printf("%s(no <title> found)%s\n", dim, reset)
		return
	}
	closeTag := strings.Index(lower[start:], ">")
	if closeTag < 0 {
		return
	}
	contentStart := start + closeTag + 1
	end := strings.Index(lower[contentStart:], "</title")
	if end < 0 {
		return
	}
	title := collapseWS(stripTags(body[contentStart : contentStart+end]))
	fmt.Printf("\n%s%s Title%s\n", bold, cyan, reset)
	fmt.Println(strings.Repeat("─", 72))
	fmt.Printf("  %s%s%s\n\n", yellow, title, reset)
}

// ─── Text ─────────────────────────────────────────────────────────────────────

func extractText(body string) {
	// remove script and style blocks first
	for _, tag := range []string{"script", "style", "noscript", "head"} {
		body = removeTagBlock(body, tag)
	}
	text := stripTags(body)
	// split into non-empty lines
	lines := strings.Split(text, "\n")
	printed := 0

	fmt.Printf("\n%s%s Visible text%s\n", bold, cyan, reset)
	fmt.Println(strings.Repeat("─", 72))

	for _, l := range lines {
		l = collapseWS(l)
		if l == "" {
			continue
		}
		fmt.Printf("  %s\n", l)
		printed++
		if printed >= 200 {
			fmt.Printf("\n  %s… (showing first 200 lines)%s\n", dim, reset)
			break
		}
	}
	fmt.Printf("\n%s%d text lines%s\n\n", gray, printed, reset)
}

// ─── Forms ────────────────────────────────────────────────────────────────────

func extractForms(body string) {
	count := 0

	fmt.Printf("\n%s%s Forms%s\n", bold, cyan, reset)
	fmt.Println(strings.Repeat("─", 72))

	forEachTag(body, "form", func(attrs map[string]string, inner string) {
		count++
		action := attrs["action"]
		method := strings.ToUpper(attrs["method"])
		if method == "" {
			method = "GET"
		}
		fmt.Printf("\n  %sForm %d%s  %saction=%s%s  %smethod=%s%s\n",
			bold, count, reset,
			dim, action, reset,
			yellow, method, reset)

		// fields
		for _, field := range []string{"input", "select", "textarea", "button"} {
			forEachTag(inner, field, func(fa map[string]string, _ string) {
				name := fa["name"]
				typ := fa["type"]
				if typ == "" {
					typ = field
				}
				val := fa["value"]
				placeholder := fa["placeholder"]
				extra := ""
				if val != "" {
					extra = fmt.Sprintf("  value=%s%s%s", dim, truncate(val, 30), reset)
				}
				if placeholder != "" {
					extra += fmt.Sprintf("  placeholder=%s%s%s", dim, placeholder, reset)
				}
				fmt.Printf("    %s%-10s%s %s%-24s%s%s\n",
					green, typ, reset,
					cyan, name, reset, extra)
			})
		}
	})

	if count == 0 {
		fmt.Printf("  %s(no forms found)%s\n", dim, reset)
	}
	fmt.Println()
}

// ─── Scripts ──────────────────────────────────────────────────────────────────

func extractScripts(body, baseURL string) {
	base, _ := url.Parse(baseURL)
	var srcs []string
	seen := map[string]bool{}

	forEachTag(body, "script", func(attrs map[string]string, _ string) {
		src := attrs["src"]
		if src == "" {
			return
		}
		if base != nil {
			if ref, err := url.Parse(src); err == nil {
				src = base.ResolveReference(ref).String()
			}
		}
		if !seen[src] {
			seen[src] = true
			srcs = append(srcs, src)
		}
	})

	if len(srcs) == 0 {
		fmt.Printf("%s(no external scripts found)%s\n", dim, reset)
		return
	}
	fmt.Printf("\n%s%s Scripts (%d)%s\n", bold, cyan, len(srcs), reset)
	fmt.Println(strings.Repeat("─", 72))
	for i, src := range srcs {
		fmt.Printf("%s%3d%s  %s%s%s\n", dim, i+1, reset, blue, src, reset)
	}
	fmt.Println()
}

// ─── Meta ─────────────────────────────────────────────────────────────────────

func extractMeta(body string) {
	type metaTag struct {
		name    string
		content string
	}
	var metas []metaTag

	forEachTag(body, "meta", func(attrs map[string]string, _ string) {
		name := attrs["name"]
		if name == "" {
			name = attrs["property"]
		}
		if name == "" {
			name = attrs["http-equiv"]
		}
		content := attrs["content"]
		if name != "" {
			metas = append(metas, metaTag{name, content})
		}
	})

	if len(metas) == 0 {
		fmt.Printf("%s(no <meta> tags found)%s\n", dim, reset)
		return
	}
	fmt.Printf("\n%s%s Meta tags (%d)%s\n", bold, cyan, len(metas), reset)
	fmt.Println(strings.Repeat("─", 72))
	for _, m := range metas {
		fmt.Printf("  %s%-32s%s  %s%s%s\n",
			cyan, m.name, reset,
			dim, truncate(m.content, 60), reset)
	}
	fmt.Println()
}

// ─── HTML parsing primitives (no external deps) ───────────────────────────────

// forEachTag calls fn for every occurrence of <tagName …> … </tagName>.
// It passes parsed attributes and the raw inner HTML.
// It is NOT a full HTML parser — it handles real-world messy HTML well enough.
func forEachTag(body, tagName string, fn func(attrs map[string]string, inner string)) {
	lower := strings.ToLower(body)
	openMarker := "<" + tagName
	closeMarker := "</" + tagName

	pos := 0
	for {
		start := indexAt(lower, openMarker, pos)
		if start < 0 {
			break
		}
		// find end of opening tag
		closeOpen := indexAt(lower, ">", start)
		if closeOpen < 0 {
			break
		}
		// self-closing?
		selfClose := lower[closeOpen-1] == '/'
		attrStr := body[start+len(openMarker) : closeOpen]
		attrs := parseAttrs(attrStr)

		inner := ""
		nextPos := closeOpen + 1

		if !selfClose {
			// find matching close tag (handles 1-level nesting)
			depth := 1
			searchFrom := closeOpen + 1
			for depth > 0 {
				nextOpen := indexAt(lower, openMarker, searchFrom)
				nextClose := indexAt(lower, closeMarker, searchFrom)
				if nextClose < 0 {
					break
				}
				if nextOpen >= 0 && nextOpen < nextClose {
					depth++
					searchFrom = nextOpen + len(openMarker)
				} else {
					depth--
					if depth == 0 {
						inner = body[closeOpen+1 : nextClose]
						// advance past </tag>
						endClose := indexAt(lower, ">", nextClose)
						if endClose >= 0 {
							nextPos = endClose + 1
						} else {
							nextPos = nextClose + len(closeMarker)
						}
					} else {
						searchFrom = nextClose + len(closeMarker)
					}
				}
			}
		}

		fn(attrs, inner)
		pos = nextPos
	}
}

// parseAttrs extracts key="value", key='value', key=value, and standalone key.
func parseAttrs(s string) map[string]string {
	attrs := map[string]string{}
	s = strings.TrimSpace(s)
	for len(s) > 0 {
		s = strings.TrimSpace(s)
		if len(s) == 0 {
			break
		}
		// find key
		eq := strings.IndexAny(s, "= \t\r\n/>")
		if eq < 0 {
			attrs[strings.ToLower(s)] = ""
			break
		}
		key := strings.ToLower(strings.TrimSpace(s[:eq]))
		s = s[eq:]
		s = strings.TrimSpace(s)
		if len(s) == 0 || s[0] != '=' {
			if key != "" {
				attrs[key] = ""
			}
			continue
		}
		s = s[1:] // skip '='
		s = strings.TrimSpace(s)
		val := ""
		if len(s) == 0 {
			break
		}
		if s[0] == '"' || s[0] == '\'' {
			quote := s[0]
			end := strings.IndexByte(s[1:], quote)
			if end < 0 {
				val = s[1:]
				s = ""
			} else {
				val = s[1 : end+1]
				s = s[end+2:]
			}
		} else {
			end := strings.IndexAny(s, " \t\r\n>")
			if end < 0 {
				val = s
				s = ""
			} else {
				val = s[:end]
				s = s[end:]
			}
		}
		if key != "" {
			attrs[key] = val
		}
	}
	return attrs
}

// removeTagBlock removes <tag …> … </tag> blocks entirely.
func removeTagBlock(body, tag string) string {
	lower := strings.ToLower(body)
	open := "<" + tag
	close := "</" + tag + ">"
	var sb strings.Builder
	pos := 0
	for {
		start := indexAt(lower, open, pos)
		if start < 0 {
			sb.WriteString(body[pos:])
			break
		}
		sb.WriteString(body[pos:start])
		end := indexAt(lower, close, start)
		if end < 0 {
			break
		}
		pos = end + len(close)
	}
	return sb.String()
}

// stripTags removes all HTML tags from s.
func stripTags(s string) string {
	var sb strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
			sb.WriteRune(' ') // preserve word boundary
		case r == '>':
			inTag = false
		case !inTag:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// collapseWS collapses runs of whitespace to a single space and trims.
func collapseWS(s string) string {
	var sb strings.Builder
	space := false
	for _, r := range s {
		if r == '\t' || r == '\r' || r == '\n' || r == ' ' {
			if !space {
				sb.WriteRune(' ')
				space = true
			}
		} else {
			sb.WriteRune(r)
			space = false
		}
	}
	return strings.TrimSpace(sb.String())
}

func indexAt(s, sub string, from int) int {
	idx := strings.Index(s[from:], sub)
	if idx < 0 {
		return -1
	}
	return from + idx
}

func isSameHost(href, base string) bool {
	h, err := url.Parse(href)
	if err != nil {
		return false
	}
	b, err := url.Parse(base)
	if err != nil {
		return false
	}
	return strings.EqualFold(h.Host, b.Host)
}
