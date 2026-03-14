package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"
)

// ─── Types ────────────────────────────────────────────────────────────────────

type BrowserProfile struct {
	Name    string
	OS      string
	Engine  string
	UA      string
	Headers []KV // sent in this exact order — order matters for fingerprinting

	// Behavioural metadata (used to build dynamic headers)
	ChromeMajor int    // non-zero for Chromium family
	FFMajor     int    // non-zero for Gecko family
	Platform    string // "Windows", "macOS", "Linux", "Android", "iOS"
	Mobile      bool
}

type KV struct{ K, V string }

type InjectionResult struct {
	Profile  *BrowserProfile
	Injected []KV
}

// ─── Version pools ────────────────────────────────────────────────────────────
// Versions are randomised per-request to avoid static fingerprinting.

// each entry: {major, minor-patch}
var chromeBuilds = []struct{ major, full string }{
	{"120", "120.0.6099.130"},
	{"121", "121.0.6167.184"},
	{"122", "122.0.6261.129"},
	{"123", "123.0.6312.122"},
	{"124", "124.0.6367.155"},
	{"125", "125.0.6422.112"},
}

var firefoxBuilds = []struct{ major, rv string }{
	{"122", "122.0"},
	{"123", "123.0"},
	{"124", "124.0"},
	{"125", "125.0"},
	{"126", "126.0"},
}

// Accept-Language: realistic pool including regional variants
var acceptLanguagePools = map[string][]string{
	"en-US": {
		"en-US,en;q=0.9",
		"en-US,en;q=0.8",
		"en-US,en;q=0.9,fr;q=0.7",
		"en-US,en;q=0.9,de;q=0.7",
		"en-US,en;q=0.9,es;q=0.7",
		"en-US,en;q=0.9,zh-CN;q=0.8,zh;q=0.7",
		"en-US,en;q=0.5",
	},
	"en-GB": {
		"en-GB,en;q=0.9",
		"en-GB,en-US;q=0.9,en;q=0.8",
		"en-GB,en;q=0.7",
	},
	"firefox": {
		"en-US,en;q=0.5",
		"en-US,en;q=0.7",
		"en-GB,en;q=0.5",
	},
}

// Referers: realistic entry points that look like organic traffic
var refererPool = []string{
	"https://www.google.com/",
	"https://www.google.com/search?q=amazon+deals",
	"https://www.google.com/search?q=buy+online",
	"https://www.google.co.uk/search?q=shop",
	"https://www.bing.com/search?q=amazon",
	"https://www.bing.com/",
	"https://duckduckgo.com/",
	"https://www.reddit.com/r/deals/",
	"https://news.ycombinator.com/",
	"https://t.co/",
	"https://www.facebook.com/",
	"https://www.instagram.com/",
	"", // no referer — direct navigation (common)
}

// Realistic viewport sizes for different device classes
var viewportDesktop = []string{
	"1920x1080", "1440x900", "1366x768",
	"2560x1440", "1280x800", "1600x900",
}
var viewportMobile = []string{
	"390x844", "414x896", "375x812",
	"412x915", "393x873", "360x780",
}

// ─── Profile definitions ──────────────────────────────────────────────────────
// Headers are listed in the EXACT order Chrome/Firefox sends them —
// anti-bot systems check header order as part of fingerprinting.

