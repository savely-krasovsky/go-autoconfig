# Deployment runbook

This runbook deploys the service behind Caddy with automatic HTTPS. The application container is not published directly; only Caddy listens on 80/443.

## Architecture

```
client ──443──> Caddy (TLS, HSTS, redirects)
                  │  X-Forwarded-Proto / Host / For
                  └──8080──> autoconfig (distroless, nonroot, read-only)
```

Caddy routes by `Host`, so one instance serves every domain listed in `config.json`.

## Prerequisites

- Two DNS names per domain: `autoconfig.<domain>` and `autodiscover.<domain>`.
- Ports 80 and 443 reachable from the internet (ACME HTTP-01 and clients).
- A working mail server with IMAP/SMTP ports.
- Docker with Compose.
- A registry you control. Prebuilt images are not distributed: build from this repository and push to your registry; deployments pin the resulting digest.

## DNS

For each zone replace `example.com` and the targets:

| Record | Value |
|---|---|
| `autoconfig.example.com` | `A`/`AAAA` to the proxy (or `CNAME`) |
| `autodiscover.example.com` | `A`/`AAAA` to the proxy (or `CNAME`) |
| `example.com` | `A`/`AAAA` to the proxy if `/.well-known` is served here |
| `_autodiscover._tcp.example.com` | `SRV 0 0 443 autodiscover.example.com.` |
| `_imaps._tcp.example.com` | `SRV 0 1 993 imap.example.com.` |
| `_submission._tcp.example.com` | `SRV 0 1 587 smtp.example.com.` |
| `_submissions._tcp.example.com` | `SRV 0 1 465 smtp.example.com.` |

Notes:

- `_autodiscover._tcp` is used by Outlook; `_imaps`/`_submission(s)` follow RFC 6186/8314.
- If the bare domain already hosts a website, do **not** move it to this service. Add only a reverse-proxy rule for `/.well-known/autoconfig/*` on the existing site (see the Caddyfile comment below).
- DNSSEC and TLSA are recommended but optional.

Verify:

```sh
scripts/check_dns.sh example.com
```

## Configuration

```sh
cd deploy
cp config.example.json config.json
$EDITOR config.json
```

Checklist:

- every domain has `imap` and `smtp` with host/port;
- `starttls: true` for 143/587, `false` for 993/465 (or set `socket_type` explicitly);
- `display_name` is optional;
- `rate_limit_per_minute` is per client IP; `0` disables it; adjust for NAT;
- `trust_proxy: true` only because Caddy overwrites `X-Forwarded-For`;
- `enable_apple: true` only if the static `user@<domain>` profile is wanted.

## Caddy

Edit `deploy/Caddyfile`:

- replace `example.com` / `example.org` with real domains;
- set `ACME_EMAIL`;
- if the bare domain is also a website, restrict its block to `.well-known`:

```caddyfile
example.com {
	handle /.well-known/autoconfig/* {
		reverse_proxy autoconfig:8080
	}
	# existing site configuration...
}
```

Caddy obtains certificates automatically and redirects HTTP to HTTPS. The app also emits HSTS when it sees `X-Forwarded-Proto: https`.

## Start

```sh
cd deploy
docker compose up -d --build
docker compose logs -f caddy
```

Wait until Caddy reports certificate issuance for every host.

## Verify

```sh
DOMAIN=example.com scripts/smoke.sh
DOMAIN=example.com scripts/check_dns.sh
scripts/check_tls.sh autoconfig.example.com
```

`smoke.sh` accepts `AUTOCONFIG_URL`, `APEX_URL`, `AUTOCONFIG_HOST`, `APEX_HOST`, `EXPECT_APPLE` and `CURL_OPTS=-k` for local/self-signed checks.

The landing page at `https://autoconfig.<domain>/` shows the manual IMAP/SMTP settings and the project version; it is covered by the smoke script.

## Microsoft RCA

Before switching traffic:

1. Open <https://testconnectivity.microsoft.com>.
2. Run **Outlook Autodiscover** for a real mailbox.
3. Confirm the service returns the expected IMAP/SMTP settings and a valid certificate chain.
4. Record the result in `docs/compat.md`.

RCA is Exchange-oriented: for an IMAP provider it validates transport, DNS, certificate and XML, but not the final account creation in Outlook.

## Manual acceptance

Follow `docs/compat.md`. Minimum before cutover:

- Outlook 2016/2019/365 desktop: new profile with email+password only;
- Thunderbird: autoconfig flow;
- Apple Mail/iOS (if `enable_apple`);
- one Android client (K-9/FairEmail).

## Rollback

1. Keep the old Node service running until acceptance is complete.
2. Switch DNS/LB gradually: one domain first.
3. To roll back, point the `autoconfig`/`autodiscover` records back to the old service.
4. `docker compose down` on the new stack; keep Caddy volumes for certificates (`caddy_data`).

## Security notes

- Expose only 80/443; the app port stays on the internal Docker network.
- `trust_proxy: true` is safe only when Caddy is the first trusted hop and sets `X-Forwarded-For: {remote_host}` (as in the shipped Caddyfile).
- Keep the container read-only with `cap_drop: ALL`, `no-new-privileges` and resource limits (already in `docker-compose.yml`).
- Logs contain method/host/path/status/duration/request_id and never email or query strings.
- Rate limiting is defense in depth; add ingress-level limits as well if the service is heavily exposed.
