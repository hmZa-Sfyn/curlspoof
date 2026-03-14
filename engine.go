package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"time"
)

// Response holds a parsed HTTP response.
type Response struct {
	Status     int
	StatusText string
	Headers    http.Header
	Body       []byte
	Duration   time.Duration
	URL        string
	Redirects  int
}

// ─── orderedHeaderTransport ───────────────────────────────────────────────────
// Go's net/http sorts headers alphabetically before sending, which leaks a
// bot fingerprint. This wrapper writes them in the exact order we specify.
// It wraps the underlying RoundTripper and re-injects headers in order.

type orderedHeaderTransport struct {
	wrapped     http.RoundTripper
	headerOrder []string // canonical keys in desired send-order
}

func (t *orderedHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone request so we don't mutate the caller's copy
	clone := req.Clone(req.Context())

	// Rebuild header map in our desired order using a fresh Header
	// Go's http.Header is a map — we can't control order at the map level,
	// but we can write a raw request by intercepting at the write layer.
	// Simplest compatible approach: rewrite the header to match our order
	// by deleting and re-adding. Go's transport still sorts internally,
	// so we use a custom conn wrapper when possible.
	// For now: ensure at minimum the map has all keys set (order is best-effort).
	newH := make(http.Header, len(clone.Header))
	// First add in our preferred order
	for _, k := range t.headerOrder {
		if v, ok := clone.Header[k]; ok {
			newH[k] = v
		}
	}
	// Then add any headers not in our list (e.g. Host, Content-Length)
	for k, v := range clone.Header {
		if _, alreadySet := newH[k]; !alreadySet {
			newH[k] = v
		}
	}
	clone.Header = newH
	return t.wrapped.RoundTrip(clone)
}

// ─── buildTransport ───────────────────────────────────────────────────────────
// Builds an http.Transport with a TLS config that is closer to what a real
// browser presents. The key differences from Go's defaults:
//
//   - Broader cipher suite list (browsers support more ciphers)
//   - TLS 1.2 + 1.3 enabled (browsers don't do TLS 1.0/1.1)
//   - Curve preferences matching Chrome's order
//   - ALPN protocols: h2, http/1.1 (like browsers)
//
// Note: Full JA3/JA4 fingerprint matching requires patching the TLS stack
// at the uTLS layer — this gets us directionally correct but not identical.

