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
		sp := w - 2 - 7
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
	fmt.Printf("\n%s%s curlspoof%s %sv%s%s\n\n",
		bold, cyan, reset, dim, version, reset)
}

// ─── Mini spoof line (non-verbose) ────────────────────────────────────────────

func printMini(p *BrowserProfile) {
	fmt.Printf("%s◈ profile  %s%s%-22s%s  %s%s%s\n",
		purple, reset,
		cyan, p.Name, reset,
		dim, truncate(p.UA, 55), reset,
	)
}

// ─── Verbose box ─────────────────────────────────────────────────────────────

func printVerboseBox(cr *CurlRequest, inj InjectionResult) {
	printMini(inj.Profile)

	// original headers box
	origLines := make([]string, 0, len(cr.headerOrder))
	for _, k := range cr.headerOrder {
		v := cr.Headers[k]
		// mark injected headers with a symbol
		injected := false
		for _, kv := range inj.Injected {
			if kv.K == k {
				injected = true
				break
			}
		}
		if injected {
			origLines = append(origLines, fmt.Sprintf(
				"%s+%s %s%s%s: %s%s%s",
				green, reset, cyan, k, reset, dim, truncate(v, 60), reset,
			))
		} else {
			origLines = append(origLines, fmt.Sprintf(
				"  %s%s%s: %s%s%s",
				cyan, k, reset, dim, truncate(v, 60), reset,
			))
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
		v := cr.Headers[k]
		fmt.Printf(" \\\n  -H %s'%s: %s'%s", dim, k, v, reset)
	}
	if cr.Body != "" {
		bodyDisplay := truncate(cr.Body, 120)
		fmt.Printf(" \\\n  -d %s'%s'%s", dim, bodyDisplay, reset)
	}
	fmt.Printf("\n\n")
}

// ─── Usage ────────────────────────────────────────────────────────────────────

func printUsage() {
	fmt.Fprintf(os.Stdout, `
%s%scurlspoof%s  v%s  — inject a browser fingerprint into any curl command

%sUsage:%s
  curlspoof [options] -- curl -X GET 'https://…' -H '…'
  curlspoof [options] "curl -X GET 'https://…' -H '…'"
  echo "curl …" | curlspoof [options]
  curlspoof -f requests.txt [options]        %s# batch — one curl per block%s

%sOptions:%s
  %s-p%s / %s--profile%s <n>    browser profile to use       %s(default: random)%s
  %s-t%s / %s--threads%s  <n>   worker threads for batch     %s(default: 1)%s
       %s--delay%s    <ms>   wait N ms before firing      %s(default: 0)%s
       %s--jitter%s   <ms>   ± random jitter on delay
       %s--timeout%s  <s>    request timeout in seconds   %s(default: 30)%s
       %s--retries%s  <n>    retry N times on failure     %s(default: 0)%s
       %s--proxy%s    <url>  HTTP/SOCKS proxy URL
  %s-o%s / %s--output%s   <f>   save response body to file
  %s-f%s / %s--file%s     <f>   read curl command(s) from file
  %s-n%s / %s--dry-run%s        print final curl, do not fire
  %s-v%s / %s--verbose%s        show injected headers
       %s--no-redirects%s       do not follow redirects
       %s--no-color%s           disable ANSI colours
       %s--save-cookies%s       persist cookies across batch
       %s--list-profiles%s      list all browser profiles
       %s--version%s
       %s--help%s

%sProfiles:%s
`,
		bold, cyan, reset, version,
		bold, reset,
		dim, reset,
		bold, reset,
		cyan, reset, cyan, reset, dim, reset,
		cyan, reset, cyan, reset, dim, reset,
		cyan, reset, dim, reset,
		cyan, reset,
		cyan, reset, dim, reset,
		cyan, reset, dim, reset,
		cyan, reset,
		cyan, reset, cyan, reset,
		cyan, reset, cyan, reset,
		cyan, reset, cyan, reset,
		cyan, reset,
		cyan, reset,
		cyan, reset,
		cyan, reset,
		cyan, reset,
		cyan, reset,
		bold, reset,
	)

	for _, p := range Profiles {
		fmt.Printf("  %s%-22s%s %s%-10s%s %s%s%s\n",
			cyan, p.Name, reset,
			yellow, p.Engine, reset,
			dim, truncate(p.UA, 58), reset)
	}

	fmt.Printf(`
%sExamples:%s
  # basic spoof
  curlspoof -- curl 'https://www.amazon.com'

  # your exact supabase request — with spoof added automatically
  curlspoof -- curl -X GET \
    'https://project.supabase.co/rest/v1/table?select=*' \
    -H 'apikey: YOUR_KEY' \
    -H 'Authorization: Bearer YOUR_TOKEN' \
    -H 'Accept: application/json'

  # pin Firefox, see what was injected
  curlspoof -p firefox-125-linux -v -- curl 'https://httpbin.org/headers'

  # dry-run: print the final curl command without firing
  curlspoof -n -- curl -H 'Accept: application/json' 'https://example.com'

  # batch: 4 threads, 200-500ms jitter delay between requests
  curlspoof -f requests.txt -t 4 --delay 200 --jitter 300

  # pipe
  echo "curl -X DELETE 'https://api.example.com/item/1' -H 'Auth: Bearer X'" | curlspoof

`, bold, reset)
}
