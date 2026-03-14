# curlspoof

Paste any `curl` command and get it fired with a **full browser fingerprint** injected — User-Agent, all `Sec-Fetch-*` / `Sec-CH-UA` headers, `Accept-Language`, `Referer`, and more.  
Built for research and testing. Zero external dependencies. Pure Go stdlib.

```
curl https://www.amazon.com        → 503 / bot page
curlspoof -- curl https://www.amazon.com   → 200 OK
```

## Build

```bash
go build -o curlspoof .
# optionally install globally
go install .
```

## Quick start

```bash
# bare URL
curlspoof -- curl 'https://www.amazon.com'

# your supabase request — spoof injected automatically around your headers
curlspoof -- curl -X GET \
  'https://project.supabase.co/rest/v1/notifications?select=*' \
  -H 'apikey: YOUR_KEY' \
  -H 'Authorization: Bearer YOUR_TOKEN' \
  -H 'Accept: application/json'

# single string (copy-paste from terminal)
curlspoof "curl -X GET 'https://httpbin.org/headers' -H 'Accept: application/json'"

# pipe
echo "curl https://httpbin.org/get" | curlspoof

# show every injected header
curlspoof -v -- curl 'https://httpbin.org/headers'

# dry-run: print the final curl command, don't fire
curlspoof -n -- curl 'https://example.com'

# pin to Firefox on Linux
curlspoof -p firefox-125-linux -- curl 'https://example.com'

# batch file, 4 threads, 200–500 ms human delay
curlspoof -f requests.txt -t 4 --delay 200 --jitter 300
```

## All options

| Flag | Default | Description |
|------|---------|-------------|
| `-p` / `--profile <n>` | random | Pin browser profile |
| `-t` / `--threads <n>` | 1 | Worker threads (batch mode) |
| `--delay <ms>` | 0 | Wait N ms before each request |
| `--jitter <ms>` | 0 | ± random ms added to delay |
| `--timeout <s>` | 30 | Request timeout |
| `--retries <n>` | 0 | Retry N times on failure |
| `--proxy <url>` | — | HTTP / SOCKS5 proxy |
| `-o` / `--output <f>` | — | Save response body to file |
| `-f` / `--file <f>` | — | Read curl commands from file |
| `-n` / `--dry-run` | off | Print final curl, don't fire |
| `-v` / `--verbose` | off | Show injected headers in a box |
| `--no-redirects` | off | Do not follow redirects |
| `--no-color` | off | Disable ANSI colours |
| `--list-profiles` | — | Print all profiles and exit |

## Profiles

| Profile | Engine | OS |
|---------|--------|----|
| `chrome-124-win` | Chromium | Windows |
| `chrome-124-mac` | Chromium | macOS |
| `chrome-124-android` | Chromium | Android |
| `firefox-125-win` | Gecko | Windows |
| `firefox-125-linux` | Gecko | Linux |
| `safari-17-mac` | WebKit | macOS |
| `safari-17-ios` | WebKit | iOS |
| `edge-124-win` | Chromium | Windows |

## What gets injected (only fills headers not already set)

| Header | Detail |
|--------|--------|
| `User-Agent` | Real browser UA with randomised minor version |
| `Accept` | Per-browser exact accept string |
| `Accept-Encoding` | `gzip, deflate, br, zstd` |
| `Accept-Language` | Rotated from a pool of real locale strings |
| `Cache-Control` | `max-age=0` (Chromium profiles) |
| `Upgrade-Insecure-Requests` | `1` |
| `Sec-Fetch-Dest/Mode/Site/User` | Full set |
| `Sec-CH-UA` | Client hints with version matching UA |
| `Sec-CH-UA-Mobile` | `?0` / `?1` matching device |
| `Sec-CH-UA-Platform` | `"Windows"`, `"macOS"`, etc. |
| `Priority` | `u=0, i` (Chromium only) |
| `Referer` | Realistic entry point (Google, Bing, Reddit…) |
| `DNT` | Randomly 1-in-3 requests |
| `TE: trailers` | Firefox only |

## curl flags parsed

`-X` `--request` `-H` `--header` `-d` `--data` `--data-raw` `--data-binary`
`--data-urlencode` `--json` `-u` `--user` `-A` `--user-agent` `-e` `--referer`
`-b` `--cookie` `--url` `-F` `--form` `-L` `-s` `-v` `-i` `-I` `-k`
`--compressed` `--http1.1` `--http2` `-o` `-w` `-m` `--max-time`
`--connect-timeout` `-x` `--proxy` `--retry` `--cacert` and more.

## Batch file format

One curl command per block, separated by blank lines:

```
curl -X GET 'https://api.example.com/users' -H 'Authorization: Bearer TOKEN'

curl -X POST 'https://api.example.com/items' \
  -H 'Content-Type: application/json' \
  -d '{"name":"test"}'

curl 'https://httpbin.org/get'
```

## Why Amazon / Cloudflare blocks plain curl

They check multiple signals simultaneously:

1. **User-Agent** — `curl/x.x.x` is instantly flagged
2. **Missing browser headers** — no `Accept`, `Sec-Fetch-*`, `Sec-CH-UA`
3. **Header order** — bots send headers in non-browser order
4. **TLS fingerprint (JA3)** — Go's TLS stack differs from Chrome's
5. **HTTP/2 fingerprint** — browser SETTINGS frames differ from Go's
6. **Missing `Accept-Encoding`** — browsers always send this
7. **Referer chain** — missing `Referer` on non-first page loads

`curlspoof` handles 1–3 and 6–7. For sites with advanced TLS/H2 fingerprinting
(Cloudflare Enterprise), additional tools like `curl-impersonate` (which patches
the TLS stack) are needed.
