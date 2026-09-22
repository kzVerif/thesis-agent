# HTTPS/WSS transport deployment (Security Phase 3)

TLS validates the server and protects bytes in transit. Phase 4 now separately
requires Agent Ed25519 proof before normal traffic; see [Agent authentication](agent-authentication.md).
Normal feature JSON is unchanged; authentication adds a preceding handshake.

## Configuration and upgrade

The Agent reads process environment first, then fills missing values from its
runtime .env. Console uses its working directory. Service/provisioning uses the
protected ProgramData runtime returned by --service-info, as before.

Production configuration:

```dotenv
TRANSPORT_MODE=production
AGENT_API_URL=https://api.example.com
WS_SERVER_URL=wss://socket.example.com/ws
ALLOW_LOCAL_HTTP_DOWNLOADS=false
```

If TRANSPORT_MODE is absent, console defaults to development and Service/provisioning
defaults to production. Unknown modes fail. Production rejects HTTP/WS even on
localhost and rejects the local-HTTP download exception. Development permits
HTTP/WS only for localhost or literal loopback IPs; remote endpoints still need
HTTPS/WSS. A localhost development Service must explicitly set
TRANSPORT_MODE=development in the configuration supplied to dev-service.ps1.
Existing plaintext Service configurations need this explicit development setting
or HTTPS/WSS before deploying the new binary; the installed binary/configuration
are not changed by source edits or build checks.

Do not change agent_config.json or recreate identity to change transport.
The existing enrollment marker is tied to API URL; switching to HTTPS causes an
existence check against that URL using the same identity. A present record skips
enrollment; Service receiving 404 still requires administrator provisioning.
Verify the new public hostname points to the same database before rollout.
Do not enter another enrollment token to hide a misrouted deployment.

## TLS and redirects

Clients use Go's normal certificate-chain, hostname and validity checks with
platform trust. There is no skip-verification flag, pinning, downloaded CA,
automatic trust-store modification or alternate cryptographic handshake.
For public CA certificates no custom root configuration is required. An internal
CA must be independently installed by administrators in the appropriate Windows
machine trust store for LocalSystem; a developer's CurrentUser trust is not proof
of Service trust.

REST, WS dialing and downloads share a redirect policy: max 10, no HTTPS-to-HTTP
downgrade, no other hostname/port. Same-authority redirects are supported;
HTTP default port 80 to HTTPS default port 443 on the same hostname is allowed.
Configure final endpoints directly when migrating hosts or using a different port.
This avoids replaying registration bodies, Agent headers or download token URLs
to another authority. Download tokens, size limits and SHA-256 checks are unchanged.
Config endpoints disallow embedded credentials/query/fragment; download token
URLs keep their existing representation.

TLS failures use existing bounded backoff/jitter and cancellation behavior.
Logs report a fixed failure category, never raw certificate errors or token URLs.
Service becomes locally ready before network enrollment verification as before.
Legacy DPAPI identity does not trigger migration/decryption at startup.

## Same-host Cloudflare deployment

Agent -> HTTPS api.example.com / WSS socket.example.com -> Cloudflare edge ->
encrypted tunnel -> cloudflared -> HTTP 127.0.0.1:8080 / :8081.
The WS repository contains deploy/cloudflared.example.yml and its transport guide.
Do not expose the HTTP origin listener to the LAN/public Internet.
If cloudflared is on a different machine, terminate and validate TLS in a proxy
on the origin host (or use another protected origin transport). The Go services
may remain loopback HTTP. Never disable origin certificate verification.

The browser Dashboard and /ws/frontend must share a hostname because
__Host-session is host-only. Example: lab.example.com goes to Next.js, except
/ws/frontend goes to WS. API and Agent WS hostnames may be separate.
Do not broaden the cookie Domain or put authentication data in WS URLs.

## Manual LocalSystem verification (not covered by unit tests)

Use an isolated lab Agent and preserve its existing protected identity/config.
1. Deploy trusted production-like public HTTPS/WSS endpoints and confirm the
   REST/WS processes share the intended database and storage.
2. Configure production URLs in protected runtime .env; review process/machine
   environment overrides. Install/start the Service through the existing tooling.
3. Confirm LocalSystem via SCM. Confirm logs show enrollment verified over HTTPS
   and websocket connected over WSS; exercise existing feature commands.
4. On a controlled test endpoint, present an untrusted certificate, wrong hostname,
   and expired certificate separately. Confirm rejection and bounded retry logs.
   Do not install that test CA, change clocks, disable TLS checks or use real tokens.
5. Restore valid certificates/endpoints; restart for configuration changes.
   Confirm enrollment/WSS reconnect and unchanged Agent ID/public key.
6. Exercise a HTTPS-to-HTTP redirect and confirm no request reaches the HTTP sink.
7. Verify graceful administrator Stop-Service, recovery after a real crash, reboot,
   and no busy reconnect loop while the tunnel is unavailable.

Automated TLS tests use temporary local certificates and in-memory roots only.
Developer-account PASS does not prove LocalSystem trust or a live Cloudflare route.

Phase 2 manual checks remain outstanding: LocalSystem private-key decrypt,
actual protected-runtime migration, post-migration SCM deployment, reboot and
real power-loss behavior. Phase 3 does not claim to complete them.