var Profiles = []BrowserProfile{

	// ── Chrome 124 / Windows 10 ──────────────────────────────────────────────
	// Exact order from Wireshark capture of Chrome 124.0.6367.155 on Win10
	{
		Name: "chrome-124-win", OS: "Windows", Engine: "Chromium",
		ChromeMajor: 124, Platform: "Windows",
		UA: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.6367.155 Safari/537.36",
		Headers: []KV{
			{"Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"},
			{"Accept-Encoding", "gzip, deflate, br, zstd"},
			{"Accept-Language", "en-US,en;q=0.9"},
			{"Cache-Control", "max-age=0"},
			{"Connection", "keep-alive"},
			{"Sec-Ch-Ua", `"Chromium";v="124", "Google Chrome";v="124", "Not-A.Brand";v="99"`},
			{"Sec-Ch-Ua-Mobile", "?0"},
			{"Sec-Ch-Ua-Platform", `"Windows"`},
			{"Sec-Fetch-Dest", "document"},
			{"Sec-Fetch-Mode", "navigate"},
			{"Sec-Fetch-Site", "none"},
			{"Sec-Fetch-User", "?1"},
			{"Upgrade-Insecure-Requests", "1"},
			{"User-Agent", ""},          // filled dynamically
			{"Priority", "u=0, i"},
		},
	},

	// ── Chrome 124 / macOS Sonoma ─────────────────────────────────────────────
	{
		Name: "chrome-124-mac", OS: "macOS", Engine: "Chromium",
		ChromeMajor: 124, Platform: "macOS",
		UA: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.6367.155 Safari/537.36",
		Headers: []KV{
			{"Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"},
			{"Accept-Encoding", "gzip, deflate, br, zstd"},
			{"Accept-Language", "en-US,en;q=0.9"},
			{"Cache-Control", "max-age=0"},
			{"Connection", "keep-alive"},
			{"Sec-Ch-Ua", `"Chromium";v="124", "Google Chrome";v="124", "Not-A.Brand";v="99"`},
			{"Sec-Ch-Ua-Mobile", "?0"},
			{"Sec-Ch-Ua-Platform", `"macOS"`},
			{"Sec-Fetch-Dest", "document"},
			{"Sec-Fetch-Mode", "navigate"},
			{"Sec-Fetch-Site", "none"},
			{"Sec-Fetch-User", "?1"},
			{"Upgrade-Insecure-Requests", "1"},
			{"User-Agent", ""},
			{"Priority", "u=0, i"},
		},
	},

	// ── Chrome 125 / Windows 11 ──────────────────────────────────────────────
	{
		Name: "chrome-125-win", OS: "Windows", Engine: "Chromium",
		ChromeMajor: 125, Platform: "Windows",
		UA: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.6422.112 Safari/537.36",
		Headers: []KV{
			{"Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"},
			{"Accept-Encoding", "gzip, deflate, br, zstd"},
			{"Accept-Language", "en-US,en;q=0.9"},
			{"Cache-Control", "max-age=0"},
			{"Connection", "keep-alive"},
			{"Sec-Ch-Ua", `"Chromium";v="125", "Google Chrome";v="125", "Not-A.Brand";v="24"`},
			{"Sec-Ch-Ua-Mobile", "?0"},
			{"Sec-Ch-Ua-Platform", `"Windows"`},
			{"Sec-Fetch-Dest", "document"},
			{"Sec-Fetch-Mode", "navigate"},
			{"Sec-Fetch-Site", "none"},
			{"Sec-Fetch-User", "?1"},
			{"Upgrade-Insecure-Requests", "1"},
			{"User-Agent", ""},
			{"Priority", "u=0, i"},
		},
	},

	// ── Chrome 124 / Android Pixel 8 ─────────────────────────────────────────
	{
		Name: "chrome-124-android", OS: "Android", Engine: "Chromium",
		ChromeMajor: 124, Platform: "Android", Mobile: true,
		UA: "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.6367.82 Mobile Safari/537.36",
		Headers: []KV{
			{"Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"},
			{"Accept-Encoding", "gzip, deflate, br, zstd"},
			{"Accept-Language", "en-US,en;q=0.9"},
			{"Cache-Control", "max-age=0"},
			{"Connection", "keep-alive"},
			{"Sec-Ch-Ua", `"Chromium";v="124", "Google Chrome";v="124", "Not-A.Brand";v="99"`},
			{"Sec-Ch-Ua-Mobile", "?1"},
			{"Sec-Ch-Ua-Platform", `"Android"`},
			{"Sec-Fetch-Dest", "document"},
			{"Sec-Fetch-Mode", "navigate"},
			{"Sec-Fetch-Site", "none"},
			{"Sec-Fetch-User", "?1"},
			{"Upgrade-Insecure-Requests", "1"},
			{"User-Agent", ""},
		},
	},

	// ── Firefox 125 / Windows ────────────────────────────────────────────────
	// Firefox header order from Wireshark capture of FF 125.0 on Win10
	{
		Name: "firefox-125-win", OS: "Windows", Engine: "Gecko",
		FFMajor: 125, Platform: "Windows",
		UA: "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:125.0) Gecko/20100101 Firefox/125.0",
		Headers: []KV{
			{"User-Agent", ""},
			{"Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8"},
			{"Accept-Language", "en-US,en;q=0.5"},
			{"Accept-Encoding", "gzip, deflate, br"},
			{"Connection", "keep-alive"},
			{"Upgrade-Insecure-Requests", "1"},
			{"Sec-Fetch-Dest", "document"},
			{"Sec-Fetch-Mode", "navigate"},
			{"Sec-Fetch-Site", "none"},
			{"Sec-Fetch-User", "?1"},
			{"Priority", "u=1"},
			{"TE", "trailers"},
		},
	},

	// ── Firefox 126 / Windows ────────────────────────────────────────────────
	{
		Name: "firefox-126-win", OS: "Windows", Engine: "Gecko",
		FFMajor: 126, Platform: "Windows",
		UA: "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:126.0) Gecko/20100101 Firefox/126.0",
		Headers: []KV{
			{"User-Agent", ""},
			{"Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8"},
			{"Accept-Language", "en-US,en;q=0.5"},
			{"Accept-Encoding", "gzip, deflate, br"},
			{"Connection", "keep-alive"},
			{"Upgrade-Insecure-Requests", "1"},
			{"Sec-Fetch-Dest", "document"},
			{"Sec-Fetch-Mode", "navigate"},
			{"Sec-Fetch-Site", "none"},
			{"Sec-Fetch-User", "?1"},
			{"Priority", "u=1"},
			{"TE", "trailers"},
		},
	},

	// ── Firefox 125 / Linux ──────────────────────────────────────────────────
	{
		Name: "firefox-125-linux", OS: "Linux", Engine: "Gecko",
		FFMajor: 125, Platform: "Linux",
		UA: "Mozilla/5.0 (X11; Linux x86_64; rv:125.0) Gecko/20100101 Firefox/125.0",
		Headers: []KV{
			{"User-Agent", ""},
			{"Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8"},
			{"Accept-Language", "en-US,en;q=0.5"},
			{"Accept-Encoding", "gzip, deflate, br"},
			{"Connection", "keep-alive"},
			{"Upgrade-Insecure-Requests", "1"},
			{"Sec-Fetch-Dest", "document"},
			{"Sec-Fetch-Mode", "navigate"},
			{"Sec-Fetch-Site", "none"},
			{"Sec-Fetch-User", "?1"},
			{"Priority", "u=1"},
			{"TE", "trailers"},
		},
	},

	// ── Safari 17.4 / macOS Sonoma ───────────────────────────────────────────
	// Safari sends fewer headers and in a different order
	{
		Name: "safari-17-mac", OS: "macOS", Engine: "WebKit",
		Platform: "macOS",
		UA: "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_4_1) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4.1 Safari/605.1.15",
		Headers: []KV{
			{"User-Agent", ""},
			{"Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"},
			{"Accept-Language", "en-US,en;q=0.9"},
			{"Accept-Encoding", "gzip, deflate, br"},
			{"Connection", "keep-alive"},
		},
	},

	// ── Safari 17 / iPhone iOS 17.4 ──────────────────────────────────────────
	{
		Name: "safari-17-ios", OS: "iOS", Engine: "WebKit",
		Platform: "iOS", Mobile: true,
		UA: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_4_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4.1 Mobile/15E148 Safari/604.1",
		Headers: []KV{
			{"User-Agent", ""},
			{"Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"},
			{"Accept-Language", "en-US,en;q=0.9"},
			{"Accept-Encoding", "gzip, deflate, br"},
			{"Connection", "keep-alive"},
		},
	},

	// ── Edge 124 / Windows ───────────────────────────────────────────────────
	{
		Name: "edge-124-win", OS: "Windows", Engine: "Chromium",
		ChromeMajor: 124, Platform: "Windows",
		UA: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36 Edg/124.0.0.0",
		Headers: []KV{
			{"Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"},
			{"Accept-Encoding", "gzip, deflate, br, zstd"},
			{"Accept-Language", "en-US,en;q=0.9"},
			{"Cache-Control", "max-age=0"},
			{"Connection", "keep-alive"},
			{"Sec-Ch-Ua", `"Microsoft Edge";v="124", "Chromium";v="124", "Not-A.Brand";v="99"`},
			{"Sec-Ch-Ua-Mobile", "?0"},
			{"Sec-Ch-Ua-Platform", `"Windows"`},
			{"Sec-Fetch-Dest", "document"},
			{"Sec-Fetch-Mode", "navigate"},
			{"Sec-Fetch-Site", "none"},
			{"Sec-Fetch-User", "?1"},
			{"Upgrade-Insecure-Requests", "1"},
			{"User-Agent", ""},
			{"Priority", "u=0, i"},
		},
	},
}

