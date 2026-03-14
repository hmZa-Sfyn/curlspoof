package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"time"
)

// Response holds the parsed response.
type Response struct {
	Status     int
	StatusText string
	Headers    http.Header
	Body       []byte
	Duration   time.Duration
	URL        string
	Redirects  int
}

// ─── Fire ─────────────────────────────────────────────────────────────────────

func fire(cr *CurlRequest, cfg Config) (*Response, error) {
	var bodyReader io.Reader
	if cr.Body != "" {
		bodyReader = strings.NewReader(cr.Body)
	}

	req, err := http.NewRequest(cr.Method, cr.URL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	// apply headers in insertion order for realistic ordering
	for _, k := range cr.headerOrder {
		req.Header.Set(k, cr.Headers[k])
	}

	// proxy
	transport := &http.Transport{
		DisableCompression: true, // we decompress ourselves
		MaxIdleConns:       100,
		IdleConnTimeout:    90 * time.Second,
	}
	if cfg.ProxyURL != "" {
		proxyURL, err := url.Parse(cfg.ProxyURL)
		if err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	jar, _ := cookiejar.New(nil)
	redirectCount := 0

	client := &http.Client{
		Timeout:   time.Duration(cfg.TimeoutSec) * time.Second,
		Jar:       jar,
		Transport: transport,
	}

	if cfg.FollowRedirs {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			redirectCount++
			if redirectCount > 15 {
				return fmt.Errorf("too many redirects")
			}
			// preserve spoofed headers through redirects
			for k, v := range via[0].Header {
				if req.Header.Get(k) == "" {
					req.Header[k] = v
				}
			}
			return nil
		}
	} else {
		client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	start := time.Now()
	resp, err := client.Do(req)
	dur := time.Since(start)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := decodeBody(resp)
	if err != nil {
		body = []byte(fmt.Sprintf("[decode error: %v]", err))
	}

	finalURL := resp.Request.URL.String()

	return &Response{
		Status:     resp.StatusCode,
		StatusText: resp.Status,
		Headers:    resp.Header,
		Body:       body,
		Duration:   dur,
		URL:        finalURL,
		Redirects:  redirectCount,
	}, nil
}

// decodeBody decompresses gzip/deflate/br bodies.
func decodeBody(resp *http.Response) ([]byte, error) {
	ce := strings.ToLower(resp.Header.Get("Content-Encoding"))
	switch {
	case strings.Contains(ce, "gzip"):
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return io.ReadAll(resp.Body)
		}
		defer gr.Close()
		return io.ReadAll(gr)
	case strings.Contains(ce, "deflate"):
		zr, err := zlib.NewReader(resp.Body)
		if err != nil {
			return io.ReadAll(resp.Body)
		}
		defer zr.Close()
		return io.ReadAll(zr)
	default:
		// br (brotli) requires a third-party lib; just read raw
		return io.ReadAll(resp.Body)
	}
}

// ─── humanDelay ───────────────────────────────────────────────────────────────

func humanDelay(ms, jitterMs int) {
	if ms <= 0 && jitterMs <= 0 {
		return
	}
	total := ms
	if jitterMs > 0 {
		total += rand.Intn(jitterMs*2) - jitterMs
		if total < 0 {
			total = 0
		}
	}
	if total > 0 {
		fmt.Printf("%s  ⏱  waiting %dms…%s\n", gray, total, reset)
		time.Sleep(time.Duration(total) * time.Millisecond)
	}
}

// ─── printResponse ────────────────────────────────────────────────────────────

func printResponse(r *Response, outFile string) {
	col := statusColor(r.Status)

	redir := ""
	if r.Redirects > 0 {
		redir = fmt.Sprintf(" %s(%d redirect)%s", gray, r.Redirects, reset)
	}

	fmt.Printf("\n%s%s%d  %s%s  %s%s%s%s\n",
		col, bold, r.Status, r.StatusText[len(fmt.Sprintf("%d ", r.Status)):], reset,
		gray, r.Duration.Round(time.Millisecond), reset,
		redir,
	)

	// response headers
	var hlines []string
	for k, vv := range r.Headers {
		hlines = append(hlines, fmt.Sprintf("%s%s%s: %s%s%s",
			cyan, k, reset, dim, strings.Join(vv, ", "), reset))
	}
	printBox("Response Headers", hlines)

	if len(r.Body) == 0 {
		fmt.Printf("%s(empty body)%s\n\n", dim, reset)
		return
	}

	// print body
	bodyStr := string(r.Body)
	ct := r.Headers.Get("Content-Type")

	if strings.Contains(ct, "json") || looksLikeJSON(bodyStr) {
		var pretty bytes.Buffer
		if json.Indent(&pretty, r.Body, "", "  ") == nil {
			colorJSON(pretty.String())
			fmt.Println()
		} else {
			fmt.Println(bodyStr)
		}
	} else if strings.Contains(ct, "html") {
		printHTML(bodyStr)
	} else {
		fmt.Println(bodyStr)
	}

	// optionally save to file
	if outFile != "" {
		if err := os.WriteFile(outFile, r.Body, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "%s⚠ could not write output file: %v%s\n", yellow, err, reset)
		} else {
			fmt.Printf("%s  ✓ body saved to %s%s\n", green, outFile, reset)
		}
	}
}

