# Set up the server with `deploy.sh` or `menu.sh`

Use [`deploy.sh`](../deploy.sh) on a Linux server with systemd. It builds the binaries, writes `.env.production`, and runs the service as the `gwp` system user. Use [`menu.sh`](../menu.sh) for an interactive setup that runs in your terminal with `go run`. It uses `.env` by default or takes an existing env file as its first argument.

## Production with `./deploy.sh`

### 1. Put the project on the server

Place a fresh checkout under `/opt` so the service user can access the project files. For example, if Git is installed and your SSH key has access to the repository, run the following on the server.

```bash
git clone git@github.com:mobintmu/go-wordpress-extractor.git "$HOME/go-wordpress-extractor"
sudo mv "$HOME/go-wordpress-extractor" /opt/go-wordpress-extractor
cd /opt/go-wordpress-extractor
./deploy.sh help
```

You can also copy a clean checkout into `/opt/go-wordpress-extractor` and start with the `cd` command. On a 1–2 GB VPS, check [Building on a small server](BUILD_SMALL_SERVERS.md) before compiling.

### 2. Install Go and build the binaries

```bash
sudo ./deploy.sh go
sudo ./deploy.sh build
```

The build creates `bin/server`, `bin/apikey`, and `bin/sitectl`.

### 3. Create the production settings

```bash
sudo ./deploy.sh env
```

For a first setup, enter `127.0.0.1` as the listen address, accept the default port `80`, and leave the outbound proxy host empty. This lets you check the server locally and put an HTTPS reverse proxy in front of it when clients need remote access.

### 4. Create the pepper and your first API key

```bash
sudo ./deploy.sh pepper
sudo ./deploy.sh key new catalog-client 720h
sudo ./deploy.sh key list
```

The pepper is the server secret used to verify keys. Keep your current pepper if the script asks about replacing it. Changing it invalidates existing API keys and the Swagger password. Save the full `gwp_...` key when it appears. This example key lasts 30 days, and the list command shows its metadata.

### 5. Choose and validate your sites

```bash
sudo ./deploy.sh sites
```

Press Enter to keep the current list. To try a single enabled site, answer `n` when asked whether the current list is correct, enter `badomjip.com`, and press Enter to save. The command runs site config validation after the prompt.

Browser, search and AI engines can stay off for this example. Enable the engine a site requires when you add a site that uses one. This example turns on Chromium if you need the browser engine.

```bash
sudo ./deploy.sh browser enable
```

Choose `chromium`, enter `1` for the number of browser pages, and answer `n` to the Xvfb question to run headless. See [Engine configuration](ENGINES.md) for the other engines.

If Playwright 1.57.0 reports `Playwright does not support chromium on ubuntu26.04-x64`, use the updated checkout with Playwright 1.64.1 and retry.

```bash
sudo ./deploy.sh build
sudo ./deploy.sh browser enable
```

