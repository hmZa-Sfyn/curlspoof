package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strings"
)

// ─── Profile ──────────────────────────────────────────────────────────────────

type BrowserProfile struct {
	Name    string
	OS      string
	Engine  string
	UA      string
	// Headers injected only when absent from the original request.
	// Order matters: we preserve a realistic send-order.
	Headers []KV
}

// KV is an ordered key-value header pair.
type KV struct{ K, V string }

// InjectionResult records what was added.
type InjectionResult struct {
	Profile  *BrowserProfile
	Injected []KV // headers that were actually added
}

// ─── Profiles ─────────────────────────────────────────────────────────────────

var Profiles = []BrowserProfile{
	// ── Chrome 124 / Windows 10 ──────────────────────────────────────────────
	{
		Name:   "chrome-124-win",
		OS:     "Windows",
		Engine: "Chromium",
		UA:     "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
		Headers: []KV{
			{"Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"},
			{"Accept-Encoding", "gzip, deflate, br, zstd"},
			{"Accept-Language", "en-US,en;q=0.9"},
			{"Cache-Control", "max-age=0"},
			{"Connection", "keep-alive"},
			{"Upgrade-Insecure-Requests", "1"},
			{"Sec-Fetch-Dest", "document"},
			{"Sec-Fetch-Mode", "navigate"},
			{"Sec-Fetch-Site", "none"},
			{"Sec-Fetch-User", "?1"},
			{"Sec-CH-UA", `"Chromium";v="124", "Google Chrome";v="124", "Not-A.Brand";v="99"`},
			{"Sec-CH-UA-Mobile", "?0"},
			{"Sec-CH-UA-Platform", `"Windows"`},
			{"Priority", "u=0, i"},
		},
	},
	// ── Chrome 124 / macOS ───────────────────────────────────────────────────
	{
		Name:   "chrome-124-mac",
		OS:     "macOS",
		Engine: "Chromium",
		UA:     "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
		Headers: []KV{
			{"Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"},
			{"Accept-Encoding", "gzip, deflate, br, zstd"},
			{"Accept-Language", "en-US,en;q=0.9"},
			{"Cache-Control", "max-age=0"},
			{"Connection", "keep-alive"},
			{"Upgrade-Insecure-Requests", "1"},
			{"Sec-Fetch-Dest", "document"},
			{"Sec-Fetch-Mode", "navigate"},
			{"Sec-Fetch-Site", "none"},
			{"Sec-Fetch-User", "?1"},
			{"Sec-CH-UA", `"Chromium";v="124", "Google Chrome";v="124", "Not-A.Brand";v="99"`},
			{"Sec-CH-UA-Mobile", "?0"},
			{"Sec-CH-UA-Platform", `"macOS"`},
			{"Priority", "u=0, i"},
		},
	},
	// ── Chrome 124 / Android (Pixel 8) ───────────────────────────────────────
	{
		Name:   "chrome-124-android",
		OS:     "Android",
		Engine: "Chromium",
		UA:     "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.6367.82 Mobile Safari/537.36",
		Headers: []KV{
			{"Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"},
			{"Accept-Encoding", "gzip, deflate, br, zstd"},
			{"Accept-Language", "en-US,en;q=0.9"},
			{"Cache-Control", "max-age=0"},
			{"Connection", "keep-alive"},
			{"Upgrade-Insecure-Requests", "1"},
			{"Sec-Fetch-Dest", "document"},
			{"Sec-Fetch-Mode", "navigate"},
			{"Sec-Fetch-Site", "none"},
			{"Sec-Fetch-User", "?1"},
			{"Sec-CH-UA", `"Chromium";v="124", "Google Chrome";v="124", "Not-A.Brand";v="99"`},
			{"Sec-CH-UA-Mobile", "?1"},
			{"Sec-CH-UA-Platform", `"Android"`},
		},
	},
	// ── Firefox 125 / Windows ────────────────────────────────────────────────
	{
		Name:   "firefox-125-win",
		OS:     "Windows",
		Engine: "Gecko",
		UA:     "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:125.0) Gecko/20100101 Firefox/125.0",
		Headers: []KV{
			{"Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8"},
			{"Accept-Language", "en-US,en;q=0.5"},
			{"Accept-Encoding", "gzip, deflate, br"},
			{"Connection", "keep-alive"},
			{"Upgrade-Insecure-Requests", "1"},
			{"Sec-Fetch-Dest", "document"},
			{"Sec-Fetch-Mode", "navigate"},
			{"Sec-Fetch-Site", "none"},
			{"Sec-Fetch-User", "?1"},
			{"TE", "trailers"},
		},
	},
	// ── Firefox 125 / Linux ──────────────────────────────────────────────────
	{
		Name:   "firefox-125-linux",
		OS:     "Linux",
		Engine: "Gecko",
		UA:     "Mozilla/5.0 (X11; Linux x86_64; rv:125.0) Gecko/20100101 Firefox/125.0",
		Headers: []KV{
			{"Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8"},
			{"Accept-Language", "en-US,en;q=0.5"},
			{"Accept-Encoding", "gzip, deflate, br"},
			{"Connection", "keep-alive"},
			{"Upgrade-Insecure-Requests", "1"},
			{"Sec-Fetch-Dest", "document"},
			{"Sec-Fetch-Mode", "navigate"},
			{"Sec-Fetch-Site", "none"},
			{"Sec-Fetch-User", "?1"},
			{"TE", "trailers"},
		},
	},
	// ── Safari 17 / macOS ────────────────────────────────────────────────────
	{
		Name:   "safari-17-mac",
		OS:     "macOS",
		Engine: "WebKit",
		UA:     "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_4_1) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4.1 Safari/605.1.15",
		Headers: []KV{
			{"Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"},
			{"Accept-Language", "en-US,en;q=0.9"},
			{"Accept-Encoding", "gzip, deflate, br"},
			{"Connection", "keep-alive"},
			{"Upgrade-Insecure-Requests", "1"},
		},
	},
	// ── Safari 17 / iOS ──────────────────────────────────────────────────────
	{
		Name:   "safari-17-ios",
		OS:     "iOS",
		Engine: "WebKit",
		UA:     "Mozilla/5.0 (iPhone; CPU iPhone OS 17_4_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4.1 Mobile/15E148 Safari/604.1",
		Headers: []KV{
			{"Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"},
			{"Accept-Language", "en-US,en;q=0.9"},
			{"Accept-Encoding", "gzip, deflate, br"},
			{"Connection", "keep-alive"},
			{"Upgrade-Insecure-Requests", "1"},
		},
	},
	// ── Edge 124 / Windows ───────────────────────────────────────────────────
	{
		Name:   "edge-124-win",
		OS:     "Windows",
		Engine: "Chromium",
		UA:     "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36 Edg/124.0.0.0",
		Headers: []KV{
			{"Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"},
			{"Accept-Encoding", "gzip, deflate, br, zstd"},
			{"Accept-Language", "en-US,en;q=0.9"},
			{"Cache-Control", "max-age=0"},
			{"Connection", "keep-alive"},
			{"Upgrade-Insecure-Requests", "1"},
			{"Sec-Fetch-Dest", "document"},
			{"Sec-Fetch-Mode", "navigate"},
			{"Sec-Fetch-Site", "none"},
			{"Sec-Fetch-User", "?1"},
			{"Sec-CH-UA", `"Microsoft Edge";v="124", "Chromium";v="124", "Not-A.Brand";v="99"`},
			{"Sec-CH-UA-Mobile", "?0"},
			{"Sec-CH-UA-Platform", `"Windows"`},
			{"Priority", "u=0, i"},
		},
	},
}

