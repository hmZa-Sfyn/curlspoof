package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const version = "1.0.0"

// CLI flags
type Config struct {
	Profile      string // "" = random
	Threads      int
	DelayMs      int
	DelayJitter  int // ± jitter added to delay
	DryRun       bool
	Verbose      bool
	FollowRedirs bool
	TimeoutSec   int
	Retries      int
	OutputFile   string
	NoColor      bool
	SaveCookies  bool
	ProxyURL     string
	InputFile    string // file with multiple curl commands (one per line / block)
	Extract      string // extract mode: links, links-text, images, headings, …
}

func defaultConfig() Config {
	return Config{
		Threads:      1,
		TimeoutSec:   30,
		FollowRedirs: true,
		Retries:      0,
		DelayJitter:  0,
	}
}

func main() {
	cfg := defaultConfig()
	args := os.Args[1:]

	if len(args) == 0 {
		printUsage()
		os.Exit(0)
	}

	var curlTokens []string
	passThroughMode := false

	for i := 0; i < len(args); i++ {
		a := args[i]

		if passThroughMode {
			curlTokens = append(curlTokens, a)
			continue
		}

		nextArg := func() string {
			if i+1 < len(args) {
				i++
				return args[i]
			}
			die("flag %s requires a value", a)
			return ""
		}

		switch a {
		case "--":
			passThroughMode = true
		case "--profile", "-p":
			cfg.Profile = nextArg()
		case "--threads", "-t":
			cfg.Threads, _ = strconv.Atoi(nextArg())
		case "--delay":
			cfg.DelayMs, _ = strconv.Atoi(nextArg())
		case "--jitter":
			cfg.DelayJitter, _ = strconv.Atoi(nextArg())
		case "--timeout":
			cfg.TimeoutSec, _ = strconv.Atoi(nextArg())
		case "--retries":
			cfg.Retries, _ = strconv.Atoi(nextArg())
		case "--proxy":
			cfg.ProxyURL = nextArg()
		case "--output", "-o":
			cfg.OutputFile = nextArg()
		case "--file", "-f":
			cfg.InputFile = nextArg()
		case "--extract", "-e":
			cfg.Extract = nextArg()
		case "--dry-run", "-n":
			cfg.DryRun = true
		case "--verbose", "-v":
			cfg.Verbose = true
		case "--no-redirects":
			cfg.FollowRedirs = false
		case "--no-color":
			cfg.NoColor = true
		case "--save-cookies":
			cfg.SaveCookies = true
		case "--list-profiles":
			listProfiles()
			return
		case "--version":
			fmt.Printf("curlspoof %s\n", version)
			return
		case "--help", "-h":
			printUsage()
			return
		default:
			if strings.HasPrefix(a, "-") {
				die("unknown flag: %s  (try --help)", a)
			}
			curlTokens = append(curlTokens, a)
		}
	}

	if cfg.NoColor {
		disableColor()
	}

	// ── collect raw curl text(s) ──────────────────────────────────────────────

	var rawInputs []string

	// from --file
	if cfg.InputFile != "" {
		rawInputs = append(rawInputs, loadFile(cfg.InputFile)...)
	}

	// from stdin (piped)
	if isPiped() {
		rawInputs = append(rawInputs, readStdin()...)
	}

	// from CLI tokens
	if len(curlTokens) > 0 {
		rawInputs = append(rawInputs, strings.Join(curlTokens, " "))
	}

	if len(rawInputs) == 0 {
		die("no curl command provided.  try: curlspoof -- curl 'https://example.com'")
	}

	// ── run ───────────────────────────────────────────────────────────────────

	printBanner()

	if len(rawInputs) == 1 {
		runSingle(rawInputs[0], cfg)
		return
	}

	// multi-input → concurrent worker pool
	runBatch(rawInputs, cfg)
}

// ─── Batch runner ─────────────────────────────────────────────────────────────