Choose `chromium` and answer `y` to Xvfb to keep window mode. Once the browser launch check succeeds, continue to step 7. [Playwright release notes](https://github.com/microsoft/playwright/releases/tag/v1.61.0)

### 6. Set up Swagger if you need it

```bash
sudo ./deploy.sh swagger enable
```

Enter `admin` and type your own password of at least 12 characters twice. Once the service is running, open `http://127.0.0.1/swagger/index.html` on the server and log in with those credentials. Skip this step if you only need the API.

### 7. Install and start the service

```bash
sudo ./deploy.sh service
sudo ./deploy.sh restart
sudo ./deploy.sh status
```

`service` enables startup at boot. `restart` starts the service and waits for `/ready`. Look for `service is up` and `active (running)`. Then run the API checks below using the configured port.

To make the API available on an HTTPS domain, follow the [Cloudflare HTTPS setup guide](CLOUDFLARE_HTTPS_SETUP.md).

### Change the service port

Run `sudo ./deploy.sh port 8080` to set a custom port, or `sudo ./deploy.sh port` to set port 80. The command updates `.env.production`, restarts the service, and checks `/ready`. If startup fails, it restores the previous port and attempts to restart on it. Ports below 1024 receive the systemd permission needed to bind without running the extractor as root. Update any reverse proxy that points to the previous port.

### Watch the service logs live

```bash
cd /opt/go-wordpress-extractor
sudo ./deploy.sh logs
```

The command shows recent service logs and streams new entries as they arrive. Press `Ctrl+C` to stop watching. The service keeps running.

Find `extraction started` and `extraction finished` by their shared `request_id`. The finish entry shows the outcome, duration, engine and model when available. It includes the price and completeness on success, or the error code and stage on failure. AI failures also include available upstream diagnostics such as `ai_http_status` and `ai_request_id`. A failed extraction can have `http_status: 200` because the extractor returns the failure in the response body. Logged product URLs omit query parameters.

### Fix `200/CHDIR` for a checkout under `/root`

The systemd service runs as `gwp` and needs to enter the checkout directory. If the logs show `200/CHDIR` for `/root/go-wordpress-extractor`, run `relocate` from that checkout.

```bash
sudo ./deploy.sh relocate
```

The command stops the service, moves the checkout to `/opt/go-wordpress-extractor`, updates absolute paths in `.env.production`, and rewrites and starts the systemd unit. The binaries, pepper and API key file move with the checkout. If the destination already exists, the command stops before moving anything.

Look for `service is up`. After the move, use the new path to check status. If setup was interrupted after the move, rerun `sudo ./deploy.sh relocate` from the new location.

```bash
cd /opt/go-wordpress-extractor
sudo ./deploy.sh status
```

### Run the same setup in one command

After putting the project under `/opt`, you can use the interactive installer in place of steps 2–7.

```bash
sudo ./deploy.sh install
```

For the minimal setup, enter `127.0.0.1` and accept port `80`, leave the outbound proxy host blank, and explicitly answer `n` to the browser, search and AI prompts. When asked for a key, enter `catalog-client` and `720h`, then save the printed key. Answer `n` to the extra key menu, confirm the website list or pick `badomjip.com`, and answer `n` to Swagger if you only need the API. The installer starts the service and checks readiness.

## Foreground setup with `./menu.sh`

### 1. Check Go and create a local `.env`

Run as your normal user from the project root. `menu.sh` needs Go on `PATH` and a writable env file. The Go version required by this checkout is listed in [go.mod](../go.mod).

```bash
go version
```

In a fresh checkout, create `.env` with the example below. If you already have a `.env` you want to keep, use it and check its port, key file, and engine settings before opening the menu. The example uses port `4001` so it can run alongside the production service on port `80`.

```bash
(
  umask 077
  mkdir -p secrets
  set -C
  cat > .env <<'EOF'
APP_ENV=development
APP_HTTP_ADDRESS=127.0.0.1
APP_HTTP_PORT=4001
APP_SITE_CONFIG_DIR=configs
APP_API_KEYS_FILE=secrets/menu-api-keys.json
APP_API_KEY_PEPPER=
APP_BROWSER_ENABLED=false
APP_DUCKDUCKGO_ENABLED=false
APP_AI_CRAWLER_ENABLED=false
APP_DEFAULT_ENGINE=none
EOF
)
```

### 2. Set up the pepper, operator login, and client key

```bash
./menu.sh
```

Choose `4` (**Set up all keys**). The menu backs up `.env` and generates a pepper if needed. Enter `admin` for the operator login and type your own password of at least 12 characters twice. Answer `y` to create a client key, enter `menu-client` as its name, and enter `30` for the number of days. Save the full `gwp_...` key, then choose `3` to see the key listed. Keep the env file, its `.bak.*` copies, and `secrets/` out of Git.

### 3. Start the server from the menu

Choose `1` (**Run server**). Leave that terminal open and run the API checks below in a second terminal using port `4001`. Press Ctrl+C in the server terminal to stop it. Run `./menu.sh` again when you want to return to the menu.

### Other menu choices

| Choice | Simple example |
| --- | --- |
| `5` Create a client key | Enter `reporting-client` and `7` for a seven-day key. |
| `6` Set proxy | If an HTTP proxy is already listening at `127.0.0.1:3067`, enter `127.0.0.1` and `3067`, then answer `y` to save it for the server's outbound fetches. Enter `none` as the host to clear it. |
| `7` AI crawler | Choose `a` for AvalAI if you have its API key, or `c` for Cloudflare if you have an account ID and token. Enter the requested values, then press Enter at the submenu to return. |
| `8` Learned configs | Choose `8` to list configs. To inspect an existing one, enter `show` and a domain from that list. Press Enter to return. |
| `9` Default engine | Choose `3` in the submenu to set `none`. Hosts without a rule then return `SITE_NOT_CONFIGURED`. |
| `2` Delete all keys | Enter `n` if you opened this by mistake. To clear the demo keys, answer `y` and type `remove all`. Use choice `5` to issue a fresh key. |
| `0` Exit | Enter `0` to close the menu without starting the server. |

## Check that the server is ready

Run these checks from the machine where the server is running. Use port `80` for the production example or `4001` for the menu example.

```bash
PORT=80
curl -fsS "http://127.0.0.1:$PORT/health"
curl -fsS "http://127.0.0.1:$PORT/ready"
```

Both endpoints should return HTTP `200` with `{"message":"OK"}`. Then paste the API key from the setup you started when prompted.

```bash
read -r -s -p 'Paste your API key and press Enter: ' API_KEY
printf '\n'
curl -fsS -H "X-API-Key: $API_KEY" "http://127.0.0.1:$PORT/api/v1/sites"
unset API_KEY
```

A successful response confirms that the server accepts your key and returns the configured site list. If you get `401`, check that you used the key from the same setup and that it hasn't expired.