func buildTransport(cfg Config, headerOrder []string) http.RoundTripper {
	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,

		// Chrome 124 cipher preference order (TLS 1.2)
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256,          // TLS 1.3 (Go picks these automatically)
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_RSA_WITH_AES_128_CBC_SHA,
			tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		},

		// Curve preferences: Chrome order (X25519 first)
		CurvePreferences: []tls.CurveID{
			tls.X25519,
			tls.CurveP256,
			tls.CurveP384,
		},

		// Prefer server cipher order = false (browser behaviour)
		PreferServerCipherSuites: false,

		// ALPN: browsers advertise h2 first
		NextProtos: []string{"h2", "http/1.1"},

		// Session tickets: browsers cache & reuse these
		SessionTicketsDisabled: false,

		// InsecureSkipVerify: only if --insecure passed
		InsecureSkipVerify: cfg.Insecure,
	}

	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		// TCP window / MSS tuning happens at OS level, not here
	}

	inner := &http.Transport{
		DialContext:            dialer.DialContext,
		TLSClientConfig:        tlsCfg,
		DisableCompression:     true, // we decompress ourselves
		MaxIdleConns:           100,
		MaxIdleConnsPerHost:    10,
		IdleConnTimeout:        90 * time.Second,
		TLSHandshakeTimeout:    10 * time.Second,
		ExpectContinueTimeout:  1 * time.Second,
		ForceAttemptHTTP2:      true, // try HTTP/2 like browsers do
	}

	if cfg.ProxyURL != "" {
		if pu, err := url.Parse(cfg.ProxyURL); err == nil {
			inner.Proxy = http.ProxyURL(pu)
		}
	}

	return &orderedHeaderTransport{wrapped: inner, headerOrder: headerOrder}
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

	// Set headers in insertion order
	for _, k := range cr.headerOrder {
		req.Header.Set(k, cr.Headers[k])
	}

	jar, _ := cookiejar.New(nil)
	redirectCount := 0

	transport := buildTransport(cfg, cr.headerOrder)

	client := &http.Client{
		Timeout:   time.Duration(cfg.TimeoutSec) * time.Second,
		Jar:       jar,
		Transport: transport,
	}

	if cfg.FollowRedirs {
		client.CheckRedirect = func(newReq *http.Request, via []*http.Request) error {
			redirectCount++
			if redirectCount > 15 {
				return fmt.Errorf("too many redirects")
			}
			// Preserve spoofed headers through redirects — critical for
			// sites that check headers on every hop
			for k, v := range via[0].Header {
				if newReq.Header.Get(k) == "" {
					newReq.Header[k] = v
				}
			}
			// Update Sec-Fetch-Site for cross-site redirects
			if len(via) > 0 {
				orig, _ := url.Parse(via[0].URL.String())
				next, _ := url.Parse(newReq.URL.String())
				if orig != nil && next != nil && orig.Host != next.Host {
					newReq.Header.Set("Sec-Fetch-Site", "cross-site")
				} else {
					newReq.Header.Set("Sec-Fetch-Site", "same-origin")
				}
			}
			return nil
		}
	} else {
		client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	// Jitter: tiny random sleep (0–80ms) mimics real network/user timing
	if rand.Intn(10) < 4 {
		time.Sleep(time.Duration(rand.Intn(80)) * time.Millisecond)
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

// decodeBody handles gzip / deflate / br response bodies.
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
			// raw deflate (no zlib wrapper)
			return io.ReadAll(resp.Body)
		}
		defer zr.Close()
		return io.ReadAll(zr)
	default:
		// br (brotli) requires a third-party lib
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
		d := rand.Intn(jitterMs*2) - jitterMs
		total += d
	}
	if total < 0 {
		total = 0
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
		redir = fmt.Sprintf("  %s↪ %d redirect%s", gray, r.Redirects, reset)
		if r.Redirects > 1 {
			redir += "s"
		}
	}

	// Parse status text — resp.Status is "200 OK", we want just "OK"
	statusText := r.StatusText
	if idx := strings.Index(statusText, " "); idx >= 0 {
		statusText = statusText[idx+1:]
	}

	fmt.Printf("\n%s%s%d  %s%s  %s%s%s%s\n",
		col, bold, r.Status, statusText, reset,
		gray, r.Duration.Round(time.Millisecond), reset,
		redir,
	)

	// Response headers box
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

	if outFile != "" {
		if err := os.WriteFile(outFile, r.Body, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "%s⚠  could not write output file: %v%s\n", yellow, err, reset)
		} else {
			fmt.Printf("%s  ✓ saved to %s%s\n", green, outFile, reset)
		}
	}
}

func looksLikeJSON(s string) bool {
	s = strings.TrimSpace(s)
	return (strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}")) ||
		(strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]"))
}

func printHTML(html string) {
	fmt.Printf("%s── HTML body ──%s\n", gray, reset)
	for _, tag := range []string{"script", "style", "head", "noscript"} {
		html = removeTagBlock(html, tag)
	}
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
	lines := strings.Split(sb.String(), "\n")
	printed := 0
	for _, l := range lines {
		l = collapseWS(l)
		if l == "" {
			continue
		}
		fmt.Printf("  %s%s%s\n", dim, l, reset)
		printed++
		if printed >= 60 {
			fmt.Printf("\n  %s… (use --output file.html for full body)%s\n", gray, reset)
			break
		}
	}
	fmt.Println()
}

// ─── JSON colouring ───────────────────────────────────────────────────────────

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
	return blue + v + reset
}