// ─── Dynamic header generators ────────────────────────────────────────────────

// buildSecCHUA returns consistent Sec-CH-UA / Sec-CH-UA-Full-Version-List
// values that match the actual UA version being used.
func buildSecCHUA(profile *BrowserProfile, chromeBuild, edgeBuild string) map[string]string {
	out := map[string]string{}
	if profile.ChromeMajor == 0 {
		return out
	}

	major := chromeBuild // e.g. "124"
	full := ""
	for _, b := range chromeBuilds {
		if b.major == major {
			full = b.full
			break
		}
	}
	if full == "" {
		full = major + ".0.0.0"
	}

	isEdge := edgeBuild != ""

	if isEdge {
		out["Sec-Ch-Ua"] = fmt.Sprintf(`"Microsoft Edge";v="%s", "Chromium";v="%s", "Not-A.Brand";v="99"`, major, major)
		out["Sec-Ch-Ua-Full-Version-List"] = fmt.Sprintf(`"Microsoft Edge";v="%s", "Chromium";v="%s", "Not-A.Brand";v="99.0.0.0"`, full, full)
	} else {
		out["Sec-Ch-Ua"] = fmt.Sprintf(`"Chromium";v="%s", "Google Chrome";v="%s", "Not-A.Brand";v="99"`, major, major)
		out["Sec-Ch-Ua-Full-Version-List"] = fmt.Sprintf(`"Chromium";v="%s", "Google Chrome";v="%s", "Not-A.Brand";v="99.0.0.0"`, full, full)
	}
	out["Sec-Ch-Ua-Arch"] = `"x86"`
	out["Sec-Ch-Ua-Bitness"] = `"64"`
	out["Sec-Ch-Ua-Full-Version"] = `"` + full + `"`

	if profile.Mobile {
		out["Sec-Ch-Ua-Mobile"] = "?1"
		out["Sec-Ch-Ua-Arch"] = `""`
	} else {
		out["Sec-Ch-Ua-Mobile"] = "?0"
	}
	out["Sec-Ch-Ua-Platform"] = `"` + profile.Platform + `"`
	switch profile.Platform {
	case "Windows":
		out["Sec-Ch-Ua-Platform-Version"] = `"15.0.0"` // Windows 11
	case "macOS":
		out["Sec-Ch-Ua-Platform-Version"] = `"14.4.1"`
	case "Android":
		out["Sec-Ch-Ua-Platform-Version"] = `"14.0.0"`
		out["Sec-Ch-Ua-Model"] = `"Pixel 8"`
		out["Sec-Ch-Ua-Arch"] = `""`
		out["Sec-Ch-Ua-Bitness"] = `"32"`
	}
	return out
}

