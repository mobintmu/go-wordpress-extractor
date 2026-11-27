# Publish the extractor over HTTPS with Cloudflare

Give the integrating service a base URL such as `https://extractor.example.com`. Cloudflare accepts HTTPS on TCP port **443** and redirects public HTTP on TCP port **80** to HTTPS. The Go service listens on **port 80** on the server's loopback interface. Clients use the HTTPS URL directly and send their existing `X-API-Key` header.

```text
Integrating service → Cloudflare :443 → Caddy :443 → Go HTTP 127.0.0.1:80
Public HTTP on port 80 redirects at Cloudflare.
```

Replace `extractor.example.com` throughout with a subdomain you control. Use the Caddy setup on a Linux server with systemd and a reachable public IP. Use [Cloudflare Tunnel](#cloudflare-tunnel) if you want the server itself to listen only on port 80 for this service.

## 1. Bind the extractor to the local server

For an install managed by `deploy.sh`, edit `.env.production` in the server checkout. Set these values alongside the existing configuration. For a first install, follow the [server setup guide](SERVER_SETUP.md) to create the service before configuring the port.

```dotenv
APP_HTTP_ADDRESS=127.0.0.1
APP_TLS_CERT_FILE=
APP_TLS_KEY_FILE=
```

Run `sudo ./deploy.sh port 80` from the checkout. The command sets the port, configures the systemd permission to bind to port 80 as the unprivileged `gwp` user, and restarts the service. [systemd service capabilities](https://www.freedesktop.org/software/systemd/man/latest/systemd.exec.html#AmbientCapabilities=)

Docker Compose already runs the extractor on port 80. In the server's [docker-compose.yaml](../docker-compose.yaml), replace the extractor's existing `ports` list with the mapping below to publish it on the host's loopback interface. Clear `APP_TLS_CERT_FILE` and `APP_TLS_KEY_FILE` in the Compose environment, then run `docker compose up -d extractor`.

```yaml
services:
  extractor:
    ports:
      - "127.0.0.1:80:80"
```

On the server, check that the extractor is ready before configuring Caddy or the Tunnel.

```bash
curl -fsS http://127.0.0.1/ready
```

Expect HTTP 200 with `{"message":"OK"}`.

## 2. Configure Caddy on port 443

### Use `deploy.sh caddy`

Start the extractor on local port 80 as shown in step 1. In Cloudflare, create an Origin CA certificate for your hostname under **SSL/TLS → Origin Server**. [Cloudflare Origin CA](https://developers.cloudflare.com/ssl/origin-configuration/origin-ca/)

```bash
sudo install -d -m 0755 /etc/caddy/certs
```

Save the certificate as `/etc/caddy/certs/extractor.pem` and the private key as `/etc/caddy/certs/extractor.key`. Save the key with permissions restricted to root. Then run the command from the repository checkout.

```bash
sudo ./deploy.sh caddy extractor.example.com
```

The command installs Caddy on systems with `apt-get` or `dnf` if needed, gives the Caddy service read access to the certificate and key, and validates and activates the configuration. On other distributions, install the Caddy systemd package first. For an existing **dedicated** Caddy install with a starter or other configuration you intend to replace, add `--replace` to back up the existing Caddyfile and replace it. For a shared Caddy instance, configure the hostname manually. Continue with the firewall and Cloudflare settings below. [Caddy installation](https://caddyserver.com/docs/install)

### Configure Caddy manually

1. Install Caddy using the systemd package instructions for your distribution. If another proxy already owns port 443, you can configure the hostname there. Any process listening on all addresses on port 80 must free the loopback address before the extractor can bind it. If the Caddy package starts with a default `:80` site, remove that starter site when configuring the extractor. [Caddy installation](https://caddyserver.com/docs/install)
2. In Cloudflare, open **SSL/TLS → Origin Server → Create Certificate**. Include your extractor hostname. On the server, save the PEM certificate to `/etc/caddy/certs/extractor.pem` and the private key to `/etc/caddy/certs/extractor.key`. The standard Caddy service runs as user `caddy`. Give that service read access to the files and restrict the key to the service and root, for example with owner `root:caddy` and mode `0640`. Keep the key out of the repository and track the certificate expiry so you can replace it before it expires. [Cloudflare Origin CA](https://developers.cloudflare.com/ssl/origin-configuration/origin-ca/)
3. For a dedicated Caddy instance, use this configuration in `/etc/caddy/Caddyfile`. It leaves port 80 available for the extractor and makes Caddy serve HTTPS with the origin certificate. The `auto_https disable_redirects` option is global to this Caddy instance. [Caddy global options](https://caddyserver.com/docs/caddyfile/options#auto-https) [Caddy TLS configuration](https://caddyserver.com/docs/caddyfile/directives/tls)

```caddyfile
{
    auto_https disable_redirects
}

https://extractor.example.com {
    tls /etc/caddy/certs/extractor.pem /etc/caddy/certs/extractor.key
    reverse_proxy 127.0.0.1:80
}
```

Validate and load the configuration on the server.

```bash
sudo -u caddy caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
sudo systemctl enable --now caddy
sudo systemctl reload caddy
sudo ss -ltnp '( sport = :80 or sport = :443 )'
```

Allow inbound TCP `443` in the host firewall and any cloud firewall or security group. If the server is behind a router with a public IP, forward TCP `443` to it. Keep the extractor's port 80 local to the server.

## 3. Connect the domain in Cloudflare

1. Make sure the domain is active in Cloudflare. If you are moving DNS there, follow the nameserver setup and verify existing DNS records before switching. [Cloudflare full DNS setup](https://developers.cloudflare.com/dns/zone-setups/full-setup/setup/)
2. Under **DNS → Records**, add an **A** record with name `extractor` and your server's public IPv4 address. Set **Proxy status** to **Proxied** (orange cloud). Add an **AAAA** record only if the origin also accepts connections at a public IPv6 address. [Cloudflare proxy status](https://developers.cloudflare.com/dns/proxy-status/)
3. Under **SSL/TLS → Overview**, use **Full (strict)**. This makes Cloudflare validate the certificate Caddy presents. If you change the zone-wide mode, verify that the other proxied origins in the zone also serve valid certificates. Under **SSL/TLS → Edge Certificates**, verify that an active certificate covers your extractor hostname. Clients validate that edge certificate. [Cloudflare Full (strict)](https://developers.cloudflare.com/ssl/origin-configuration/ssl-modes/full-strict/)
4. In **Rules**, create a **Single Redirect** with wildcard request URL `http://extractor.example.com/*` and target URL `https://extractor.example.com/${1}`. Choose status **308** and enable **Preserve query string**. Cloudflare handles public HTTP on port 80 before the request reaches your server. [Cloudflare Single Redirects](https://developers.cloudflare.com/rules/url-forwarding/single-redirects/settings/)

## 4. Verify from outside the server

Run these commands from another machine once the record and edge certificate are active.

```bash
API_URL=https://extractor.example.com
curl -sS -I http://extractor.example.com/ready
curl -fsS "$API_URL/ready"

read -r -s -p 'Extractor API key: ' EXTRACTOR_API_KEY
printf '\n'
curl -fsS -H "X-API-Key: $EXTRACTOR_API_KEY" "$API_URL/api/v1/sites"
unset EXTRACTOR_API_KEY
```

The HTTP check should redirect to `https://extractor.example.com/ready`. The HTTPS readiness check should return `{"message":"OK"}`, and the authenticated check should return the configured site list. Use the HTTPS base URL and the same header for [extraction requests](CURL_EXAMPLES.md).

## Troubleshooting

| Symptom | Check |
| --- | --- |
| Cloudflare `521` or `522` | Verify the DNS record points at the public origin IP, Caddy is running, and TCP 443 is reachable through every firewall and any router. |
| Cloudflare `525` or `526` | Check the Caddy TLS configuration, certificate expiry, hostname coverage, and key and certificate pairing. Keep the record proxied when using an Origin CA certificate because standard client trust stores lack that CA. |
| Caddy returns `502` | Run the local readiness check again and inspect `sudo journalctl -u caddy -n 50 --no-pager`. Verify the backend port and that the extractor serves HTTP on it. |
| The API returns `401` | Check that `X-API-Key` contains a valid, unexpired key issued by this instance. |

Extraction is synchronous and the app defaults to a three-minute request limit. Cloudflare's proxy read timeout can expire sooner and return `524`, including on public Tunnel routes. Test a representative extraction and set the application's deadline below your Cloudflare limit if you want the API to return its own timeout response first. [Cloudflare connection limits](https://developers.cloudflare.com/fundamentals/reference/connection-limits/)

## Cloudflare Tunnel

With a Tunnel, the only extractor listener on the server is `127.0.0.1:80`. Cloudflare serves the public HTTPS hostname on port 443 through an outbound connection from `cloudflared`. In **Networking → Tunnels**, create a tunnel and follow the dashboard commands to install `cloudflared` on the extractor host. Add a published application route with hostname `extractor.example.com` and service URL `http://127.0.0.1:80`. The route uses a tunnel DNS record, so clear any conflicting record for that same hostname before creating it. [Cloudflare Tunnel setup](https://developers.cloudflare.com/tunnel/get-started/)

Create the same Cloudflare **Single Redirect** for this hostname as described above, then run the external checks.

## Quick setup with Cloudflare Flexible SSL

Use this setup if your server has a reachable public IP and you want the extractor to serve HTTP directly on port 80. Cloudflare serves the domain's certificate on port 443. This setup skips Caddy and the origin certificate.

```text
Integrating service → HTTPS :443 → Cloudflare → HTTP :80 → Extractor
```

Cloudflare encrypts the client connection. The connection from Cloudflare to the server carries requests, responses, and the `X-API-Key` header in plaintext. [Cloudflare Flexible mode](https://developers.cloudflare.com/ssl/origin-configuration/ssl-modes/flexible/)

1. **Start the extractor on port 80.** For `deploy.sh`, set the following in `.env.production` and run `sudo ./deploy.sh port 80`.

   ```dotenv
   APP_HTTP_ADDRESS=0.0.0.0
   APP_TLS_CERT_FILE=
   APP_TLS_KEY_FILE=
   ```

   For Docker Compose, use the default port mapping `"80:80"`. If you applied the loopback mapping from step 1, replace it with the default mapping. Clear the same TLS settings in the Compose environment and run `docker compose up -d extractor`. In either deployment, port 80 must be available on the host. Run `curl -fsS http://127.0.0.1/ready` on the server and expect `{"message":"OK"}`.

2. **Allow Cloudflare to reach port 80.** In your server firewall or cloud security group, allow inbound TCP 80 from Cloudflare's IPv4 ranges. If you publish an IPv6 origin, also allow Cloudflare's IPv6 ranges. Prefer the cloud security group when using Docker so the restriction applies before traffic reaches the published port. If the server is behind a router, forward TCP 80 to it. The origin only needs inbound port 80 for this setup. [Cloudflare IP ranges and firewall guidance](https://developers.cloudflare.com/fundamentals/concepts/cloudflare-ip-addresses/)

3. **Add the DNS record.** In Cloudflare **DNS → Records**, create an **A** record named `extractor` that points to the server's public IPv4 address. Set it to **Proxied** (orange cloud). Replace `extractor.example.com` below with your hostname. [Cloudflare proxy status](https://developers.cloudflare.com/dns/proxy-status/)

4. **Enable Flexible and check the edge certificate.** If the zone is dedicated to this service, choose **Flexible** under **SSL/TLS → Overview**. If the zone also serves other hosts, use a **Configuration Rule** matching `http.host eq "extractor.example.com"` with **SSL** set to **Flexible**. Under **SSL/TLS → Edge Certificates**, verify that an active certificate covers the hostname. For a public HTTP-to-HTTPS redirect, use the Cloudflare Single Redirect from step 3 above and leave the extractor serving plain HTTP. [Cloudflare Configuration Rules](https://developers.cloudflare.com/rules/configuration-rules/settings/#ssl) [Cloudflare Universal SSL](https://developers.cloudflare.com/ssl/edge-certificates/universal-ssl/enable-universal-ssl/)

5. **Verify over HTTPS.** From another machine, run `curl -fsS https://extractor.example.com/ready`. Expect `{"message":"OK"}`. Then run the [authenticated check in step 4](#4-verify-from-outside-the-server). Give the integrating service `https://extractor.example.com` as its base URL.
