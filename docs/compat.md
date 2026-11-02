# Client compatibility matrix

The manual client matrix was intentionally skipped for `1.0.0`; HTTP-level checks are recorded below. Keep the table for a future release if real-client acceptance is required.

## Build under test

| Field | Value |
|---|---|
| Build / commit | `1.0.0` (linux/amd64, digest-pinned) |
| Date | 2026-10-28 |
| Server / region | three independent sites |
| Config | `domains` map, `enable_apple: false` |
| Certificate issuer | public ACME (valid chain) |
| `enable_apple` | false |

## HTTP-level checks (completed 2026-10-28)

Passed on every site: `GET /` (landing page with manual settings), `GET /healthz`, `GET /version` (`1.0.0`), Thunderbird `config-v1.1.xml`, Outlook POX with `AutoDiscoverSMTPAddress`, unknown schema → 400, unknown host → 404, Apple disabled → 404, `Cache-Control: no-store`, `X-Content-Type-Options: nosniff`, HSTS, body > 1 MiB → 413. No email/query in logs.

Real clients and Outlook Connectivity acceptance remain below.

## Microsoft RCA (2026-10-28)

`SSL Server Test` (available without mailbox credentials) for the three `autoconfig.*` endpoints:

- all three: **Connectivity Test Successful with Warnings** — certificate valid and trusted, name/dates OK, modern TLS protocols;
- two sites: warning "only 2 cipher suites compatible with Microsoft 365 services" (`TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256`, `TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384`) — sufficient for mail clients;
- all three: warning that the chain requires Windows Root Certificate Update; normal on supported Windows.

`Outlook Connectivity` was not run: RCA requires a working email, `Domain\User`/UPN, password and a captcha. RCA 4.3.9 has no separate "Outlook Autodiscover" test — Autodiscover is part of `Outlook Connectivity`.

## Matrix

| # | Client | OS | Scenario | Expected | Actual | Status |
|---|---|---|---|---|---|---|
| 1 | Outlook 2016 | Windows | New profile, email+password only | IMAP/SMTP settings applied | | |
| 2 | Outlook 2019 | Windows | New profile, email+password only | IMAP/SMTP settings applied | | |
| 3 | Outlook 365 (classic) | Windows | `Test E-mail AutoConfiguration` | 200, Account/Protocol IMAP+SMTP | | |
| 4 | New Outlook | Windows | Autodiscover v2 JSON behavior | Not applicable for IMAP/SMTP; v2 returns an EWS/ActiveSync URL only | | |
| 5 | Thunderbird | Windows | Account Setup, autoconfig | Servers/socket/auth correct | | |
| 6 | Thunderbird | macOS | Account Setup, autoconfig | Servers/socket/auth correct | | |
| 7 | Thunderbird | Linux | `.well-known` fallback | Servers/socket/auth correct | | |
| 8 | Apple Mail | macOS | Install `email.mobileconfig` | Profile installs, account connects | | |
| 9 | Mail | iOS | Install `email.mobileconfig` | Profile installs, account connects | | |
| 10 | K-9 Mail | Android | Autoconfig on first run | Servers/socket/auth correct | | |
| 11 | FairEmail | Android | Autoconfig on first run | Servers/socket/auth correct | | |
| 12 | curl | any | Unknown host | 404 | 404 | PASS (HTTP) |
| 13 | curl | any | Body > 1 MiB | 413 | 413 | PASS (HTTP) |
| 14 | curl | any | Logs after all tests | No email/query strings | no PII found | PASS (HTTP) |

## Steps

### Outlook desktop

1. Create a new profile and enter only the email address and password.
2. Let Autodiscover run; do not fill server fields manually.
3. Run `Test E-mail AutoConfiguration` (Ctrl+right-click the Outlook tray icon).
4. Confirm the IMAP/SMTP host, port, encryption and login name.
5. Restart Outlook and verify the account still connects (Autodiscover cache).

### Thunderbird

1. Account Setup → enter name, email and password.
2. Confirm the detected IMAP/SMTP settings before saving.
3. Check both `https://autoconfig.<domain>/mail/config-v1.1.xml` and `https://<domain>/.well-known/autoconfig/mail/config-v1.1.xml`.
4. Send and receive a test message.

### Apple

1. Open `https://autoconfig.<domain>/email.mobileconfig` in Safari and install the profile.
2. Replace the static `user@<domain>` address with the real one.
3. Verify incoming/outgoing servers and `Use SSL` for 993 and 587.
4. Send and receive a test message.

### Android

1. Install K-9 Mail or FairEmail.
2. Start account setup and enter email+password only.
3. Verify the detected servers and encryption.
4. Send and receive a test message.

### RCA

1. Open <https://testconnectivity.microsoft.com>.
2. Run **Outlook Autodiscover** and **Outlook Connectivity** for a real mailbox.
3. Save the report and note any Exchange-specific warnings that are expected for an IMAP provider.

## Sign-off

- [ ] All rows pass or have a documented, accepted deviation.
- [ ] No PII in logs after the run.
- [ ] RCA report attached.
- [ ] Rollback tested.