// randomLang picks a locale appropriate for the profile's engine.
func randomLang(profile *BrowserProfile) string {
	pool := acceptLanguagePools["en-US"]
	if profile.FFMajor > 0 {
		pool = acceptLanguagePools["firefox"]
	} else if profile.Platform == "macOS" || profile.Platform == "iOS" {
		pool = acceptLanguagePools["en-GB"]
		if rand.Intn(3) != 0 {
			pool = acceptLanguagePools["en-US"]
		}
	}
	return pool[rand.Intn(len(pool))]
}

// pickReferer returns a realistic referer, sometimes empty (direct load).
func pickReferer() string {
	return refererPool[rand.Intn(len(refererPool))]
}

// ─── UA version mutation ─────────────────────────────────────────────────────

func mutateUA(ua string, profile *BrowserProfile) (string, string) {
	// returns (mutated UA, major version string)
	if profile.ChromeMajor > 0 {
		build := chromeBuilds[rand.Intn(len(chromeBuilds))]
		// Replace Chrome/xxx.x.xxxx.xxx
		ua = replaceVersionAfter(ua, "Chrome/", build.full)
		// For Edge, also replace Edg/
		if strings.Contains(ua, "Edg/") {
			ua = replaceVersionAfter(ua, "Edg/", build.major+".0.0.0")
		}
		return ua, build.major
	}
	if profile.FFMajor > 0 {
		build := firefoxBuilds[rand.Intn(len(firefoxBuilds))]
		// Replace rv:xxx.0 and Firefox/xxx.0
		ua = strings.ReplaceAll(ua, fmt.Sprintf("rv:%d.0", profile.FFMajor), "rv:"+build.rv)
		ua = replaceVersionAfter(ua, "Firefox/", build.rv)
		return ua, build.major
	}
	return ua, ""
}

func replaceVersionAfter(s, marker, newVer string) string {
	idx := strings.Index(s, marker)
	if idx < 0 {
		return s
	}
	start := idx + len(marker)
	end := start
	for end < len(s) && (s[end] >= '0' && s[end] <= '9' || s[end] == '.') {
		end++
	}
	return s[:start] + newVer + s[end:]
}

// ─── pickProfile ─────────────────────────────────────────────────────────────

func pickProfile(name string) *BrowserProfile {
	if name == "" || name == "random" {
		p := Profiles[rand.Intn(len(Profiles))]
		return &p
	}
	for i := range Profiles {
		if Profiles[i].Name == name {
			cp := Profiles[i]
			return &cp
		}
	}
	fmt.Fprintf(os.Stderr, "%s⚠  unknown profile '%s', using random%s\n", yellow, name, reset)
	p := Profiles[rand.Intn(len(Profiles))]
	return &p
}

