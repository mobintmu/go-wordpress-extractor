# Outbound proxy and the SSRF guard

When `APP_PROXY_HOST` and `APP_PROXY_PORT` are set, every fetch (HTTP engine, AI engines' page fetch, and the browser engine) goes through that proxy.

## The rule

| Destination | With a proxy | Without a proxy |
|---|---|---|
| Hostname (`shop.example`) | The **proxy resolves it**. The local resolver is not asked and the answer is not checked. | Resolved locally; every address must be public. |
| IP literal (`10.0.0.5`, `169.254.169.254`, `127.0.0.1`) | Refused (private, loopback, link-local, reserved). | Refused. |
| Loopback or metadata name (`localhost`, `*.localhost`, `*.local`, `*.internal`, `metadata.google.internal`) | Refused. | Refused by resolution. |
| Blocked port (SSH, SMTP, databases, ...) | Refused. | Refused. |

## Why

On some networks the local DNS answers a private address for some public hosts (for example the Iran filter answers `10.10.34.36` for several US shops). Without a proxy those hosts cannot be reached, because the address is private. A configured proxy can resolve them, so the local answer is not used.

## What it trusts

The proxy. Hostnames are resolved by the proxy, so a name that resolves to a private address on the proxy's network would be reached through it. Only configure a proxy you trust, and that is not on a network you need to protect from these requests.

## Tests

- `internal/fetch/proxy_test.go`: `TestProxyResolvesHostnames`, `TestProxyStillBlocksPrivateLiteralsAndLoopbackNames`, `TestCheckProxiedDestination`.