func runBatch(inputs []string, cfg Config) {
	threads := cfg.Threads
	if threads < 1 {
		threads = 1
	}
	if threads > len(inputs) {
		threads = len(inputs)
	}

	type job struct {
		idx int
		raw string
	}
	type result struct {
		idx int
		out string
	}

	jobs := make(chan job, len(inputs))
	results := make(chan result, len(inputs))
	var wg sync.WaitGroup

	// start workers
	for w := 0; w < threads; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				out := captureRun(j.raw, cfg)
				results <- result{idx: j.idx, out: out}
			}
		}()
	}

	// feed jobs
	for i, raw := range inputs {
		jobs <- job{idx: i, raw: raw}
	}
	close(jobs)

	// collect + print in order
	go func() {
		wg.Wait()
		close(results)
	}()

	ordered := make([]string, len(inputs))
	for r := range results {
		ordered[r.idx] = r.out
	}
	for _, out := range ordered {
		fmt.Print(out)
	}
}

// ─── Single request ───────────────────────────────────────────────────────────

func runSingle(raw string, cfg Config) {
	cr, err := parseCurl(raw)
	if err != nil {
		die("parse error: %v", err)
	}

	inj := buildSpoofHeaders(cr, cfg.Profile)

	if cfg.Verbose {
		printVerboseBox(cr, inj)
	} else {
		printMini(inj.Profile)
	}

	if cfg.DryRun {
		printDryRun(cr)
		return
	}

	humanDelay(cfg.DelayMs, cfg.DelayJitter)

	var resp *Response
	for attempt := 0; attempt <= cfg.Retries; attempt++ {
		if attempt > 0 {
			fmt.Printf("%s  ↩ retry %d/%d…%s\n", yellow, attempt, cfg.Retries, reset)
			time.Sleep(time.Duration(500*attempt) * time.Millisecond)
		}
		resp, err = fire(cr, cfg)
		if err == nil {
			break
		}
		fmt.Printf("%s  ✗ %v%s\n", red, err, reset)
	}
	if err != nil {
		os.Exit(1)
	}

	if cfg.Extract != "" {
		extract(cfg.Extract, string(resp.Body), resp.URL)
	} else {
		printResponse(resp, cfg.OutputFile)
	}
}

// captureRun runs a single request and returns its output as a string (for batch).
func captureRun(raw string, cfg Config) string {
	// capture stdout into a string builder
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	runSingle(raw, cfg)

	w.Close()
	os.Stdout = old
	var buf strings.Builder
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		buf.WriteString(scanner.Text() + "\n")
	}
	return buf.String()
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func isPiped() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) == 0
}

func readStdin() []string {
	var lines []string
	scanner := bufio.NewScanner(os.Stdin)
	var block strings.Builder
	for scanner.Scan() {
		l := scanner.Text()
		// blank line separates multiple curl commands in stdin
		if strings.TrimSpace(l) == "" {
			if block.Len() > 0 {
				lines = append(lines, block.String())
				block.Reset()
			}
			continue
		}
		if block.Len() > 0 {
			block.WriteString(" ")
		}
		block.WriteString(l)
	}
	if block.Len() > 0 {
		lines = append(lines, block.String())
	}
	return lines
}

func loadFile(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		die("cannot open file: %v", err)
	}
	defer f.Close()
	var all []string
	scanner := bufio.NewScanner(f)
	var block strings.Builder
	for scanner.Scan() {
		l := scanner.Text()
		if strings.TrimSpace(l) == "" {
			if block.Len() > 0 {
				all = append(all, block.String())
				block.Reset()
			}
			continue
		}
		if block.Len() > 0 {
			block.WriteString(" ")
		}
		block.WriteString(l)
	}
	if block.Len() > 0 {
		all = append(all, block.String())
	}
	return all
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, red+"✗ "+format+reset+"\n", args...)
	os.Exit(1)
}