// ─── UA version pools ─────────────────────────────────────────────────────────

var chromeVersions = []string{"120", "121", "122", "123", "124", "125"}
var firefoxVersions = []string{"122.0", "123.0", "124.0", "125.0"}
var edgeVersions = []string{"120.0.0.0", "121.0.0.0", "122.0.0.0", "123.0.0.0", "124.0.0.0"}

var acceptLanguages = []string{
	"en-US,en;q=0.9",
	"en-GB,en;q=0.9",
	"en-US,en;q=0.9,fr;q=0.8",
	"en-US,en;q=0.9,de;q=0.8",
	"en-US,en;q=0.5",
	"en-CA,en;q=0.9,fr-CA;q=0.8",
	"en-US,en;q=0.9,es;q=0.8",
}

// ─── Referer pools (realistic entry points) ───────────────────────────────────

var referers = []string{
	"https://www.google.com/",
	"https://www.google.com/search?q=site",
	"https://duckduckgo.com/",
	"https://www.bing.com/search?q=site",
	"https://t.co/",
	"https://www.reddit.com/",
}

// ─── Mutate UA version ────────────────────────────────────────────────────────

func mutateUA(ua string) string {
	bump := func(ua, marker string, pool []string) string {
		idx := strings.Index(ua, marker)
		if idx < 0 {
			return ua
		}
		start := idx + len(marker)
		end := start
		for end < len(ua) && (ua[end] >= '0' && ua[end] <= '9' || ua[end] == '.') {
			end++
		}
		return ua[:start] + pool[rand.Intn(len(pool))] + ua[end:]
	}
	if strings.Contains(ua, "Chrome/") {
		ua = bump(ua, "Chrome/", chromeVersions)
	}
	if strings.Contains(ua, "Edg/") {
		ua = bump(ua, "Edg/", edgeVersions)
	}
	if strings.Contains(ua, "Firefox/") {
		ua = bump(ua, "Firefox/", firefoxVersions)
	}
	return ua
}