// ─── buildSpoofHeaders ────────────────────────────────────────────────────────
// Applies full browser fingerprint to cr.
// Existing headers in cr are NEVER overwritten — spoof only fills gaps.

func buildSpoofHeaders(cr *CurlRequest, profileName string) InjectionResult {
	rand.Seed(time.Now().UnixNano()) //nolint

	profile := pickProfile(profileName)

	// 1. Mutate UA version (randomised build number)
	mutatedUA, chromeMajor := mutateUA(profile.UA, profile)
	profile.UA = mutatedUA

	// 2. Build dynamic Sec-CH-UA headers matching the UA version
	var edgeBuild string
	if strings.Contains(mutatedUA, "Edg/") {
		edgeBuild = chromeMajor
	}
	secCH := buildSecCHUA(profile, chromeMajor, edgeBuild)

	var injected []KV

	// set-if-missing helper — also records what was injected
	set := func(k, v string) {
		if v == "" {
			return
		}
		if !cr.hasHeader(k) {
			cr.setHeader(k, v)
			injected = append(injected, KV{k, v})
		}
	}

	// 3. Inject headers in the profile's canonical order
	for _, kv := range profile.Headers {
		v := kv.V
		// Placeholders filled dynamically
		switch kv.K {
		case "User-Agent":
			v = mutatedUA
		case "Accept-Language":
			if v == "" {
				v = randomLang(profile)
			}
		case "Sec-Ch-Ua":
			if dyn, ok := secCH["Sec-Ch-Ua"]; ok && v != "" {
				v = dyn
			}
		case "Sec-Ch-Ua-Mobile":
			if dyn, ok := secCH["Sec-Ch-Ua-Mobile"]; ok {
				v = dyn
			}
		case "Sec-Ch-Ua-Platform":
			if dyn, ok := secCH["Sec-Ch-Ua-Platform"]; ok {
				v = dyn
			}
		}
		set(kv.K, v)
	}

	// 4. Extended Sec-CH-UA hints (only sent on Chromium; not in base list)
	if profile.ChromeMajor > 0 {
		for _, k := range []string{
			"Sec-Ch-Ua-Full-Version-List",
			"Sec-Ch-Ua-Full-Version",
			"Sec-Ch-Ua-Arch",
			"Sec-Ch-Ua-Bitness",
			"Sec-Ch-Ua-Platform-Version",
			"Sec-Ch-Ua-Model",
		} {
			if v, ok := secCH[k]; ok {
				set(k, v)
			}
		}
	}

	// 5. Accept-Language (fill if profile didn't include it)
	set("Accept-Language", randomLang(profile))

	// 6. Referer (realistic entry point — not always present)
	ref := pickReferer()
	if ref != "" {
		set("Referer", ref)
	}

	// 7. DNT — ~30% of browsers send it
	if rand.Intn(10) < 3 {
		set("DNT", "1")
	}

	// 8. Save-Data — rare but real (~2% of mobile)
	if profile.Mobile && rand.Intn(50) == 0 {
		set("Save-Data", "on")
	}

	// 9. Viewport-Width hint (Chromium sends this when server opts in)
	if profile.ChromeMajor > 0 {
		pool := viewportDesktop
		if profile.Mobile {
			pool = viewportMobile
		}
		vp := pool[rand.Intn(len(pool))]
		parts := strings.SplitN(vp, "x", 2)
		if len(parts) == 2 {
			set("Viewport-Width", parts[0])
		}
	}

	// 10. X-Forwarded-For randomisation is intentionally NOT done here —
	//     adding a fake XFF looks more suspicious than omitting it.

	return InjectionResult{Profile: profile, Injected: injected}
}

// ─── canonicalHeaderKey wraps net/http for external use ──────────────────────

func canonicalKey(k string) string {
	return http.CanonicalHeaderKey(k)
}

// ─── listProfiles ─────────────────────────────────────────────────────────────

func listProfiles() {
	lines := make([]string, 0, len(Profiles))
	for _, p := range Profiles {
		tag := ""
		if p.Mobile {
			tag = yellow + " mobile" + reset
		}
		lines = append(lines, fmt.Sprintf(
			"%s%-22s%s %s%-10s%s %s%s%s%s",
			cyan, p.Name, reset,
			green, p.Engine, reset,
			dim, truncate(p.UA, 56), reset, tag,
		))
	}
	printBox("Available profiles", lines)
}
