<p align="center">
    <img src="public/logo.svg" width="100px">
</p>

<h1 align="center">Ladderflare</h1>

[Ladder](https://github.com/everywall/ladder) is a HTTP web proxy, designed to bypass web restrictions and remove CORS headers.

Ladderflare is pipeline for Ladder:

1. Ladderflare compiles the [Golang source code](https://github.com/everywall/ladder) into WebAssembly.
2. The javascript wrapper `index.js` loads and delegates to the WebAssembly module.
3. Static assets `index.html` and `style.css` serve an updated interface.
4. [Domain-specific rules]((https://github.com/everywall/ladder/) are downloaded and compiled into the build.
5. The complete package is deployed to Cloudflare Workers, with support for user-friendly support for [Deploy to Cloudflare](https://deploy.workers.cloudflare.com/?url=https://github.com/andesco/ladderflare).

The result is a fast, globally distributed HTTP proxy that runs entirely on the Cloudflare edge network.

## Deploy to Cloudflare

### Cloudflare Dashboard

[![Deploy to Cloudflare](https://deploy.workers.cloudflare.com/button)](https://deploy.workers.cloudflare.com/?url=https://github.com/andesco/ladderflare)

<nobr>Workers & Pages</nobr> ⇢ Create an application ⇢ [Clone a repository](https://dash.cloudflare.com/?to=/:account/workers-and-pages/create/deploy-to-workers): \
   `http://github.com/andesco/ladderflare`

### Wrangler CLI
   
```bash
git clone https://github.com/andesco/ladderflare.git
cd ladderflare
npm install
npm run build
wrangler deploy
```
   
## Usage

Visit your worker and enter a URL: \
`https://ladder.{subdomain}.workers.dev`

Directly appended a URL to the end of Ladderflare’s hostname: \
`https://ladder.{subdomain}.workers.dev/https://example.com`

Create a [bookmarklet](https://wikipedia.org/wiki/Bookmarklet) with the following URL: \
`javascript:window.location.href="https://ladder.{subdomain}.workers.dev/"+location.href`

Save this bookmarklet (will prompt for your domain): \
[Open with Ladder](<javascript:var domain=prompt("Enter your Ladder domain (e.g., ladder.example.workers.dev):");if(domain)window.location.href="https://"+domain+"/"+location.href>)

## Configuration

The worker is configured using environment variables. You can set these in your `wrangler.toml` file or in the Cloudflare dashboard.

| Variable | Description | Value |
| --- | --- | --- |
| `USER_AGENT` | User agent to emulate | `Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)` |
| `X_FORWARDED_FOR` | IP forwarder address | `66.249.66.1` |
| `LOG_URLS` | Log fetched URL's | `true` |
| `DISABLE_FORM` | Disables URL Form Frontpage | `false` |
| `FORM_PATH` | Path to custom Form HTML |  |
| `RULESET` | Path or URL to a ruleset file, accepts local directories | `https://raw.githubusercontent.com/everywall/ladder-rules/main/ruleset.yaml` |
| `EXPOSE_RULESET` | Make your Ruleset available to other ladders | `true` |
| `ALLOWED_DOMAINS` | Comma separated list of allowed domains. (no limitations when not set) |  |
| `ALLOWED_DOMAINS_RULESET` | Allow Domains from Ruleset. (no limitations when `false` | `false` |

`ALLOWED_DOMAINS` and `ALLOWED_DOMAINS_RULESET` are joined together. If both are empty, no limitations are applied.



## Development

```bash
npm run init # initialize
npm run build # builds WASM and generates usable rules
npm run deploy # wrangler deploy
npm run deploy:local # wrangler deploy --config wrangler.local.toml
npm run dev # wrangler dev --local --config wrangler.local.toml
```

**Note**: Test URLs for the `/test` endpoint are sourced from the [everywall/ladder-rules](https://github.com/everywall/ladder-rules) repository (stored locally in `@ruleset.yaml`), since the main everywall/ladder repository does not contain test URLs with complete article paths.
### Interface Updates

- `form.html` has been renamed to `index.html`
   - add Apple Shortcut
   - save bookmarklet
- `styles.css` is served with dependencies
- `/test` endpoint

### Endpoints

**TEST**: `https://ladder.{subdomain}.workers.dev/test`

**API**: `curl -X GET "{subdomain}.workers.dev/api/https://www.example.com"`

**RAW:** `https://ladder.{subdomain}.workers.dev/raw/https://www.example.com`

**Running Ruleset**: `https://ladder.{subdomain}.workers.dev/ruleset`

&zwnj;

---

Ladderflare is licensed under the [MIT License](LICENSE).
