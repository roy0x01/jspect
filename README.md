<div align="center">

<img src="logo.svg" width="120" height="120" alt="jspect logo" />

# jspect

**JavaScript formatter and recon scanner for daily terminal use**

![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat-square&logo=go&logoColor=white)
![License](https://img.shields.io/badge/license-MIT-4ade80?style=flat-square)
![Platform](https://img.shields.io/badge/platform-linux%20%7C%20macOS%20%7C%20windows-6B7280?style=flat-square)

</div>

---

`jspect` formats JavaScript with syntax highlighting — including expanding minified/obfuscated bundles into readable code — and inspects JS for endpoints, secrets, and other recon findings using a configurable rule set. Pure Go, zero dependencies.

## Install

```bash
go install github.com/roy0x01/jspect@latest
```

---

## Usage

```
jspect <input.js|url> [-o output.js] [--analyze] [--config <file>]
```

| Command | What it does |
|---|---|
| `jspect app.js` | Format and print with color |
| `jspect app.js -o pretty.js` | Format and save to file |
| `jspect https://site.com/app.js` | Fetch, format, and print |
| `jspect app.js --analyze` | Scan for endpoints, secrets, IPs, and more |
| `jspect --init-config` | Write the default rule set to `~/.jspect/analyze.conf` |

Input is never modified unless `-o` is given.

---

## Formatting

Works on both clean and minified/obfuscated JS — expands single-line bundles into properly indented, readable code.

| Token | Color |
|---|---|
| Keywords | Violet |
| Strings / template literals | Sage green |
| Numbers | Coral |
| Function names | Sky blue |
| Operators | Pink |
| `{ } ( ) [ ]` | Gold |
| Comments | Grey |

`JSON.parse("...")` calls are automatically detected and the JSON inside is pretty-printed.

---

## Recon analysis

```bash
jspect app.js --analyze
```

The default rule set lives in [`analyze.conf`](analyze.conf) at the repo root — a plain text file, easy to read and PR against. It's embedded into the binary at build time, so `go install` works standalone with no extra files needed. On first run, it's copied to `~/.jspect/analyze.conf` covering:

- **Endpoints** — API paths, HTTP client calls (`fetch`, `axios`, `$.ajax`, `.get/.post/...`), template literal paths
- **URLs** — absolute URLs, WebSocket endpoints, S3/Azure/GCS buckets
- **GraphQL** — operations and endpoints
- **Credentials** — JWTs, API keys, passwords, basic auth in URLs
- **Cloud Keys** — AWS, Google, Stripe, Slack, GitHub, SendGrid, Twilio
- **Network** — IPv4 addresses, private ranges
- **Debug** — console statements, TODO/FIXME/hardcoded-credential comments

Findings are grouped by category and sorted by severity (`critical → high → medium → info`).

### Customizing rules

Edit `~/.jspect/analyze.conf` directly — no recompiling needed:

```
name     = Internal Auth Header
category = Custom
severity = high
pattern  = X-Internal-Token:\s*[\w\-]+
```

Use a team-shared config with `--config`:

```bash
jspect app.js --analyze --config ./team-rules.conf
```

---

## Sample file

[`sample.js`](sample.js) — a realistic minified bundle containing endpoints, a JWT, an S3 bucket, GraphQL operations, and hardcoded secrets, for testing both formatting and analysis.

```bash
jspect sample.js
jspect sample.js --analyze
```

---

## License

MIT