// ─── pickProfile ─────────────────────────────────────────────────────────────

func pickProfile(name string) *BrowserProfile {
	if name == "" || name == "random" {
		p := Profiles[rand.Intn(len(Profiles))]
		return &p
	}
	for i := range Profiles {
		if Profiles[i].Name == name {
			return &Profiles[i]
		}
	}
	fmt.Fprintf(os.Stderr, "%s⚠ unknown profile '%s', using random%s\n", yellow, name, reset)
	p := Profiles[rand.Intn(len(Profiles))]
	return &p
}

// ─── buildSpoofHeaders ────────────────────────────────────────────────────────
// Injects browser fingerprint headers into cr.
// Existing headers in cr are NEVER overwritten.

func buildSpoofHeaders(cr *CurlRequest, profileName string) InjectionResult {
	p := pickProfile(profileName)

	// clone profile and mutate UA version
	profile := *p
	profile.UA = mutateUA(profile.UA)

	var injected []KV

	set := func(k, v string) {
		if !cr.hasHeader(k) {
			cr.setHeader(k, v)
			injected = append(injected, KV{k, v})
		}
	}

	// 1. User-Agent (most critical)
	set("User-Agent", profile.UA)

	// 2. All profile-specific headers
	for _, kv := range profile.Headers {
		set(kv.K, kv.V)
	}

	// 3. Randomise Accept-Language if not already present
	set("Accept-Language", acceptLanguages[rand.Intn(len(acceptLanguages))])

	// 4. Referer — realistic entry point
	set("Referer", referers[rand.Intn(len(referers))])

	// 5. DNT (1-in-3 browsers send it)
	if rand.Intn(3) == 0 {
		set("DNT", "1")
	}

	// 6. Chromium: randomise Sec-CH-UA version to match UA
	if strings.Contains(profile.UA, "Chrome/") {
		ver := extractChromeVersion(profile.UA)
		if ver != "" {
			chUA := fmt.Sprintf(`"Chromium";v="%s", "Google Chrome";v="%s", "Not-A.Brand";v="99"`, ver, ver)
			// override our own injected value to keep version consistent
			cr.Headers[http.CanonicalHeaderKey("Sec-CH-UA")] = chUA
		}
	}
	if strings.Contains(profile.UA, "Edg/") {
		ver := extractEdgeVersion(profile.UA)
		if ver != "" {
			chUA := fmt.Sprintf(`"Microsoft Edge";v="%s", "Chromium";v="%s", "Not-A.Brand";v="99"`, ver, ver)
			cr.Headers[http.CanonicalHeaderKey("Sec-CH-UA")] = chUA
		}
	}

	return InjectionResult{Profile: &profile, Injected: injected}
}

// ─── Version extractors ───────────────────────────────────────────────────────

func extractChromeVersion(ua string) string {
	return extractVersionAfter(ua, "Chrome/")
}

func extractEdgeVersion(ua string) string {
	return extractVersionAfter(ua, "Edg/")
}

func extractVersionAfter(s, marker string) string {
	idx := strings.Index(s, marker)
	if idx < 0 {
		return ""
	}
	start := idx + len(marker)
	end := start
	for end < len(s) && (s[end] >= '0' && s[end] <= '9') {
		end++
	}
	return s[start:end]
}

// ─── listProfiles ─────────────────────────────────────────────────────────────

func listProfiles() {
	lines := make([]string, 0, len(Profiles))
	for _, p := range Profiles {
		lines = append(lines, fmt.Sprintf(
			"%s%-22s%s %s%-8s%s %s%s%s",
			cyan, p.Name, reset,
			yellow, p.Engine, reset,
			dim, truncate(p.UA, 60), reset,
		))
	}
	printBox("Available profiles", lines)
}
