package main

import (
	"fmt"
	"net/http"
	"strings"
	"unicode"
)

// CurlRequest is a parsed curl command.
type CurlRequest struct {
	Method      string
	URL         string
	Headers     map[string]string // canonical → value
	headerOrder []string          // canonical keys in insertion order
	Body        string
	BodyIsForm  bool // set when --data-urlencode / application/x-www-form-urlencoded
}

// ─── Tokeniser ────────────────────────────────────────────────────────────────
// Handles: single quotes, double quotes with \-escapes, backslash-newline
// continuations, and regular whitespace splitting.

func shellTokens(raw string) []string {
	// join backslash-newline continuations first
	raw = strings.ReplaceAll(raw, "\\\r\n", " ")
	raw = strings.ReplaceAll(raw, "\\\n", " ")

	var tokens []string
	var cur strings.Builder
	inSingle := false
	inDouble := false
	runes := []rune(raw)

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case inSingle:
			if r == '\'' {
				inSingle = false
			} else {
				cur.WriteRune(r)
			}
		case inDouble:
			if r == '"' {
				inDouble = false
			} else if r == '\\' && i+1 < len(runes) {
				i++
				switch runes[i] {
				case 'n':
					cur.WriteRune('\n')
				case 't':
					cur.WriteRune('\t')
				case '"':
					cur.WriteRune('"')
				case '\\':
					cur.WriteRune('\\')
				default:
					cur.WriteRune('\\')
					cur.WriteRune(runes[i])
				}
			} else {
				cur.WriteRune(r)
			}
		case r == '\'':
			inSingle = true
		case r == '"':
			inDouble = true
		case unicode.IsSpace(r):
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

// ─── Parser ───────────────────────────────────────────────────────────────────

// parseCurl turns a raw curl command string (with or without leading "curl")
// into a CurlRequest.
func parseCurl(raw string) (*CurlRequest, error) {
	tokens := shellTokens(strings.TrimSpace(raw))
	if len(tokens) == 0 {
		return nil, fmt.Errorf("empty input")
	}

	// strip leading "curl"
	if strings.ToLower(tokens[0]) == "curl" {
		tokens = tokens[1:]
	}
	if len(tokens) == 0 {
		return nil, fmt.Errorf("no URL in curl command")
	}

	cr := &CurlRequest{
		Method:  "GET",
		Headers: make(map[string]string),
	}

	for i := 0; i < len(tokens); i++ {
		t := tokens[i]

		// pull the next token (value for this flag)
		peek := func() (string, bool) {
			if i+1 < len(tokens) {
				i++
				return tokens[i], true
			}
			return "", false
		}

		switch {

		// ── method ────────────────────────────────────────────────────────────
		case t == "-X" || t == "--request":
			if v, ok := peek(); ok {
				cr.Method = strings.ToUpper(v)
			}

		// ── headers ───────────────────────────────────────────────────────────
		case t == "-H" || t == "--header":
			if v, ok := peek(); ok {
				parseHeader(cr, v)
			}

		// ── body ──────────────────────────────────────────────────────────────
		case t == "-d" || t == "--data" || t == "--data-raw" || t == "--data-binary":
			if v, ok := peek(); ok {
				appendBody(cr, v)
			}
			if cr.Method == "GET" || cr.Method == "HEAD" {
				cr.Method = "POST"
			}

		case t == "--data-urlencode":
			if v, ok := peek(); ok {
				appendBody(cr, v)
			}
			cr.BodyIsForm = true
			if cr.Method == "GET" || cr.Method == "HEAD" {
				cr.Method = "POST"
			}

		case t == "--json":
			if v, ok := peek(); ok {
				appendBody(cr, v)
			}
			cr.setHeader("Content-Type", "application/json")
			cr.setHeader("Accept", "application/json")
			if cr.Method == "GET" || cr.Method == "HEAD" {
				cr.Method = "POST"
			}

		// ── basic auth ────────────────────────────────────────────────────────
		case t == "-u" || t == "--user":
			if v, ok := peek(); ok {
				cr.setHeader("Authorization", "Basic "+b64Encode(v))
			}

		// ── user-agent ────────────────────────────────────────────────────────
		case t == "-A" || t == "--user-agent":
			if v, ok := peek(); ok {
				cr.setHeader("User-Agent", v)
			}

		// ── referer ───────────────────────────────────────────────────────────
		case t == "-e" || t == "--referer" || t == "--referrer":
			if v, ok := peek(); ok {
				cr.setHeader("Referer", v)
			}

		// ── cookies ───────────────────────────────────────────────────────────
		case t == "-b" || t == "--cookie":
			if v, ok := peek(); ok {
				cr.setHeader("Cookie", v)
			}

		// ── explicit URL ──────────────────────────────────────────────────────
		case t == "--url":
			if v, ok := peek(); ok {
				cr.URL = cleanURL(v)
			}

		// ── form fields ───────────────────────────────────────────────────────
		case t == "-F" || t == "--form" || t == "--form-string":
			// multipart form — just note that body is form data
			if v, ok := peek(); ok {
				if cr.Body != "" {
					cr.Body += "&" + v
				} else {
					cr.Body = v
				}
			}
			cr.BodyIsForm = true
			if cr.Method == "GET" {
				cr.Method = "POST"
			}

		// ── ignored flags ─────────────────────────────────────────────────────
		case t == "-L" || t == "--location":
		case t == "-s" || t == "--silent":
		case t == "-S" || t == "--show-error":
		case t == "-v" || t == "--verbose":
		case t == "-i" || t == "--include":
		case t == "-I" || t == "--head":
			cr.Method = "HEAD"
		case t == "-k" || t == "--insecure":
		case t == "--compressed":
		case t == "--http1.0":
		case t == "--http1.1":
		case t == "--http2":
		case t == "--http3":
		case t == "-g" || t == "--globoff":

		// flags that consume a value but we ignore
		case t == "-o" || t == "--output":
			peek()
		case t == "-w" || t == "--write-out":
			peek()
		case t == "--connect-timeout":
			peek()
		case t == "-m" || t == "--max-time":
			peek()
		case t == "-x" || t == "--proxy":
			peek()
		case t == "--proxy-user":
			peek()
		case t == "-r" || t == "--range":
			peek()
		case t == "-T" || t == "--upload-file":
			peek()
		case t == "--limit-rate":
			peek()
		case t == "--max-redirs":
			peek()
		case t == "--retry":
			peek()
		case t == "--cacert" || t == "--capath" || t == "--cert" || t == "--key":
			peek()

		// bare URL, -H"Key: Val" (no space), or unknown flag
		default:
			if strings.HasPrefix(t, "-H") && len(t) > 2 {
				parseHeader(cr, t[2:])
			} else if !strings.HasPrefix(t, "-") && cr.URL == "" {
				cr.URL = cleanURL(t)
			}
		}
	}

	if cr.URL == "" {
		return nil, fmt.Errorf("no URL found")
	}
	if cr.BodyIsForm && cr.getHeader("Content-Type") == "" {
		cr.setHeader("Content-Type", "application/x-www-form-urlencoded")
	}

	return cr, nil
}

// ─── CurlRequest helpers ──────────────────────────────────────────────────────

func (cr *CurlRequest) setHeader(k, v string) {
	ck := http.CanonicalHeaderKey(k)
	if _, exists := cr.Headers[ck]; !exists {
		cr.headerOrder = append(cr.headerOrder, ck)
	}
	cr.Headers[ck] = v
}

func (cr *CurlRequest) hasHeader(k string) bool {
	_, ok := cr.Headers[http.CanonicalHeaderKey(k)]
	return ok
}

func (cr *CurlRequest) getHeader(k string) string {
	return cr.Headers[http.CanonicalHeaderKey(k)]
}

func parseHeader(cr *CurlRequest, raw string) {
	idx := strings.Index(raw, ":")
	if idx < 0 {
		// header with no value (e.g., "X-Empty;")
		k := strings.TrimRight(strings.TrimSpace(raw), ";")
		cr.setHeader(k, "")
		return
	}
	cr.setHeader(strings.TrimSpace(raw[:idx]), strings.TrimSpace(raw[idx+1:]))
}

func appendBody(cr *CurlRequest, v string) {
	v = strings.TrimPrefix(v, "@") // @filename — we keep as literal
	if cr.Body == "" {
		cr.Body = v
	} else {
		cr.Body += "&" + v
	}
}

func cleanURL(u string) string {
	u = strings.TrimSpace(u)
	if !strings.Contains(u, "://") {
		return "https://" + u
	}
	return u
}

// ─── base64 (stdlib-only, no import needed) ───────────────────────────────────

const b64chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

func b64Encode(s string) string {
	data := []byte(s)
	var b strings.Builder
	for i := 0; i < len(data); i += 3 {
		b0 := data[i]
		var b1, b2 byte
		has1 := i+1 < len(data)
		has2 := i+2 < len(data)
		if has1 {
			b1 = data[i+1]
		}
		if has2 {
			b2 = data[i+2]
		}
		b.WriteByte(b64chars[b0>>2])
		b.WriteByte(b64chars[((b0&3)<<4)|(b1>>4)])
		if has1 {
			b.WriteByte(b64chars[((b1&0xf)<<2)|(b2>>6)])
		} else {
			b.WriteByte('=')
		}
		if has2 {
			b.WriteByte(b64chars[b2&0x3f])
		} else {
			b.WriteByte('=')
		}
	}
	return b.String()
}