func looksLikeJSON(s string) bool {
	s = strings.TrimSpace(s)
	return (strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}")) ||
		(strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]"))
}

// printHTML strips tags and shows text content of HTML responses.
func printHTML(html string) {
	fmt.Printf("%s── HTML body ──%s\n", gray, reset)
	// strip script/style blocks
	for _, tag := range []string{"script", "style", "head"} {
		for {
			start := strings.Index(strings.ToLower(html), "<"+tag)
			if start < 0 {
				break
			}
			end := strings.Index(strings.ToLower(html[start:]), "</"+tag+">")
			if end < 0 {
				break
			}
			html = html[:start] + html[start+end+len("</"+tag+">"):]
		}
	}
	// strip all remaining tags
	inTag := false
	var sb strings.Builder
	for _, r := range html {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
			sb.WriteRune(' ')
		case !inTag:
			sb.WriteRune(r)
		}
	}
	// clean up whitespace
	lines := strings.Split(sb.String(), "\n")
	printed := 0
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		fmt.Printf("  %s%s%s\n", dim, l, reset)
		printed++
		if printed > 40 {
			fmt.Printf("  %s… (truncated, use --output to save full body)%s\n", gray, reset)
			break
		}
	}
	fmt.Println()
}

// ─── JSON syntax highlighting ─────────────────────────────────────────────────

func colorJSON(src string) {
	scanner := bufio.NewScanner(strings.NewReader(src))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]

		switch {
		case strings.HasPrefix(trimmed, `"`) && strings.Contains(trimmed, ":"):
			if idx := strings.Index(trimmed, `":`); idx >= 0 {
				key := trimmed[:idx+2]
				val := strings.TrimSpace(trimmed[idx+2:])
				fmt.Printf("%s%s%s%s\n", indent, cyan+key+reset, colorVal(val), "")
				continue
			}
		case strings.HasPrefix(trimmed, `"`):
			fmt.Printf("%s%s%s%s\n", indent, green, trimmed, reset)
			continue
		case trimmed == "{" || trimmed == "}" || trimmed == "[" || trimmed == "]" ||
			strings.HasSuffix(trimmed, "{") || strings.HasSuffix(trimmed, "["):
			fmt.Printf("%s%s%s%s\n", indent, gray, trimmed, reset)
			continue
		}
		fmt.Println(line)
	}
}

func colorVal(v string) string {
	v2 := strings.TrimRight(v, ",")
	switch {
	case v2 == "true" || v2 == "false":
		return yellow + v + reset
	case v2 == "null":
		return red + v + reset
	case len(v2) > 0 && v2[0] == '"':
		return green + v + reset
	}
	// number or other
	return blue + v + reset
}
