package main

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

// ─── ANSI ─────────────────────────────────────────────────────────────────────

var (
	reset  = "\033[0m"
	bold   = "\033[1m"
	dim    = "\033[2m"
	cyan   = "\033[36m"
	green  = "\033[32m"
	yellow = "\033[33m"
	red    = "\033[31m"
	blue   = "\033[34m"
	gray   = "\033[90m"
	purple = "\033[35m"
)

func disableColor() {
	reset = ""
	bold = ""
	dim = ""
	cyan = ""
	green = ""
	yellow = ""
	red = ""
	blue = ""
	gray = ""
	purple = ""
}

// ─── Box ──────────────────────────────────────────────────────────────────────

func visLen(s string) int {
	inEsc := false
	n := 0
	for _, r := range s {
		if r == '\033' {
			inEsc = true
		} else if inEsc {
			if r == 'm' {
				inEsc = false
			}
		} else {
			n++
		}
	}
	return n
}

func printBox(title string, lines []string) {
	allLines := append([]string{title}, lines...)
	w := 34
	for _, l := range allLines {
		if v := visLen(l) + 4; v > w {
			w = v
		}
	}
	titleRunes := utf8.RuneCountInString(title)
	pad := w - titleRunes - 3
	if pad < 1 {
		pad = 1
	}
	fmt.Println("╭─ " + bold + cyan + title + reset + " " + strings.Repeat("─", pad) + "╮")
	if len(lines) == 0 {
		sp := w - 9
		if sp < 0 {
			sp = 0
		}
		fmt.Printf("│ %s(empty)%s%s │\n", dim, reset, strings.Repeat(" ", sp))
	}
	for _, l := range lines {
		sp := w - 2 - visLen(l)
		if sp < 0 {
			sp = 0
		}
		fmt.Printf("│ %s%s │\n", l, strings.Repeat(" ", sp))
	}
	fmt.Println("╰" + strings.Repeat("─", w) + "╯")
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func statusColor(code int) string {
	switch {
	case code >= 200 && code < 300:
		return green
	case code >= 300 && code < 400:
		return yellow
	case code >= 400:
		return red
	}
	return red + bold
}

// ─── Banner ───────────────────────────────────────────────────────────────────

func printBanner() {
	fmt.Printf("\n%s%s curlspoof%s %sv%s%s\n\n", bold, cyan, reset, dim, version, reset)
}

// ─── Mini spoof line ──────────────────────────────────────────────────────────

func printMini(p *BrowserProfile) {
	fmt.Printf("%s◈ profile  %s%s%-22s%s  %s%s%s\n",
		purple, reset, cyan, p.Name, reset, dim, truncate(p.UA, 55), reset)
}

// ─── Verbose box ─────────────────────────────────────────────────────────────

func printVerboseBox(cr *CurlRequest, inj InjectionResult) {
	printMini(inj.Profile)
	origLines := make([]string, 0, len(cr.headerOrder))
	for _, k := range cr.headerOrder {
		v := cr.Headers[k]
		injected := false
		for _, kv := range inj.Injected {
			if kv.K == k {
				injected = true
				break
			}
		}
		if injected {
			origLines = append(origLines, fmt.Sprintf("%s+%s %s%s%s: %s%s%s",
				green, reset, cyan, k, reset, dim, truncate(v, 60), reset))
		} else {
			origLines = append(origLines, fmt.Sprintf("  %s%s%s: %s%s%s",
				cyan, k, reset, dim, truncate(v, 60), reset))
		}
	}
	printBox(fmt.Sprintf("Final headers  (%s+%s = injected)", green, reset), origLines)
}

// ─── Dry-run ──────────────────────────────────────────────────────────────────

func printDryRun(cr *CurlRequest) {
	fmt.Printf("\n%s%s# dry run — final curl command%s\n", bold, gray, reset)
	fmt.Printf("%scurl%s -X %s%s%s \\\n", cyan, reset, yellow, cr.Method, reset)
	fmt.Printf("  %s'%s'%s", green, cr.URL, reset)
	for _, k := range cr.headerOrder {
		fmt.Printf(" \\\n  -H %s'%s: %s'%s", dim, k, cr.Headers[k], reset)
	}
	if cr.Body != "" {
		fmt.Printf(" \\\n  -d %s'%s'%s", dim, truncate(cr.Body, 120), reset)
	}
	fmt.Printf("\n\n")
}

// ─── Usage ────────────────────────────────────────────────────────────────────

func printUsage() {
	w := os.Stdout
	nl := func() { fmt.Fprintln(w) }
	h := func(s string) { fmt.Fprintln(w, bold+s+reset) }
	opt := func(flag, desc, def string) {
		suf := ""
		if def != "" {
			suf = fmt.Sprintf("  %s(default: %s)%s", dim, def, reset)
		}
		fmt.Fprintf(w, "  %s%-28s%s %s%s\n", cyan, flag, reset, desc, suf)
	}
	ext := func(mode, desc string) {
		fmt.Fprintf(w, "  %s%-16s%s %s\n", cyan, mode, reset, desc)
	}
	ex := func(s string) { fmt.Fprintln(w, "  "+s) }

	nl()
	fmt.Fprintf(w, "%s%scurlspoof%s  v%s  — inject a browser fingerprint into any curl command\n",
		bold, cyan, reset, version)
	nl()

	h("Usage:")
	ex("curlspoof [options] -- curl -X GET 'https://…' -H '…'")
	ex("curlspoof [options] \"curl -X GET 'https://…' -H '…'\"")
	ex("echo \"curl …\" | curlspoof [options]")
	ex(fmt.Sprintf("curlspoof -f requests.txt [options]    %s# batch — one curl per block%s", dim, reset))
	nl()

	h("Options:")
	opt("-p / --profile <n>",   "browser profile",              "random")
	opt("-t / --threads <n>",   "worker threads for batch",     "1")
	opt("--delay <ms>",          "wait N ms before firing",      "0")
	opt("--jitter <ms>",         "± random ms on top of delay",  "")
	opt("--timeout <s>",         "request timeout in seconds",   "30")
	opt("--retries <n>",         "retry N times on failure",     "0")
	opt("--proxy <url>",         "HTTP/SOCKS proxy URL",         "")
	opt("-o / --output <file>",  "save body to file",            "")
	opt("-f / --file <file>",    "read curl commands from file", "")
	opt("-n / --dry-run",        "print final curl, don't fire", "")
	opt("-v / --verbose",        "show injected headers",        "")
	opt("-e / --extract <mode>", "extract elements from HTML",   "")
	opt("--no-redirects",        "do not follow redirects",      "")
	opt("--no-color",            "disable ANSI colours",         "")
	opt("--save-cookies",        "persist cookies across batch", "")
	opt("--list-profiles",       "list all browser profiles",    "")
	opt("--version",             "",                             "")
	opt("--help",                "",                             "")
	nl()

	h("Extract modes  (-e / --extract):")
	ext("links",      "all <a href> URLs (deduplicated)")
	ext("links-text", "<a href> + visible anchor text, side by side")
	ext("images",     "all <img src> URLs")
	ext("headings",   "all <h1>–<h6> text content")
	ext("title",      "<title> tag content")
	ext("text",       "visible text, tags stripped")
	ext("forms",      "<form> actions and every input/select/textarea field")
	ext("scripts",    "all <script src> values")
	ext("meta",       "all <meta name/property + content> pairs")
	nl()

	h("Profiles:")
	for _, pr := range Profiles {
		fmt.Fprintf(w, "  %s%-22s%s %s%-10s%s %s%s%s\n",
			cyan, pr.Name, reset,
			yellow, pr.Engine, reset,
			dim, truncate(pr.UA, 58), reset)
	}
	nl()

	h("Examples:")
	ex("# extract all links from a page")
	ex("curlspoof -e links -- curl 'https://news.ycombinator.com'")
	nl()
	ex("# links + anchor text from Amazon")
	ex("curlspoof -e links-text -- curl 'https://www.amazon.com'")
	nl()
	ex("# basic spoof, no extraction")
	ex("curlspoof -- curl 'https://www.amazon.com'")
	nl()
	ex("# pin Firefox + see what was injected")
	ex("curlspoof -p firefox-125-linux -v -- curl 'https://httpbin.org/headers'")
	nl()
	ex("# dry-run")
	ex("curlspoof -n -- curl -H 'Accept: application/json' 'https://example.com'")
	nl()
	ex("# batch: 4 threads, 200–500 ms jitter")
	ex("curlspoof -f requests.txt -t 4 --delay 200 --jitter 300")
	nl()
	ex("# pipe")
	ex("echo \"curl 'https://httpbin.org/get'\" | curlspoof -e links")
	nl()
}
