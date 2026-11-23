#!/usr/bin/env bash
# deploy.sh - set up and manage go-wordpress-extractor on a fresh Linux server.
#
# Copy the project folder to the server, then run as root from anywhere:
#
#   sudo ./deploy.sh install          # full setup (Go, build, env, keys, sites, service)
#   sudo ./deploy.sh help             # every other command
#
# Nothing has to be installed first. The script installs curl/tar if they are
# missing, downloads the latest Go, builds the binaries, and writes the
# production env file. Swagger UI stays OFF unless you turn it on.
#
# Optional environment overrides:
#   GO_VERSION=go1.27.4     install this Go version instead of the latest
#   GOPROXY=...             module proxy used while building (passed to go)
#   BUILD_JOBS=2            packages compiled in parallel (default: one per CPU).
#                           Use this on a small server: one big package such as
#                           github.com/openai/openai-go/v3 needs ~1.5-2 GB while
#                           it compiles, and too many at once end in
#                           "compile: signal: killed" (the kernel OOM killer).
#   NODE_MAJOR=22           Node.js major version for the browser engine
#   PLAYWRIGHT_VERSION=1.57.0  Playwright driver version (default: read from go.mod)
#
# Behind an HTTP proxy, export it and keep it through sudo; Go, Node, npm and
# the Chromium download all use it:
#   export https_proxy=http://127.0.0.1:3067 http_proxy=http://127.0.0.1:3067
#   sudo -E ./deploy.sh install

set -Eeuo pipefail

# ---------------------------------------------------------------- paths
APP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="$APP_DIR/.env.production"
BIN_DIR="$APP_DIR/bin"
CONFIG_DIR="$APP_DIR/configs"
ACTIVE_FILE="$CONFIG_DIR/active_sites.json"
SECRETS_DIR="$APP_DIR/secrets"
KEYS_FILE="$SECRETS_DIR/.env.apikeys"
GO_ROOT="/usr/local/go"
GO_BIN="$GO_ROOT/bin/go"
SERVICE_NAME="go-wordpress-extractor"
SERVICE_USER="gwp"
SERVICE_FILE="/etc/systemd/system/$SERVICE_NAME.service"
# Browser fetch engine (Playwright + Chromium), outside the project folder
# so the service user can always read it.
PW_DIR="/opt/$SERVICE_NAME-playwright"
PW_NODE_DIR="$PW_DIR/node"
PW_DRIVER_DIR="$PW_DIR/driver"
PW_BROWSERS_DIR="$PW_DIR/browsers"
NODE_MAJOR="${NODE_MAJOR:-22}"
# Virtual display for Chrome in a window on a server without a screen
# (APP_BROWSER_HEADLESS=false). It runs as its own systemd unit.
XVFB_SERVICE="$SERVICE_NAME-xvfb"
XVFB_SERVICE_FILE="/etc/systemd/system/$XVFB_SERVICE.service"
XVFB_DISPLAY="${XVFB_DISPLAY:-:99}"

# ---------------------------------------------------------------- output
if [ -t 1 ]; then
  C_RED=$'\e[31m'; C_GRN=$'\e[32m'; C_YLW=$'\e[33m'; C_BLU=$'\e[34m'; C_BLD=$'\e[1m'; C_OFF=$'\e[0m'
else
  C_RED=""; C_GRN=""; C_YLW=""; C_BLU=""; C_BLD=""; C_OFF=""
fi
info()  { printf '%s==>%s %s\n' "$C_BLU" "$C_OFF" "$*"; }
ok()    { printf '%s ok%s %s\n' "$C_GRN" "$C_OFF" "$*"; }
warn()  { printf '%swarn%s %s\n' "$C_YLW" "$C_OFF" "$*" >&2; }
die()   { printf '%serror%s %s\n' "$C_RED" "$C_OFF" "$*" >&2; exit 1; }
title() { printf '\n%s%s%s\n' "$C_BLD" "$*" "$C_OFF"; }
trap 'die "command failed at line $LINENO: $BASH_COMMAND"' ERR

# ask "question" "default" -> echoes the answer
ask() {
  local q="$1" def="${2:-}" a
  if [ -n "$def" ]; then read -r -p "$q [$def]: " a </dev/tty; else read -r -p "$q: " a </dev/tty; fi
  printf '%s' "${a:-$def}"
}

# confirm "question" y|n -> exit status 0 for yes
confirm() {
  local q="$1" def="${2:-n}" hint a
  [ "$def" = y ] && hint="Y/n" || hint="y/N"
  read -r -p "$q [$hint]: " a </dev/tty
  a="${a:-$def}"
  [[ "$a" =~ ^[Yy]([Ee][Ss])?$ ]]
}

need_root() { [ "$(id -u)" -eq 0 ] || die "run as root: sudo $0 $*"; }

# ---------------------------------------------------------------- env file
# env_get KEY -> value from the env file (empty if missing)
env_get() {
  [ -f "$ENV_FILE" ] || return 0
  grep -E "^$1=" "$ENV_FILE" | tail -n1 | cut -d= -f2- || true
}

# env_set KEY VALUE -> replace or append the line in the env file
env_set() {
  local key="$1" val="$2" tmp
  touch "$ENV_FILE"
  tmp="$(mktemp)"
  if grep -qE "^$key=" "$ENV_FILE"; then
    K="$key" V="$val" awk 'BEGIN{FS="="} $1==ENVIRON["K"] {print ENVIRON["K"] "=" ENVIRON["V"]; next} {print}' "$ENV_FILE" >"$tmp"
  else
    cat "$ENV_FILE" >"$tmp"; printf '%s=%s\n' "$key" "$val" >>"$tmp"
  fi
  cat "$tmp" >"$ENV_FILE"; rm -f "$tmp"
}

# Load pepper and key file into this shell so the apikey tool uses the
# same values as the server.
load_key_env() {
  [ -f "$ENV_FILE" ] || die "no $ENV_FILE yet. Run: $0 env"
  export APP_ENV=production
  export APP_API_KEY_PEPPER; APP_API_KEY_PEPPER="$(env_get APP_API_KEY_PEPPER)"
  export APP_API_KEYS_FILE; APP_API_KEYS_FILE="$(env_get APP_API_KEYS_FILE)"
  [ "${#APP_API_KEY_PEPPER}" -ge 32 ] || die "APP_API_KEY_PEPPER is missing or too short. Run: $0 pepper"
  [ -n "$APP_API_KEYS_FILE" ] || APP_API_KEYS_FILE="$KEYS_FILE"
}

apikey() {
  [ -x "$BIN_DIR/apikey" ] || die "apikey is not built yet. Run: $0 build"
  "$BIN_DIR/apikey" "$@"
}

fix_perms() {
  mkdir -p "$SECRETS_DIR"
  chmod 700 "$SECRETS_DIR"
  find "$SECRETS_DIR" -type f -exec chmod 600 {} +
  if id "$SERVICE_USER" >/dev/null 2>&1; then
    chown -R "$SERVICE_USER:$SERVICE_USER" "$SECRETS_DIR"
    # The server re-reads the env file itself, so its group must be able to read it.
    [ -f "$ENV_FILE" ] && chown "root:$SERVICE_USER" "$ENV_FILE" && chmod 640 "$ENV_FILE"
  else
    [ -f "$ENV_FILE" ] && chmod 600 "$ENV_FILE"
  fi
  return 0
}

# ================================================================ 1. packages
install_packages() {
  local missing=()
  for c in curl tar sha256sum; do command -v "$c" >/dev/null 2>&1 || missing+=("$c"); done
  [ "${#missing[@]}" -eq 0 ] && [ -d /etc/ssl/certs ] && { ok "base tools present"; return; }
  info "installing base tools (curl, tar, ca-certificates)"
  if   command -v apt-get >/dev/null; then apt-get update -y && DEBIAN_FRONTEND=noninteractive apt-get install -y curl tar ca-certificates coreutils
  elif command -v dnf     >/dev/null; then dnf install -y curl tar ca-certificates coreutils
  elif command -v yum     >/dev/null; then yum install -y curl tar ca-certificates coreutils
  elif command -v apk     >/dev/null; then apk add --no-cache curl tar ca-certificates coreutils bash
  elif command -v zypper  >/dev/null; then zypper --non-interactive install curl tar ca-certificates coreutils
  else die "no supported package manager found. Install curl, tar and ca-certificates by hand."
  fi
  ok "base tools installed"
}

# ================================================================ 2. Go
go_arch() {
  case "$(uname -m)" in
    x86_64|amd64)  echo amd64 ;;
    aarch64|arm64) echo arm64 ;;
    armv6l|armv7l) echo armv6l ;;
    i386|i686)     echo 386 ;;
    ppc64le)       echo ppc64le ;;
    s390x)         echo s390x ;;
    riscv64)       echo riscv64 ;;
    *) die "unsupported CPU: $(uname -m)" ;;
  esac
}

install_go() {
  need_root "go"
  install_packages
  local version arch file url tmp want got current=""
  version="${GO_VERSION:-$(curl -fsSL 'https://go.dev/VERSION?m=text' | head -n1)}"
  [[ "$version" =~ ^go[0-9]+\.[0-9]+ ]] || die "could not read the latest Go version (got '$version'). Set GO_VERSION=go1.x.y"
  [ -x "$GO_BIN" ] && current="$("$GO_BIN" env GOVERSION 2>/dev/null || true)"
  if [ "$current" = "$version" ]; then ok "Go $version already installed"; return; fi

  arch="$(go_arch)"
  file="$version.linux-$arch.tar.gz"
  url="https://dl.google.com/go/$file"
  tmp="$(mktemp -d)"
  info "downloading $file (installed: ${current:-none})"
  curl -fL --retry 3 -o "$tmp/$file" "$url"
  want="$(curl -fsSL "$url.sha256" | awk '{print $1}')"
  got="$(sha256sum "$tmp/$file" | awk '{print $1}')"
  [ -n "$want" ] && [ "$want" = "$got" ] || { rm -rf "$tmp"; die "checksum mismatch for $file"; }
  ok "checksum verified"

  rm -rf "$GO_ROOT"
  tar -C /usr/local -xzf "$tmp/$file"
  rm -rf "$tmp"
  cat >/etc/profile.d/go.sh <<'EOF'
export PATH="$PATH:/usr/local/go/bin"
EOF
  ok "installed $("$GO_BIN" version)"
}

# ================================================================ 3. build
build() {
  [ -x "$GO_BIN" ] || install_go
  info "building server, apikey and sitectl"
  mkdir -p "$BIN_DIR"
  (
    cd "$APP_DIR"
    export CGO_ENABLED=0 PATH="$GO_ROOT/bin:$PATH"
    # A single compile can take 1.5-2 GB (openai-go, sonic, ...); on a small
    # server the kernel kills the compiler ("signal: killed") when several run
    # at once. BUILD_JOBS caps how many run in parallel.
    local jobs=()
    if [ -n "${BUILD_JOBS:-}" ]; then
      [[ "$BUILD_JOBS" =~ ^[1-9][0-9]*$ ]] || die "BUILD_JOBS must be a positive number, for example BUILD_JOBS=2"
      jobs=(-p "$BUILD_JOBS")
      info "compiling at most $BUILD_JOBS package(s) at a time"
    fi
    for cmd in server apikey sitectl; do
      "$GO_BIN" build ${jobs[@]+"${jobs[@]}"} -buildvcs=false -trimpath -ldflags="-s -w" -o "$BIN_DIR/$cmd" "./cmd/$cmd"
    done
  )
  ok "binaries in $BIN_DIR"
}

# ================================================================ 3b. browser engine
browser_enabled() { [ "$(env_get APP_BROWSER_ENABLED)" = "true" ]; }

browser_channel() { local c; c="$(env_get APP_BROWSER_CHANNEL)"; printf '%s' "${c:-chromium}"; }

# The driver must match the playwright-go version in go.mod, so read it
# from the module source (run.go: playwrightCliVersion).
playwright_version() {
  if [ -n "${PLAYWRIGHT_VERSION:-}" ]; then printf '%s' "$PLAYWRIGHT_VERSION"; return; fi
  local dir v=""
  dir="$(cd "$APP_DIR" && PATH="$GO_ROOT/bin:$PATH" "$GO_BIN" mod download github.com/playwright-community/playwright-go >/dev/null 2>&1 &&
    PATH="$GO_ROOT/bin:$PATH" "$GO_BIN" list -m -f '{{.Dir}}' github.com/playwright-community/playwright-go 2>/dev/null || true)"
  if [ -n "$dir" ] && [ -f "$dir/run.go" ]; then
    v="$(grep -oE 'playwrightCliVersion = "[0-9]+\.[0-9]+\.[0-9]+"' "$dir/run.go" | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' || true)"
  fi
  [ -n "$v" ] || die "could not read the Playwright version from go.mod. Set PLAYWRIGHT_VERSION=1.x.y"
  printf '%s' "$v"
}

node_arch() {
  case "$(uname -m)" in
    x86_64|amd64)  echo x64 ;;
    aarch64|arm64) echo arm64 ;;
    armv7l)        echo armv7l ;;
    ppc64le)       echo ppc64le ;;
    s390x)         echo s390x ;;
    *) die "Node.js has no build for $(uname -m)" ;;
  esac
}

# Node.js runs the Playwright driver. It is installed under $PW_NODE_DIR
# only, so it never conflicts with a system Node.
install_node() {
  local arch base sums file version current="" tmp want got
  arch="$(node_arch)"
  base="https://nodejs.org/dist/latest-v$NODE_MAJOR.x"
  sums="$(curl -fsSL "$base/SHASUMS256.txt")" || die "could not reach $base (set https_proxy if this server needs a proxy)"
  file="$(printf '%s\n' "$sums" | awk -v suffix="-linux-$arch.tar.gz" 'substr($2, length($2) - length(suffix) + 1) == suffix {print $2; exit}')"
  [ -n "$file" ] || die "no Node.js $NODE_MAJOR build for linux-$arch"
  version="${file#node-}"; version="${version%%-linux-*}"
  [ -x "$PW_NODE_DIR/bin/node" ] && current="$("$PW_NODE_DIR/bin/node" --version 2>/dev/null || true)"
  if [ "$current" = "$version" ]; then ok "Node.js $version already installed"; return; fi

  tmp="$(mktemp -d)"
  info "downloading $file (installed: ${current:-none})"
  curl -fL --retry 3 -o "$tmp/$file" "$base/$file"
  want="$(printf '%s\n' "$sums" | awk -v f="$file" '$2 == f {print $1}')"
  got="$(sha256sum "$tmp/$file" | awk '{print $1}')"
  [ -n "$want" ] && [ "$want" = "$got" ] || { rm -rf "$tmp"; die "checksum mismatch for $file"; }
  ok "checksum verified"
  rm -rf "$PW_NODE_DIR"
  mkdir -p "$PW_NODE_DIR"
  tar -C "$PW_NODE_DIR" --strip-components=1 -xzf "$tmp/$file"
  rm -rf "$tmp"
  ok "installed Node.js $("$PW_NODE_DIR/bin/node" --version)"
}

# The driver is node + the playwright-core package. "playwright install"
# from playwright-go cannot be used: its driver download is no longer hosted.
install_playwright_driver() {
  local version="$1" tmp
  if [ -x "$PW_DRIVER_DIR/node" ] && [ -f "$PW_DRIVER_DIR/package/cli.js" ] &&
     "$PW_DRIVER_DIR/node" "$PW_DRIVER_DIR/package/cli.js" --version 2>/dev/null | grep -qx "Version $version"; then
    ok "Playwright driver $version already installed"; return
  fi
  tmp="$(mktemp -d)"
  info "downloading playwright-core $version from npm"
  (cd "$tmp" && PATH="$PW_NODE_DIR/bin:$PATH" npm pack "playwright-core@$version" --silent >/dev/null) ||
    { rm -rf "$tmp"; die "npm could not download playwright-core@$version (set https_proxy if this server needs a proxy)"; }
  rm -rf "$PW_DRIVER_DIR"
  mkdir -p "$PW_DRIVER_DIR"
  tar -xzf "$tmp/playwright-core-$version.tgz" -C "$PW_DRIVER_DIR"
  rm -rf "$tmp"
  ln -sf "$PW_NODE_DIR/bin/node" "$PW_DRIVER_DIR/node"
  ok "installed Playwright driver $("$PW_DRIVER_DIR/node" "$PW_DRIVER_DIR/package/cli.js" --version)"
}

# Download the browser build the driver expects, plus the system libraries
# it needs (Debian/Ubuntu only; other systems must install them by hand).
install_playwright_browsers() {
  local channel targets deps=()
  channel="$(browser_channel)"
  case "$channel" in
    chromium) targets=(chromium chromium-headless-shell) ;;
    chrome)   [ "$(uname -m)" = x86_64 ] || die "Google Chrome is only available for x86_64; use APP_BROWSER_CHANNEL=chromium"
              targets=(chrome) ;;
    *)        die "APP_BROWSER_CHANNEL must be chromium or chrome (is '$channel')" ;;
  esac
  if command -v apt-get >/dev/null 2>&1; then
    deps=(--with-deps)
  else
    warn "not a Debian/Ubuntu system: install Chromium's shared libraries by hand if the launch check below fails"
  fi
  mkdir -p "$PW_BROWSERS_DIR"
  info "installing ${targets[*]} (this downloads about 150-250 MB)"
  PLAYWRIGHT_BROWSERS_PATH="$PW_BROWSERS_DIR" DEBIAN_FRONTEND=noninteractive \
    "$PW_DRIVER_DIR/node" "$PW_DRIVER_DIR/package/cli.js" install "${deps[@]}" "${targets[@]}"
  ok "browsers in $PW_BROWSERS_DIR"
}

# browser_headful -> success when Chrome runs in a window (on Xvfb)
browser_headful() { browser_enabled && [ "$(env_get APP_BROWSER_HEADLESS)" = "false" ]; }

# Xvfb is an X server that draws into memory: Chrome gets a real window,
# which passes anti-bot checks (Cloudflare) that headless Chrome fails,
# without a monitor or GPU. About 20-50 MB of RAM.
install_xvfb() {
  if command -v Xvfb >/dev/null 2>&1 && command -v xvfb-run >/dev/null 2>&1; then
    ok "Xvfb present ($(command -v Xvfb))"; return
  fi
  info "installing Xvfb (virtual display)"
  if   command -v apt-get >/dev/null; then apt-get update -y && DEBIAN_FRONTEND=noninteractive apt-get install -y xvfb xauth
  elif command -v dnf     >/dev/null; then dnf install -y xorg-x11-server-Xvfb
  elif command -v yum     >/dev/null; then yum install -y xorg-x11-server-Xvfb
  elif command -v apk     >/dev/null; then apk add --no-cache xvfb
  elif command -v zypper  >/dev/null; then zypper --non-interactive install xorg-x11-server-Xvfb
  else die "no supported package manager found. Install Xvfb by hand."
  fi
  command -v Xvfb >/dev/null 2>&1 || die "Xvfb is still missing after the install"
  command -v xvfb-run >/dev/null 2>&1 || warn "xvfb-run is not available here; '$0 run' and '$0 dev' need it (the systemd unit does not)"
  ok "installed Xvfb"
}

# Launch the browser once, as the service user when it exists, to catch
# missing libraries before the server tries. With APP_BROWSER_HEADLESS=false
# it runs in a window on a temporary Xvfb display, as the service will.
check_browser_launch() {
  local channel js arg="" headless=true runner=() xpid="" display="" version
  channel="$(browser_channel)"
  [ "$channel" = chrome ] && arg=chrome
  browser_headful && headless=false
  js='const {chromium} = require(process.argv[1]);
const opts = {headless: process.argv[3] !== "false"};
if (process.argv[2]) opts.channel = process.argv[2];
chromium.launch(opts)
  .then(async b => { console.log(b.version()); await b.close(); })
  .catch(e => { console.error(e.message); process.exit(1); });'
  if id "$SERVICE_USER" >/dev/null 2>&1 && command -v runuser >/dev/null 2>&1; then
    runner=(runuser -u "$SERVICE_USER" --)
  fi
  if [ "$headless" = false ]; then
    local n=$((200 + RANDOM % 600))
    while [ -e "/tmp/.X11-unix/X$n" ] || [ -e "/tmp/.X$n-lock" ]; do n=$((n + 1)); done
    display=":$n"
    "${runner[@]}" "$(command -v Xvfb)" "$display" -screen 0 1920x1080x24 -nolisten tcp >/dev/null 2>&1 &
    xpid=$!
    sleep 2
    kill -0 "$xpid" 2>/dev/null || die "Xvfb did not start on $display"
  fi
  info "checking that $channel launches ($([ "$headless" = false ] && echo "in a window on Xvfb $display" || echo headless))"
  if version="$("${runner[@]}" env HOME=/tmp DISPLAY="$display" PLAYWRIGHT_BROWSERS_PATH="$PW_BROWSERS_DIR" \
       "$PW_DRIVER_DIR/node" -e "$js" "$PW_DRIVER_DIR/package" "$arg" "$headless" 2>&1)"; then
    [ -n "$xpid" ] && kill "$xpid" 2>/dev/null
    ok "$channel $version launches"
  else
    [ -n "$xpid" ] && kill "$xpid" 2>/dev/null
    printf '%s\n' "$version" >&2
    die "the browser does not start (see above). On non-Debian systems install its libraries, or set APP_BROWSER_ENABLED=false"
  fi
}

# install_playwright [force] -> set up the browser engine when it is enabled
# in the env file (or always with "force").
install_playwright() {
  need_root "browser"
  if [ "${1:-}" != force ] && ! browser_enabled; then
    ok "browser fetch engine is off (APP_BROWSER_ENABLED); skipping Playwright"; return
  fi
  install_packages
  [ -x "$GO_BIN" ] || install_go
  local version; version="$(playwright_version)"
  info "Playwright $version for playwright-go in go.mod"
  install_node
  install_playwright_driver "$version"
  install_playwright_browsers
  browser_headful && install_xvfb
  chmod -R a+rX "$PW_DIR"
  env_set PLAYWRIGHT_DRIVER_PATH "$PW_DRIVER_DIR"
  env_set PLAYWRIGHT_BROWSERS_PATH "$PW_BROWSERS_DIR"
  fix_perms
  check_browser_launch
}

browser_status() {
  if browser_enabled; then
    echo "Browser fetch engine: ENABLED (channel: $(browser_channel), max pages: $(v=$(env_get APP_BROWSER_MAX_PAGES); echo "${v:-3}"))"
    if browser_headful; then
      local xs="not installed"
      command -v systemctl >/dev/null 2>&1 && [ -f "$XVFB_SERVICE_FILE" ] && xs="$(systemctl is-active "$XVFB_SERVICE" 2>/dev/null || true)"
      echo "  window   : Chrome runs in a window on Xvfb $XVFB_DISPLAY (service $XVFB_SERVICE: $xs)"
    else
      echo "  window   : headless (no display)"
    fi
  else
    echo "Browser fetch engine: DISABLED"
  fi
  if [ -f "$PW_DRIVER_DIR/package/cli.js" ]; then
    echo "  driver   : $("$PW_DRIVER_DIR/node" "$PW_DRIVER_DIR/package/cli.js" --version 2>/dev/null || echo broken) in $PW_DRIVER_DIR"
    echo "  browsers : $(ls "$PW_BROWSERS_DIR" 2>/dev/null | tr '\n' ' ')"
  else
    echo "  Playwright is not installed. Run: sudo $0 browser enable"
  fi
}

browser_enable() {
  need_root "browser"
  local channel pages
  channel="$(ask "Browser (chromium = Playwright's build, chrome = Google Chrome, x86_64 only)" "$(browser_channel)")"
  case "$channel" in chromium|chrome) ;; *) die "choose chromium or chrome" ;; esac
  pages="$(ask "Browser pages open at once (each needs 100-300 MB RAM)" "$(v=$(env_get APP_BROWSER_MAX_PAGES); echo "${v:-3}")")"
  [[ "$pages" =~ ^[0-9]+$ ]] && [ "$pages" -ge 1 ] && [ "$pages" -le 50 ] || die "invalid page count: $pages"
  local def=y window
  [ "$(env_get APP_BROWSER_HEADLESS)" = "true" ] && def=n
  echo "Pages that only render in a real browser may need Chrome in a window (headless Chrome is detected more often);"
  echo "on a server without a screen that window is drawn on Xvfb, a virtual display."
  if confirm "Run Chrome in a window on Xvfb (recommended)?" "$def"; then window=false; else window=true; fi
  env_set APP_BROWSER_ENABLED true
  env_set APP_BROWSER_CHANNEL "$channel"
  env_set APP_BROWSER_MAX_PAGES "$pages"
  env_set APP_BROWSER_HEADLESS "$window"
  install_playwright
  # Rewrite the units so the service gets (or drops) the Xvfb display.
  [ -f "$SERVICE_FILE" ] && install_service
  ok "browser engine enabled. Restart the service to apply: $0 restart"
}

browser_disable() {
  need_root "browser"
  env_set APP_BROWSER_ENABLED false
  fix_perms
  [ -f "$SERVICE_FILE" ] && install_service
  ok "browser engine disabled (Playwright stays in $PW_DIR). Restart the service to apply: $0 restart"
}

# sites_need_browser -> success when a site config selects fetch.engine "browser"
sites_need_browser() {
  grep -qE '"engine"[[:space:]]*:[[:space:]]*"browser"' "$CONFIG_DIR"/sites/*.json 2>/dev/null
}

setup_browser() {
  title "Browser fetch engine"
  local def=n
  { browser_enabled || sites_need_browser; } && def=y
  sites_need_browser && echo "Some site configs set \"fetch\": {\"engine\": \"browser\"}; they fail with ENGINE_UNAVAILABLE without it."
  if confirm "Enable the browser engine (real Chrome via Playwright; ~1 GB disk, more RAM)?" "$def"; then
    browser_enable
  else
    browser_enabled && browser_disable
    ok "browser engine stays off"
  fi
}

# ================================================================ 3c. search engines
search_engine_enabled() { [ "$(env_get "APP_$(printf '%s' "$1" | tr '[:lower:]' '[:upper:]')_ENABLED")" = "true" ]; }

# sites_using_search -> config files whose fetch.engine or alternate_engines names a search engine
sites_using_search() {
  grep -lE '"(engine|alternate_engines)"[[:space:]]*:[[:space:]]*\[?[^]]*"(brave|duckduckgo|mojeek|yahoo|yandex)"' "$CONFIG_DIR"/sites/*.json 2>/dev/null | xargs -r -n1 basename || true
}

# Search engines that read a results page without a key (Mojeek, Yahoo and
# Yandex, through github.com/jcalvert/metawebsearch).
SCRAPED_ENGINES="mojeek yahoo yandex"
scraped_engine_label() {
  case "$1" in
    mojeek) echo "Mojeek (own index)" ;;
    yahoo)  echo "Yahoo (Bing's index)" ;;
    yandex) echo "Yandex (own index)" ;;
  esac
}
scraped_engine_terms() {
  case "$1" in
    mojeek) echo "https://www.mojeek.com/about/terms/" ;;
    yahoo)  echo "https://legal.yahoo.com/" ;;
    yandex) echo "https://yandex.com/legal/" ;;
  esac
}

search_status() {
  local key="not set"
  [ -n "$(env_get APP_BRAVE_API_KEY)" ] && key="set (hidden)"
  [ -z "$(env_get APP_BRAVE_API_KEY)" ] && [ -n "$(env_get APP_BRAVE_API_KEY_FILE)" ] && key="from file $(env_get APP_BRAVE_API_KEY_FILE)"
  if search_engine_enabled duckduckgo; then
    echo "Search engine duckduckgo: ENABLED (min interval $(v=$(env_get APP_DUCKDUCKGO_MIN_INTERVAL_MS); echo "${v:-2000}") ms, no key)"
  else
    echo "Search engine duckduckgo: off"
  fi
  if search_engine_enabled brave; then
    echo "Search engine brave     : ENABLED (min interval $(v=$(env_get APP_BRAVE_MIN_INTERVAL_MS); echo "${v:-1000}") ms, API key $key)"
  else
    echo "Search engine brave     : off (API key $key)"
  fi
  local e var
  for e in $SCRAPED_ENGINES; do
    var="APP_$(printf '%s' "$e" | tr '[:lower:]' '[:upper:]')_MIN_INTERVAL_MS"
    if search_engine_enabled "$e"; then
      printf 'Search engine %-10s: ENABLED (min interval %s ms, no key)\n' "$e" "$(v=$(env_get "$var"); echo "${v:-3000}")"
    else
      printf 'Search engine %-10s: off\n' "$e"
    fi
  done
  local sites; sites="$(sites_using_search | tr '\n' ' ')"
  [ -n "$sites" ] && echo "  sites using a search engine: $sites"
  return 0
}

search_enable() {
  need_root "search"
  local engine="${1:-}"
  [ -n "$engine" ] || engine="$(ask "Which search engine (brave, duckduckgo, mojeek, yahoo or yandex)" "brave")"
  case "$engine" in
    mojeek|yahoo|yandex)
      echo "The $engine engine reads $(scraped_engine_label "$engine")'s public results page with the honest"
      echo "APP_SEARCH_USER_AGENT; there is no API, no key and no SLA, and the provider has not approved this"
      echo "project. Check that your use fits its terms: $(scraped_engine_terms "$engine")"
      confirm "Enable the $engine engine?" n || { ok "$engine stays off"; return; }
      env_set "APP_$(printf '%s' "$engine" | tr '[:lower:]' '[:upper:]')_ENABLED" true
      local ivar="APP_$(printf '%s' "$engine" | tr '[:lower:]' '[:upper:]')_MIN_INTERVAL_MS"
      [ -n "$(env_get "$ivar")" ] || env_set "$ivar" 3000
      ;;
    duckduckgo)
      echo "DuckDuckGo has no API for web results and has not approved this project; the engine reads its"
      echo "HTML results page, which its robots.txt allows. Check that your use fits DuckDuckGo's terms:"
      echo "  https://duckduckgo.com/terms  https://duckduckgo.com/acceptable-use"
      confirm "Enable the duckduckgo engine?" n || { ok "duckduckgo stays off"; return; }
      env_set APP_DUCKDUCKGO_ENABLED true
      [ -n "$(env_get APP_DUCKDUCKGO_MIN_INTERVAL_MS)" ] || env_set APP_DUCKDUCKGO_MIN_INTERVAL_MS 2000
      ;;
    brave)
      echo "The brave engine needs your own Brave Search API key (https://brave.com/search/api)."
      echo "Check that your plan covers how your clients use and store the results."
      local key=""
      if [ -n "$(env_get APP_BRAVE_API_KEY)" ] && confirm "A Brave API key is already set. Keep it?" y; then
        key="$(env_get APP_BRAVE_API_KEY)"
      else
        read -r -s -p "Brave API key (input hidden): " key </dev/tty; echo
        key="$(printf '%s' "$key" | tr -d '[:space:]')"
      fi
      [ -n "$key" ] || die "no key entered; brave stays off"
      env_set APP_BRAVE_API_KEY "$key"
      env_set APP_BRAVE_ENABLED true
      [ -n "$(env_get APP_BRAVE_MIN_INTERVAL_MS)" ] || env_set APP_BRAVE_MIN_INTERVAL_MS 1000
      ;;
    *) die "choose brave, duckduckgo, mojeek, yahoo or yandex" ;;
  esac
  [ -n "$(env_get APP_SEARCH_USER_AGENT)" ] || env_set APP_SEARCH_USER_AGENT "go-wordpress-extractor/1.0"
  fix_perms
  ok "$engine engine enabled. Restart the service to apply: $0 restart"
}

search_disable() {
  need_root "search"
  local engine="${1:-}"
  [ -n "$engine" ] || engine="$(ask "Which search engine to turn off (brave, duckduckgo, mojeek, yahoo or yandex)" "")"
  case "$engine" in
    duckduckgo) env_set APP_DUCKDUCKGO_ENABLED false ;;
    mojeek|yahoo|yandex) env_set "APP_$(printf '%s' "$engine" | tr '[:lower:]' '[:upper:]')_ENABLED" false ;;
    brave)
      env_set APP_BRAVE_ENABLED false
      confirm "Also remove the Brave API key from $ENV_FILE?" n && env_set APP_BRAVE_API_KEY ""
      ;;
    *) die "choose brave, duckduckgo, mojeek, yahoo or yandex" ;;
  esac
  fix_perms
  ok "$engine engine disabled. Restart the service to apply: $0 restart"
}

setup_search() {
  title "Search engines"
  local sites; sites="$(sites_using_search | tr '\n' ' ')"
  if [ -n "$sites" ]; then
    echo "These site configs use a search engine (fetch.engine, then alternate_engines): $sites"
    echo "Without one of their engines enabled they fail with ENGINE_UNAVAILABLE."
  fi
  local ddg_def=n brave_def=n
  { search_engine_enabled duckduckgo || [ -n "$sites" ]; } && ddg_def=y
  search_engine_enabled brave && brave_def=y
  if confirm "Enable the duckduckgo engine (no key; check DuckDuckGo's terms first)?" "$ddg_def"; then
    search_enable duckduckgo
  fi
  if confirm "Enable the brave engine (needs your own Brave Search API key)?" "$brave_def"; then
    search_enable brave
  fi
  local e e_def
  for e in $SCRAPED_ENGINES; do
    e_def=n; search_engine_enabled "$e" && e_def=y
    if confirm "Enable the $e engine (no key; reads $(scraped_engine_label "$e")'s results page; check its terms first)?" "$e_def"; then
      search_enable "$e"
    fi
  done
  search_status
}

# ================================================================ 3c. AI crawler
# Two AI engines fetch the product page and extract it with an LLM:
#   ai-crawler-avalai      AvalAI's OpenAI-compatible API
#   ai-crawler-cloudflare  Cloudflare AI Gateway (default model DeepSeek V4.1
#                          Flash, hosted by Cloudflare)
# APP_DEFAULT_ENGINE picks the one that serves every host without a rule (or
# none), and learning uses it to learn CSS selectors per host.
AI_KEY_FILE="$SECRETS_DIR/avalai_api_key"
CF_TOKEN_FILE="$SECRETS_DIR/cloudflare_api_token"
CF_DEFAULT_MODEL="@cf/deepseek-ai/deepseek-v4.1-flash-1017"
LEARNED_DIR="$CONFIG_DIR/learned"

ai_enabled() { [ "$(env_get APP_AI_CRAWLER_ENABLED)" = "true" ]; }

# avalai_configured / cloudflare_configured mirror the server's rule: the
# engine's own settings are complete.
avalai_configured() {
  [ -n "$(env_get APP_AI_CRAWLER_API_KEY)" ] || [ -s "$(env_get APP_AI_CRAWLER_API_KEY_FILE)" ]
}
cloudflare_configured() {
  [ -n "$(env_get APP_AI_CRAWLER_CLOUDFLARE_ACCOUNT_ID)" ] &&
    { [ -n "$(env_get APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN)" ] || [ -s "$(env_get APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN_FILE)" ]; }
}

# default_engine prints APP_DEFAULT_ENGINE as the server resolves it.
default_engine() {
  local v; v="$(env_get APP_DEFAULT_ENGINE)"
  if [ -n "$v" ]; then printf '%s' "$v"; elif ai_enabled; then printf 'ai-crawler-avalai'; else printf 'none'; fi
}

# ai_migrate_default_rule replaces APP_AI_CRAWLER_DEFAULT_RULE (removed; the
# server refuses to start while it is set) with APP_DEFAULT_ENGINE.
ai_migrate_default_rule() {
  [ -f "$ENV_FILE" ] && grep -qE '^APP_AI_CRAWLER_DEFAULT_RULE=' "$ENV_FILE" || return 0
  local old engine; old="$(env_get APP_AI_CRAWLER_DEFAULT_RULE)"
  if [ "$old" = "false" ]; then engine=none
  elif ai_enabled; then engine="$(v=$(env_get APP_DEFAULT_ENGINE); echo "${v:-ai-crawler-avalai}")"
  else engine=none; fi
  if [ ! -w "$ENV_FILE" ]; then
    warn "$ENV_FILE still sets APP_AI_CRAWLER_DEFAULT_RULE, which was replaced by APP_DEFAULT_ENGINE; the server will not start. Run: sudo $0 ai"
    return 0
  fi
  local tmp; tmp="$(mktemp)"
  grep -vE '^APP_AI_CRAWLER_DEFAULT_RULE=' "$ENV_FILE" >"$tmp" || true
  cat "$tmp" >"$ENV_FILE"; rm -f "$tmp"
  env_set APP_DEFAULT_ENGINE "$engine"
  ok "replaced APP_AI_CRAWLER_DEFAULT_RULE=$old with APP_DEFAULT_ENGINE=$engine in $ENV_FILE"
}

sites_using_ai() {
  grep -lE '"engine"[[:space:]]*:[[:space:]]*"ai-crawler-(avalai|cloudflare)"' "$CONFIG_DIR"/sites/*.json 2>/dev/null | xargs -r -n1 basename || true
}

secret_source() {
  # secret_source VAR FILE_VAR -> where the secret comes from, never the secret
  if [ -n "$(env_get "$1")" ]; then echo "set in $ENV_FILE (hidden)"
  elif [ -s "$(env_get "$2")" ]; then echo "from file $(env_get "$2")"
  else echo "not set"; fi
}

ai_status() {
  ai_migrate_default_rule
  local state="off"; ai_enabled && state="ENABLED"
  echo "AI crawler (APP_AI_CRAWLER_ENABLED): $state"
  local a="not configured" c="not configured"
  ai_enabled && avalai_configured && a="ON"
  ai_enabled && cloudflare_configured && c="ON"
  echo "  ai-crawler-avalai     : $a"
  echo "    base URL           : $(v=$(env_get APP_AI_CRAWLER_BASE_URL); echo "${v:-https://api.avalai.ir/v1}")"
  echo "    model              : $(v=$(env_get APP_AI_CRAWLER_MODEL); echo "${v:-deepseek-v4.1-flash}")"
  echo "    API key            : $(secret_source APP_AI_CRAWLER_API_KEY APP_AI_CRAWLER_API_KEY_FILE)"
  echo "  ai-crawler-cloudflare : $c"
  echo "    account / gateway  : $(v=$(env_get APP_AI_CRAWLER_CLOUDFLARE_ACCOUNT_ID); echo "${v:-not set}") / $(v=$(env_get APP_AI_CRAWLER_CLOUDFLARE_GATEWAY_ID); echo "${v:-default}")"
  echo "    endpoint           : $(v=$(env_get APP_AI_CRAWLER_CLOUDFLARE_ENDPOINT); echo "${v:-rest}")"
  echo "    model              : $(v=$(env_get APP_AI_CRAWLER_CLOUDFLARE_MODEL); echo "${v:-$CF_DEFAULT_MODEL}")"
  echo "    API token          : $(secret_source APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN_FILE)"
  echo "  default engine        : $(default_engine) (APP_DEFAULT_ENGINE: hosts without a rule)"
  if ai_enabled; then
    echo "  learning              : $(v=$(env_get APP_AI_CRAWLER_LEARN_SELECTORS); [ "${v:-true}" = "true" ] && [ "$(default_engine)" != none ] && echo "on, with $(default_engine) (learned configs in $LEARNED_DIR)" || echo "off")"
  fi
  local sites; sites="$(sites_using_ai | tr '\n' ' ')"
  [ -n "$sites" ] && echo "  sites using an AI engine: $sites"
  if [ -x "$BIN_DIR/sitectl" ] && ls "$LEARNED_DIR"/*.json >/dev/null 2>&1; then
    echo "  learned configs:"
    "$BIN_DIR/sitectl" learned list --dir "$CONFIG_DIR" --learned-dir "$LEARNED_DIR" | sed 's/^/    /' || true
  fi
  return 0
}

# ai_common turns the AI crawler on and asks about learning.
ai_common() {
  env_set APP_AI_CRAWLER_ENABLED true
  local learn_def=y
  [ "$(env_get APP_AI_CRAWLER_LEARN_SELECTORS)" = "false" ] && learn_def=n
  if confirm "Learn CSS selectors from AI results, so proven hosts need no LLM calls?" "$learn_def"; then
    env_set APP_AI_CRAWLER_LEARN_SELECTORS true
  else
    env_set APP_AI_CRAWLER_LEARN_SELECTORS false
  fi
  env_set APP_AI_CRAWLER_LEARN_DIR "$LEARNED_DIR"
  # The service writes learned configs; nothing else under configs/ is writable.
  mkdir -p "$LEARNED_DIR"
  if id "$SERVICE_USER" >/dev/null 2>&1; then chown "$SERVICE_USER:$SERVICE_USER" "$LEARNED_DIR"; fi
  chmod 755 "$LEARNED_DIR"
}

# ask_default_engine engine -> offers engine as the default for hosts without a rule.
ask_default_engine() {
  local engine="$1" def=n
  [ "$(default_engine)" = "$engine" ] && def=y
  { [ "$(default_engine)" = none ] || [ -z "$(env_get APP_DEFAULT_ENGINE)" ]; } && [ "$engine" = ai-crawler-avalai ] && def=y
  if confirm "Serve every host without a rule with $engine (APP_DEFAULT_ENGINE)?" "$def"; then
    env_set APP_DEFAULT_ENGINE "$engine"
  elif [ -z "$(env_get APP_DEFAULT_ENGINE)" ] || [ "$(default_engine)" = "$engine" ]; then
    env_set APP_DEFAULT_ENGINE none
  fi
}

ai_enable_avalai() {
  echo "ai-crawler-avalai sends the content of every product page it serves, and the page URL, to"
  echo "AvalAI and its upstream model provider. Check that this fits your provider agreement and the"
  echo "sites' terms. Every value is checked against the page before it is returned."
  confirm "Enable ai-crawler-avalai?" n || { ok "ai-crawler-avalai stays off"; return; }

  local base model key=""
  base="$(ask "OpenAI-compatible base URL" "$(v=$(env_get APP_AI_CRAWLER_BASE_URL); echo "${v:-https://api.avalai.ir/v1}")")"
  case "$base" in https://*) ;; *) die "the base URL must start with https://" ;; esac
  model="$(ask "Model" "$(v=$(env_get APP_AI_CRAWLER_MODEL); echo "${v:-deepseek-v4.1-flash}")")"
  [ -n "$model" ] || die "the model must not be empty"

  if [ -s "$AI_KEY_FILE" ] && confirm "An AI API key is already in $AI_KEY_FILE. Keep it?" y; then
    :
  else
    read -r -s -p "AvalAI API key (input hidden): " key </dev/tty; echo
    key="$(printf '%s' "$key" | tr -d '[:space:]')"
    [ -n "$key" ] || die "no key entered; ai-crawler-avalai stays off"
    mkdir -p "$SECRETS_DIR"
    ( umask 077; printf '%s\n' "$key" >"$AI_KEY_FILE" )
  fi
  env_set APP_AI_CRAWLER_BASE_URL "$base"
  env_set APP_AI_CRAWLER_MODEL "$model"
  env_set APP_AI_CRAWLER_API_KEY ""
  env_set APP_AI_CRAWLER_API_KEY_FILE "$AI_KEY_FILE"
  ai_common
  ask_default_engine ai-crawler-avalai
  fix_perms
  ok "ai-crawler-avalai enabled (default engine: $(default_engine)). Restart the service to apply: $0 restart"
}

ai_enable_cloudflare() {
  echo "ai-crawler-cloudflare sends the content of every product page it serves, and the page URL, to"
  echo "Cloudflare AI Gateway and the model provider behind it. With the default model (DeepSeek V4.1"
  echo "Flash, $CF_DEFAULT_MODEL) the model runs on Cloudflare (Workers AI) and is billed to the"
  echo "Cloudflare account (Workers Paid plan or AI Gateway credits)."
  echo "The token needs the \"Workers AI Read\" and \"AI Gateway Run\" permissions. AI Gateway Run works on"
  echo "every gateway of the account: create a dedicated token."
  confirm "Enable ai-crawler-cloudflare?" n || { ok "ai-crawler-cloudflare stays off"; return; }

  local account gateway endpoint model token=""
  account="$(ask "Cloudflare account id (32 hex characters, dashboard account home)" "$(env_get APP_AI_CRAWLER_CLOUDFLARE_ACCOUNT_ID)")"
  account="$(printf '%s' "$account" | tr -d '[:space:]' | tr '[:upper:]' '[:lower:]')"
  [[ "$account" =~ ^[0-9a-f]{32}$ ]] || die "the account id must be 32 hex characters"
  gateway="$(ask "Gateway id" "$(v=$(env_get APP_AI_CRAWLER_CLOUDFLARE_GATEWAY_ID); echo "${v:-default}")")"
  [ -n "$gateway" ] && [ "${#gateway}" -le 64 ] || die "the gateway id must be 1 to 64 characters"
  endpoint="$(ask "Endpoint: rest (recommended) or compat" "$(v=$(env_get APP_AI_CRAWLER_CLOUDFLARE_ENDPOINT); echo "${v:-rest}")")"
  case "$endpoint" in rest|compat) ;; *) die "the endpoint must be rest or compat" ;; esac
  local model_def; model_def="$(v=$(env_get APP_AI_CRAWLER_CLOUDFLARE_MODEL); echo "${v:-$CF_DEFAULT_MODEL}")"
  [ "$endpoint" = compat ] && [ "$model_def" = "$CF_DEFAULT_MODEL" ] && model_def="workers-ai/$CF_DEFAULT_MODEL"
  model="$(ask "Model (exact id from the Cloudflare catalog)" "$model_def")"
  [ -n "$model" ] || die "the model must not be empty"

  if [ -s "$CF_TOKEN_FILE" ] && confirm "A Cloudflare token is already in $CF_TOKEN_FILE. Keep it?" y; then
    :
  else
    read -r -s -p "Cloudflare API token (input hidden): " token </dev/tty; echo
    token="$(printf '%s' "$token" | tr -d '[:space:]')"
    [ -n "$token" ] || die "no token entered; ai-crawler-cloudflare stays off"
    mkdir -p "$SECRETS_DIR"
    ( umask 077; printf '%s\n' "$token" >"$CF_TOKEN_FILE" )
  fi
  env_set APP_AI_CRAWLER_CLOUDFLARE_ACCOUNT_ID "$account"
  env_set APP_AI_CRAWLER_CLOUDFLARE_GATEWAY_ID "$gateway"
  env_set APP_AI_CRAWLER_CLOUDFLARE_ENDPOINT "$endpoint"
  env_set APP_AI_CRAWLER_CLOUDFLARE_MODEL "$model"
  env_set APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN ""
  env_set APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN_FILE "$CF_TOKEN_FILE"
  [ -n "$(env_get APP_AI_CRAWLER_CLOUDFLARE_NO_UNIFIED_BILLING)" ] || env_set APP_AI_CRAWLER_CLOUDFLARE_NO_UNIFIED_BILLING true
  [ -n "$(env_get APP_AI_CRAWLER_CLOUDFLARE_COLLECT_LOG)" ] || env_set APP_AI_CRAWLER_CLOUDFLARE_COLLECT_LOG false
  ai_common
  ask_default_engine ai-crawler-cloudflare
  fix_perms
  ok "ai-crawler-cloudflare enabled (default engine: $(default_engine)). Restart the service to apply: $0 restart"
}

# ai_enable [avalai|cloudflare]
ai_enable() {
  need_root "ai enable"
  ai_migrate_default_rule
  local which="${1:-}"
  if [ -z "$which" ]; then
    which="$(ask "Which AI engine: avalai or cloudflare" avalai)"
  fi
  case "$which" in
    avalai|ai-crawler-avalai)         ai_enable_avalai ;;
    cloudflare|ai-crawler-cloudflare) ai_enable_cloudflare ;;
    *) die "unknown AI engine \"$which\": use avalai or cloudflare" ;;
  esac
}

# recommended_engine: configured AvalAI first (the existing default;
# Cloudflare's live check, proposal step 1, is still open), then Cloudflare,
# then none.
recommended_engine() {
  if ai_enabled && avalai_configured; then echo ai-crawler-avalai
  elif ai_enabled && cloudflare_configured; then echo ai-crawler-cloudflare
  else echo none; fi
}

# ask_engine_from_list prints the engines that can serve websites without a
# rule, in recommended order, to stderr and echoes the choice.
ask_engine_from_list() {
  local cur rec av=no cf=no c
  cur="$(default_engine)"; rec="$(recommended_engine)"
  ai_enabled && avalai_configured && av=yes
  ai_enabled && cloudflare_configured && cf=yes
  tag() { local t=""; [ "$1" = "$rec" ] && t=" (recommended)"; [ "$1" = "$cur" ] && t="$t (current)"; printf '%s' "$t"; }
  {
    echo "Engine for websites with no site config and no active learned config, in recommended order:"
    echo "  1) ai-crawler-avalai$(tag ai-crawler-avalai)      configured: $av. AvalAI, $(v=$(env_get APP_AI_CRAWLER_MODEL); echo "${v:-deepseek-v4.1-flash}")"
    echo "  2) ai-crawler-cloudflare$(tag ai-crawler-cloudflare)  configured: $cf. Cloudflare AI Gateway, $(v=$(env_get APP_AI_CRAWLER_CLOUDFLARE_MODEL); echo "${v:-$CF_DEFAULT_MODEL}") (about \$3.60 per 1,000 pages)"
    echo "  3) none$(tag none)  unknown websites fail with SITE_NOT_CONFIGURED; nothing goes to an LLM"
    echo "  (http, browser and the search engines need a site config's selectors, so they are not offered)"
  } >&2
  c="$(ask "Choose 1-3" "$cur")"
  case "$c" in
    1) echo ai-crawler-avalai ;; 2) echo ai-crawler-cloudflare ;; 3) echo none ;; *) echo "$c" ;;
  esac
}

# ai_set_default_engine [ai-crawler-avalai|ai-crawler-cloudflare|none]; with
# no argument it shows the recommended list.
ai_set_default_engine() {
  need_root "default-engine"
  ai_migrate_default_rule
  local engine="${1:-}"
  [ -n "$engine" ] || engine="$(ask_engine_from_list)"
  case "$engine" in
    avalai) engine=ai-crawler-avalai ;;
    cloudflare) engine=ai-crawler-cloudflare ;;
  esac
  case "$engine" in
    none) ;;
    ai-crawler-avalai)
      ai_enabled && avalai_configured || die "ai-crawler-avalai is not configured. Run: sudo $0 ai enable avalai" ;;
    ai-crawler-cloudflare)
      ai_enabled && cloudflare_configured || die "ai-crawler-cloudflare is not configured. Run: sudo $0 ai enable cloudflare" ;;
    *) die "usage: $0 default-engine ai-crawler-avalai|ai-crawler-cloudflare|none" ;;
  esac
  env_set APP_DEFAULT_ENGINE "$engine"
  ok "APP_DEFAULT_ENGINE=$engine. Restart the service to apply: $0 restart"
}

# ai_disable_engine avalai|cloudflare turns one engine off by clearing its
# credentials; the other engine keeps running. When the engine was the
# default, APP_DEFAULT_ENGINE moves to the other engine or to none.
ai_disable_engine() {
  local which="$1" engine file
  case "$which" in
    avalai|ai-crawler-avalai)
      engine=ai-crawler-avalai; file="$AI_KEY_FILE"
      env_set APP_AI_CRAWLER_API_KEY ""
      env_set APP_AI_CRAWLER_API_KEY_FILE "" ;;
    cloudflare|ai-crawler-cloudflare)
      engine=ai-crawler-cloudflare; file="$CF_TOKEN_FILE"
      env_set APP_AI_CRAWLER_CLOUDFLARE_ACCOUNT_ID ""
      env_set APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN "" ;;
    *) die "unknown AI engine \"$which\": use avalai or cloudflare" ;;
  esac
  if [ -f "$file" ] && confirm "Also delete $file?" n; then rm -f "$file"; fi
  if [ "$(default_engine)" = "$engine" ]; then
    local other=none
    if [ "$engine" = ai-crawler-avalai ] && cloudflare_configured; then other=ai-crawler-cloudflare; fi
    if [ "$engine" = ai-crawler-cloudflare ] && avalai_configured; then other=ai-crawler-avalai; fi
    env_set APP_DEFAULT_ENGINE "$other"
    info "APP_DEFAULT_ENGINE was $engine; it is now $other"
  fi
  if ! avalai_configured && ! cloudflare_configured; then
    env_set APP_AI_CRAWLER_ENABLED false
    env_set APP_DEFAULT_ENGINE none
    info "no AI engine is left; the AI crawler is off"
  fi
  local sites; sites="$(grep -lE "\"engine\"[[:space:]]*:[[:space:]]*\"$engine\"" "$CONFIG_DIR"/sites/*.json 2>/dev/null | xargs -r -n1 basename | tr '\n' ' ' || true)"
  [ -n "$sites" ] && warn "these site configs use $engine and will fail with ENGINE_UNAVAILABLE: $sites"
  fix_perms
  ok "$engine disabled. Restart to apply: $0 restart"
}

# ai_disable [avalai|cloudflare]: one engine, or the whole AI crawler.
ai_disable() {
  need_root "ai disable"
  ai_migrate_default_rule
  if [ -n "${1:-}" ]; then ai_disable_engine "$1"; return; fi
  env_set APP_AI_CRAWLER_ENABLED false
  env_set APP_DEFAULT_ENGINE none
  if [ -f "$AI_KEY_FILE" ] && confirm "Also delete the AvalAI key file $AI_KEY_FILE?" n; then
    rm -f "$AI_KEY_FILE"
  fi
  if [ -f "$CF_TOKEN_FILE" ] && confirm "Also delete the Cloudflare token file $CF_TOKEN_FILE?" n; then
    rm -f "$CF_TOKEN_FILE"
  fi
  fix_perms
  ok "AI crawler disabled; unknown hosts fail with SITE_NOT_CONFIGURED again. Learned configs in $LEARNED_DIR are kept but not used. Restart to apply: $0 restart"
}

# ai_preflight checks what the server checks at startup about the AI
# engines, so a wrong setting is reported before the service is restarted.
ai_preflight() {
  [ -f "$ENV_FILE" ] || return 0
  local d; d="$(default_engine)"
  case "$d" in
    none) return 0 ;;
    ai-crawler-avalai|ai-crawler-cloudflare) ;;
    *) die "APP_DEFAULT_ENGINE \"$d\" is not valid: use ai-crawler-avalai, ai-crawler-cloudflare or none ($0 default-engine ...)" ;;
  esac
  ai_enabled || die "APP_DEFAULT_ENGINE is $d, but APP_AI_CRAWLER_ENABLED is not true. Run: sudo $0 default-engine none, or sudo $0 ai enable"
  if [ "$d" = ai-crawler-avalai ] && ! avalai_configured; then
    die "APP_DEFAULT_ENGINE is ai-crawler-avalai, but no AvalAI key is set. Run: sudo $0 ai enable avalai, or sudo $0 default-engine none"
  fi
  if [ "$d" = ai-crawler-cloudflare ] && ! cloudflare_configured; then
    die "APP_DEFAULT_ENGINE is ai-crawler-cloudflare, but the Cloudflare account id or token is missing. Run: sudo $0 ai enable cloudflare, or sudo $0 default-engine none"
  fi
  return 0
}

setup_ai() {
  title "AI crawler"
  ai_migrate_default_rule
  local sites; sites="$(sites_using_ai | tr '\n' ' ')"
  if [ -n "$sites" ]; then
    echo "These site configs use an AI engine (fetch.engine \"ai-crawler-avalai\" or \"ai-crawler-cloudflare\"): $sites"
    echo "Without it they fail with ENGINE_UNAVAILABLE."
  fi
  local def=n
  { ai_enabled || [ -n "$sites" ]; } && def=y
  if confirm "Enable the AvalAI engine (ai-crawler-avalai; needs an AvalAI API key)?" "$def"; then
    ai_enable_avalai
  fi
  local cf_def=n
  { ai_enabled && cloudflare_configured; } && cf_def=y
  if confirm "Enable the Cloudflare engine (ai-crawler-cloudflare; needs a Cloudflare account id and token)?" "$cf_def"; then
    ai_enable_cloudflare
  fi
  ai_status
}

# ai_learned runs "sitectl learned <action> [domain]" on the learned configs.
ai_learned() {
  [ -x "$BIN_DIR/sitectl" ] || die "bin/sitectl is missing. Run: sudo $0 build"
  local action="${1:-list}"; shift || true
  case "$action" in
    activate|reject|adopt) need_root "learned" ;;
  esac
  "$BIN_DIR/sitectl" learned "$action" --dir "$CONFIG_DIR" --learned-dir "$LEARNED_DIR" "$@"
  if [ "$(id -u)" -eq 0 ] && id "$SERVICE_USER" >/dev/null 2>&1 && [ -d "$LEARNED_DIR" ]; then
    chown -R "$SERVICE_USER:$SERVICE_USER" "$LEARNED_DIR"
  fi
  [ "$action" = "adopt" ] && echo "Restart to load the adopted config: $0 restart"
  return 0
}

# ================================================================ 4. env file
setup_env() {
  need_root "env"
  ai_migrate_default_rule
  if [ -f "$ENV_FILE" ] && ! confirm "$ENV_FILE exists. Review its server settings again?" n; then
    ok "keeping $ENV_FILE"; return
  fi
  title "Server settings"
  local addr port proxy_host proxy_port
  addr="$(ask "Listen address (0.0.0.0 = all interfaces, 127.0.0.1 = behind a reverse proxy)" "$(env_get APP_HTTP_ADDRESS || true)")"
  [ -n "$addr" ] || addr="0.0.0.0"
  port="$(ask "HTTP port" "$(v=$(env_get APP_HTTP_PORT); echo "${v:-4000}")")"
  [[ "$port" =~ ^[0-9]+$ ]] && [ "$port" -ge 1 ] && [ "$port" -le 65535 ] || die "invalid port: $port"
  proxy_host="$(ask "Outbound HTTP proxy host (empty = fetch directly)" "$(env_get APP_PROXY_HOST)")"
  proxy_port=""
  [ -n "$proxy_host" ] && proxy_port="$(ask "Outbound HTTP proxy port" "$(env_get APP_PROXY_PORT)")"

  if [ ! -f "$ENV_FILE" ]; then
    cat >"$ENV_FILE" <<EOF
# Production settings for go-wordpress-extractor. Managed by deploy.sh.
# Keep this file secret: it holds the pepper every key hash depends on.

# Server
APP_ENV=production
APP_HTTP_PORT=4000
APP_HTTP_ADDRESS=0.0.0.0

# Site configs and API keys
APP_SITE_CONFIG_DIR=$CONFIG_DIR
APP_API_KEYS_FILE=$KEYS_FILE
APP_API_KEY_PEPPER=

# Swagger UI and /metrics. Both empty = disabled (404).
# Turn on with: sudo ./deploy.sh swagger enable
APP_OPS_USER=
APP_OPS_PASSWORD_HASH=

# TLS. Leave empty when a reverse proxy terminates HTTPS.
APP_TLS_CERT_FILE=
APP_TLS_KEY_FILE=

# Limits
APP_MAX_REQUEST_DURATION=3m
APP_SITE_WAIT_TIMEOUT=30s
APP_MAX_CONCURRENT_REQUESTS=20
APP_MAX_CONCURRENT_PER_SITE=1

# Outbound HTTP proxy for page fetches. Leave both empty to fetch directly.
APP_PROXY_HOST=
APP_PROXY_PORT=

# Browser fetch engine (real Chrome via Playwright) for sites whose config
# sets "fetch": {"engine": "browser"}. Turn on with: sudo ./deploy.sh browser enable
APP_BROWSER_ENABLED=false
APP_BROWSER_CHANNEL=chromium
APP_BROWSER_MAX_PAGES=3
# false = Chrome in a window on the Xvfb virtual display (needed for Cloudflare).
APP_BROWSER_HEADLESS=true

# Search engines for sites whose config sets "fetch": {"engine": "brave"},
# "duckduckgo", "mojeek", "yahoo" or "yandex". They never contact the
# product's site; they return what the search result shows and list the rest
# in unavailable_fields.
# Turn on with: sudo ./deploy.sh search enable brave|duckduckgo|mojeek|yahoo|yandex
APP_SEARCH_USER_AGENT=go-wordpress-extractor/1.0
APP_SEARCH_CACHE_TTL=6h
APP_SEARCH_NO_RESULT_CACHE_TTL=1h
# No key needed. Enable only if your use fits DuckDuckGo's terms (README, "Responsible use").
APP_DUCKDUCKGO_ENABLED=false
APP_DUCKDUCKGO_MIN_INTERVAL_MS=2000
# Needs your own Brave Search API key. Secret: this file is mode 640/600 and not in git.
APP_BRAVE_ENABLED=false
APP_BRAVE_API_KEY=
APP_BRAVE_MIN_INTERVAL_MS=1000
# No key needed: the engine's results page through github.com/jcalvert/metawebsearch.
# Enable only the providers whose terms fit your use (README, "Responsible use").
APP_MOJEEK_ENABLED=false
APP_MOJEEK_MIN_INTERVAL_MS=3000
APP_YAHOO_ENABLED=false
APP_YAHOO_MIN_INTERVAL_MS=3000
APP_YANDEX_ENABLED=false
APP_YANDEX_MIN_INTERVAL_MS=3000

# Engine for hosts without a rule: ai-crawler-avalai, ai-crawler-cloudflare
# or none (SITE_NOT_CONFIGURED). Set with: sudo ./deploy.sh default-engine <engine>
APP_DEFAULT_ENGINE=none

# AI crawler: extracts products with an LLM. Sends page content to the
# provider. Off by default. Engines:
#   ai-crawler-avalai: sudo ./deploy.sh ai enable avalai (key in secrets/avalai_api_key)
#   ai-crawler-cloudflare: sudo ./deploy.sh ai enable cloudflare (token in secrets/cloudflare_api_token)
APP_AI_CRAWLER_ENABLED=false
APP_AI_CRAWLER_BASE_URL=https://api.avalai.ir/v1
APP_AI_CRAWLER_MODEL=deepseek-v4.1-flash
APP_AI_CRAWLER_API_KEY=
APP_AI_CRAWLER_API_KEY_FILE=$AI_KEY_FILE
APP_AI_CRAWLER_CACHE_TTL=6h
APP_AI_CRAWLER_MIN_INTERVAL_MS=500
APP_AI_CRAWLER_LEARN_SELECTORS=true
APP_AI_CRAWLER_LEARN_DIR=$LEARNED_DIR
APP_AI_CRAWLER_CLOUDFLARE_ACCOUNT_ID=
APP_AI_CRAWLER_CLOUDFLARE_GATEWAY_ID=default
APP_AI_CRAWLER_CLOUDFLARE_ENDPOINT=rest
APP_AI_CRAWLER_CLOUDFLARE_MODEL=$CF_DEFAULT_MODEL
APP_AI_CRAWLER_CLOUDFLARE_REASONING_EFFORT=none
APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN=
APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN_FILE=$CF_TOKEN_FILE
APP_AI_CRAWLER_CLOUDFLARE_NO_UNIFIED_BILLING=true
APP_AI_CRAWLER_CLOUDFLARE_COLLECT_LOG=false
APP_AI_CRAWLER_CLOUDFLARE_MIN_INTERVAL_MS=500
EOF
  fi
  env_set APP_ENV production
  env_set APP_HTTP_ADDRESS "$addr"
  env_set APP_HTTP_PORT "$port"
  env_set APP_SITE_CONFIG_DIR "$CONFIG_DIR"
  [ -n "$(env_get APP_API_KEYS_FILE)" ] || env_set APP_API_KEYS_FILE "$KEYS_FILE"
  env_set APP_PROXY_HOST "$proxy_host"
  env_set APP_PROXY_PORT "$proxy_port"
  fix_perms
  ok "wrote $ENV_FILE"
}

# ================================================================ 5. keys
setup_pepper() {
  need_root "pepper"
  local cur; cur="$(env_get APP_API_KEY_PEPPER)"
  if [ "${#cur}" -ge 32 ]; then
    if ! confirm "A pepper is already set. Replace it? (every existing API key and the Swagger password stop working)" n; then
      ok "keeping the current pepper"; return
    fi
  fi
  env_set APP_API_KEY_PEPPER "$(apikey generate-pepper)"
  fix_perms
  ok "new pepper written to $ENV_FILE"
}

key_generate() {
  load_key_env
  local name expires
  name="${1:-$(ask "Client name (letters, digits, - _)" "")}"
  [[ "$name" =~ ^[A-Za-z0-9_-]+$ ]] || die "invalid client name: '$name'"
  expires="${2:-$(ask "Lifetime (e.g. 8760h, 720h, never)" "8760h")}"
  apikey generate --name "$name" --expires-in "$expires"
  fix_perms
  printf '%sCopy the gwp_... key above now. It is shown only once.%s\n' "$C_YLW" "$C_OFF"
}

key_bulk() {
  load_key_env
  local count prefix expires out
  count="$(ask "How many keys" "10")"
  prefix="$(ask "Name prefix (names become <prefix>-001, ...)" "app")"
  expires="$(ask "Lifetime (e.g. 8760h, never)" "8760h")"
  out="$SECRETS_DIR/keys-$(date +%Y%m%d-%H%M%S).csv"
  apikey generate-bulk --count "$count" --name-prefix "$prefix" --expires-in "$expires" --plain-output "$out"
  fix_perms
  warn "keys written to $out. Hand it over securely, then delete it: it is the only copy."
}

key_list()   { load_key_env; apikey list "$@"; }

key_revoke() {
  load_key_env
  apikey list
  local target="${1:-$(ask "Key id to revoke (or prefix:<name-prefix>)" "")}"
  [ -n "$target" ] || die "nothing to revoke"
  if [[ "$target" == prefix:* ]]; then
    confirm "Revoke every key whose name starts with '${target#prefix:}'?" n || return 0
    apikey revoke --name-prefix "${target#prefix:}"
  else
    confirm "Revoke key '$target'?" n || return 0
    apikey revoke --id "$target"
  fi
  fix_perms
}

keys_menu() {
  need_root "keys"
  while true; do
    title "API keys"
    echo "  1) Create a key"
    echo "  2) Create many keys (CSV)"
    echo "  3) List keys"
    echo "  4) Revoke a key"
    echo "  5) New pepper (invalidates all keys)"
    echo "  0) Done"
    case "$(ask "Choose" "0")" in
      1) key_generate ;;
      2) key_bulk ;;
      3) key_list ;;
      4) key_revoke ;;
      5) setup_pepper ;;
      0|q) break ;;
      *) warn "unknown choice" ;;
    esac
  done
}

# ================================================================ 6. Swagger
swagger_status() {
  if [ -n "$(env_get APP_OPS_USER)" ] && [ -n "$(env_get APP_OPS_PASSWORD_HASH)" ]; then
    echo "Swagger UI and /metrics: ENABLED (user: $(env_get APP_OPS_USER))"
  else
    echo "Swagger UI and /metrics: DISABLED"
  fi
}

swagger_enable() {
  need_root "swagger"
  load_key_env
  local user pw pw2 hash
  user="$(ask "Swagger user (APP_OPS_USER)" "$(env_get APP_OPS_USER)")"
  [[ "$user" =~ ^[A-Za-z0-9._-]+$ ]] || die "invalid user name: '$user'"
  read -r -s -p "Swagger password (at least 12 characters): " pw </dev/tty; echo
  read -r -s -p "Repeat password: " pw2 </dev/tty; echo
  [ "$pw" = "$pw2" ] || die "passwords do not match"
  [ "${#pw}" -ge 12 ] || die "password must be at least 12 characters"
  hash="$(printf '%s\n' "$pw" | apikey hash-ops-password)"
  unset pw pw2
  env_set APP_OPS_USER "$user"
  env_set APP_OPS_PASSWORD_HASH "$hash"
  fix_perms
  ok "Swagger enabled for '$user'. Restart the service to apply: $0 restart"
}

swagger_disable() {
  need_root "swagger"
  env_set APP_OPS_USER ""
  env_set APP_OPS_PASSWORD_HASH ""
  fix_perms
  ok "Swagger disabled. Restart the service to apply: $0 restart"
}

setup_swagger() {
  title "Swagger UI"
  swagger_status
  if confirm "Enable Swagger UI and /metrics? (needs APP_OPS_USER and a password)" n; then
    swagger_enable
  else
    swagger_disable
  fi
}

# ================================================================ 7. active sites
active_list() {
  [ -f "$ACTIVE_FILE" ] || return 0
  sed -n '/"active"[[:space:]]*:/,/\]/p' "$ACTIVE_FILE" | sed 's/"active"[[:space:]]*://' |
    grep -o '"[^"]*"' | tr -d '"' || true
}

available_sites() {
  local f
  for f in "$CONFIG_DIR"/sites/*.json; do [ -e "$f" ] && basename "$f" .json; done
}

# site_state DOMAIN -> "enabled", "disabled" or "no config"
site_state() {
  local f
  f="$(grep -lE "\"$(printf '%s' "$1" | sed 's/\./\\./g')\"" "$CONFIG_DIR"/sites/*.json 2>/dev/null | head -n1 || true)"
  if [ -z "$f" ]; then echo "no config"
  elif grep -qE '"enabled"[[:space:]]*:[[:space:]]*false' "$f"; then echo "disabled"
  else echo "enabled"
  fi
}

print_sites() {
  local d st
  printf '  %-30s %s\n' "WEBSITE" "CONFIG"
  while read -r d; do
    [ -n "$d" ] || continue
    st="$(site_state "$d")"
    case "$st" in
      enabled) printf '  %-30s %s%s%s\n' "$d" "$C_GRN" "$st" "$C_OFF" ;;
      *)       printf '  %-30s %s%s (left out of /api/v1/sites)%s\n' "$d" "$C_YLW" "$st" "$C_OFF" ;;
    esac
  done
}

write_active() {
  local comment tmp first=1 d
  comment="$(grep -o '"\$comment"[[:space:]]*:[[:space:]]*"[^"]*"' "$ACTIVE_FILE" 2>/dev/null | head -n1 || true)"
  tmp="$(mktemp)"
  {
    echo "{"
    [ -n "$comment" ] && echo "  $comment,"
    printf '  "active": ['
    for d in "$@"; do
      [ $first -eq 1 ] && printf '\n' || printf ',\n'
      printf '    "%s"' "$d"; first=0
    done
    [ $first -eq 1 ] && printf ']\n' || printf '\n  ]\n'
    echo "}"
  } >"$tmp"
  cat "$tmp" >"$ACTIVE_FILE"; rm -f "$tmp"
}

setup_sites() {
  title "Active websites ($ACTIVE_FILE)"
  local current=() avail=() picked=() d
  mapfile -t current < <(active_list)
  mapfile -t avail < <(available_sites)
  if [ "${#current[@]}" -eq 0 ]; then
    warn "no active list found: every enabled site config would be served"
  else
    printf '%s\n' "${current[@]}" | print_sites
  fi

  if [ "${#current[@]}" -gt 0 ] && confirm "Is this list of websites correct?" y; then
    ok "active websites confirmed (${#current[@]})"
    return
  fi

  title "Site configs available in $CONFIG_DIR/sites"
  local i=1
  for d in "${avail[@]}"; do printf '  %2d) %-30s %s\n' "$i" "$d" "$(site_state "$d")"; i=$((i + 1)); done
  echo "Enter numbers or domains separated by spaces (e.g. '1 3 example.com'), or 'all'."
  local input; input="$(ask "Active websites" "all")"
  if [ "$input" = "all" ]; then
    picked=("${avail[@]}")
  else
    for d in $input; do
      d="${d//,/}"
      if [[ "$d" =~ ^[0-9]+$ ]]; then
        [ "$d" -ge 1 ] && [ "$d" -le "${#avail[@]}" ] || die "no site number $d"
        picked+=("${avail[$((d - 1))]}")
      else
        d="$(printf '%s' "$d" | tr 'A-Z' 'a-z' | sed -e 's#^https\?://##' -e 's#/.*##' -e 's#^www\.##')"
        [ "$(site_state "$d")" = "no config" ] && warn "$d has no config in $CONFIG_DIR/sites; it will be left out until one is added"
        picked+=("$d")
      fi
    done
  fi
  mapfile -t picked < <(printf '%s\n' "${picked[@]}" | awk 'NF && !seen[$0]++')
  [ "${#picked[@]}" -gt 0 ] || die "no websites selected"

  echo; printf '%s\n' "${picked[@]}" | print_sites
  confirm "Save this list to $ACTIVE_FILE?" y || { warn "not saved"; return; }
  write_active "${picked[@]}"
  ok "saved ${#picked[@]} active websites (the server reloads it within 30s)"
}

validate_sites() {
  [ -x "$BIN_DIR/sitectl" ] || return 0
  info "validating site configs"
  if "$BIN_DIR/sitectl" validate --dir "$CONFIG_DIR"; then ok "site configs valid"; else warn "sitectl validate reported problems (see above)"; fi
}

# ================================================================ 8. service
install_service() {
  need_root "service"
  command -v systemctl >/dev/null 2>&1 || { warn "systemd not found; start by hand: $0 run"; return; }
  if ! id "$SERVICE_USER" >/dev/null 2>&1; then
    useradd --system --no-create-home --shell /usr/sbin/nologin "$SERVICE_USER" 2>/dev/null ||
      useradd -r -M -s /sbin/nologin "$SERVICE_USER"
    ok "created system user $SERVICE_USER"
  fi
  fix_perms
  if ! su -s /bin/sh "$SERVICE_USER" -c "test -x '$BIN_DIR/server' && test -r '$ACTIVE_FILE' && test -r '$ENV_FILE'" 2>/dev/null; then
    warn "user $SERVICE_USER cannot read $APP_DIR. Put the project in a readable place such as /opt/$SERVICE_NAME"
    warn "(not under /root or a private home folder), or fix the folder permissions."
  fi
  local stop_timeout xvfb_unit="" xvfb_env=""
  stop_timeout="$(env_get APP_MAX_REQUEST_DURATION)"
  if browser_headful; then
    install_xvfb
    install_xvfb_service
    # Both units share one private /tmp, so the server sees Xvfb's socket.
    xvfb_unit="Requires=$XVFB_SERVICE.service
After=$XVFB_SERVICE.service
JoinsNamespaceOf=$XVFB_SERVICE.service"
    xvfb_env="Environment=DISPLAY=$XVFB_DISPLAY"
  else
    remove_xvfb_service
  fi
  cat >"$SERVICE_FILE" <<EOF
[Unit]
Description=go-wordpress-extractor API
After=network-online.target
Wants=network-online.target
$xvfb_unit

[Service]
Type=simple
User=$SERVICE_USER
Group=$SERVICE_USER
WorkingDirectory=$APP_DIR
EnvironmentFile=$ENV_FILE
# A writable home for Chrome (the service user has none).
StateDirectory=$SERVICE_NAME
Environment=HOME=/var/lib/$SERVICE_NAME
$xvfb_env
ExecStart=$BIN_DIR/server
Restart=on-failure
RestartSec=5
# Running requests may take up to APP_MAX_REQUEST_DURATION (${stop_timeout:-3m}) to finish.
TimeoutStopSec=11min
KillSignal=SIGTERM
NoNewPrivileges=true
ProtectSystem=full
ProtectHome=read-only
PrivateTmp=true
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable "$SERVICE_NAME" >/dev/null
  ok "systemd service $SERVICE_NAME installed"
}

# The virtual display Chrome draws its window on. A separate unit (not
# xvfb-run around the server) so SIGTERM still reaches the server and it
# drains running requests on stop.
install_xvfb_service() {
  local xvfb; xvfb="$(command -v Xvfb)"
  cat >"$XVFB_SERVICE_FILE" <<EOF
[Unit]
Description=Virtual display $XVFB_DISPLAY for go-wordpress-extractor's browser engine
After=network.target
PartOf=$SERVICE_NAME.service

[Service]
Type=simple
User=$SERVICE_USER
Group=$SERVICE_USER
ExecStart=$xvfb $XVFB_DISPLAY -screen 0 1920x1080x24 -nolisten tcp
Restart=on-failure
RestartSec=2
PrivateTmp=true
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable "$XVFB_SERVICE" >/dev/null
  ok "systemd service $XVFB_SERVICE installed (display $XVFB_DISPLAY)"
}

remove_xvfb_service() {
  [ -f "$XVFB_SERVICE_FILE" ] || return 0
  systemctl disable --now "$XVFB_SERVICE" >/dev/null 2>&1 || true
  rm -f "$XVFB_SERVICE_FILE"
  systemctl daemon-reload
  ok "removed $XVFB_SERVICE (browser runs headless)"
}

restart() {
  need_root "restart"
  ai_migrate_default_rule
  ai_preflight
  command -v systemctl >/dev/null 2>&1 && [ -f "$SERVICE_FILE" ] || die "service not installed. Run: $0 service"
  browser_headful && [ -f "$XVFB_SERVICE_FILE" ] && systemctl restart "$XVFB_SERVICE"
  systemctl restart "$SERVICE_NAME"
  local port host url i
  port="$(env_get APP_HTTP_PORT)"; port="${port:-4000}"
  host="$(env_get APP_HTTP_ADDRESS)"
  case "$host" in ""|0.0.0.0|::) host=127.0.0.1 ;; *:*) host="[$host]" ;; esac
  [ -n "$(env_get APP_TLS_CERT_FILE)" ] && url="https://$host:$port/ready" || url="http://$host:$port/ready"
  info "waiting for $url"
  for i in $(seq 1 30); do
    if curl -fsk -o /dev/null "$url"; then ok "service is up"; swagger_status; return; fi
    sleep 1
  done
  journalctl -u "$SERVICE_NAME" -n 40 --no-pager || true
  die "service did not become ready; log above"
}

# Chrome in a window always goes on a hidden Xvfb display, even on a
# desktop (DISPLAY set), so no browser window opens on screen.
XVFB_RUN_ARGS=(--auto-servernum --server-args="-screen 0 1920x1080x24")

run_foreground() {
  ai_migrate_default_rule
  ai_preflight
  load_key_env
  set -a; . "$ENV_FILE"; set +a
  if browser_headful; then
    command -v xvfb-run >/dev/null 2>&1 || die "APP_BROWSER_HEADLESS=false needs Xvfb. Install it: sudo $0 xvfb"
    cd "$APP_DIR" && exec xvfb-run "${XVFB_RUN_ARGS[@]}" "$BIN_DIR/server"
  fi
  cd "$APP_DIR" && exec "$BIN_DIR/server"
}

# dev -> run the development server (go run ./cmd/server, .env.development)
# inside a hidden Xvfb display, so the browser engine never opens a window.
# Run it as your own user, not with sudo. Xvfb is installed with sudo when
# it is missing.
run_dev() {
  [ "$(id -u)" -ne 0 ] || warn "running the development server as root; usually run '$0 dev' as your own user"
  if ! command -v xvfb-run >/dev/null 2>&1; then
    info "xvfb-run is missing; installing Xvfb (sudo apt-get install -y xvfb)"
    if [ "$(id -u)" -eq 0 ]; then install_xvfb; else sudo "$APP_DIR/deploy.sh" xvfb; fi
  fi
  local go_cmd
  go_cmd="$(command -v go || true)"
  if [ -z "$go_cmd" ] && [ -x "$GO_BIN" ]; then go_cmd="$GO_BIN"; fi
  [ -n "$go_cmd" ] || die "go is not installed. Run: sudo $0 go"
  info "starting the development server on a hidden display: xvfb-run -a go run ./cmd/server"
  cd "$APP_DIR" && exec xvfb-run "${XVFB_RUN_ARGS[@]}" "$go_cmd" run ./cmd/server
}

# ================================================================ install
install_all() {
  need_root "install"
  title "1/10 Go";            install_go
  title "2/10 Build";         build
  title "3/10 Env file";      setup_env
  title "4/10 Browser";       setup_browser
  title "5/10 Search";        setup_search
  title "6/10 AI crawler";    setup_ai
  title "7/10 Pepper";        setup_pepper
  title "8/10 API keys"
  load_key_env
  if [ -s "$APP_API_KEYS_FILE" ]; then
    apikey list || true
    confirm "Create another API key?" n && key_generate
  else
    echo "The server needs at least one API key to start."
    key_generate
  fi
  confirm "Open the key menu for more key tasks?" n && keys_menu
  title "9/10 Websites";      setup_sites; validate_sites
                             setup_swagger
  title "10/10 Service";       install_service; restart

  title "Done"
  echo "  Env file : $ENV_FILE"
  echo "  Key file : $(env_get APP_API_KEYS_FILE)"
  echo "  Logs     : journalctl -u $SERVICE_NAME -f"
  echo "  Keys     : sudo $0 keys"
  swagger_status | sed 's/^/  /'
  browser_status | sed 's/^/  /'
  search_status | sed 's/^/  /'
  ai_status | sed 's/^/  /'
}

usage() {
  cat <<EOF
Usage: sudo $0 <command>

Setup
  install              Full setup: Go, build, env, browser, search, AI crawler, pepper, key, websites, Swagger, service
  go                   Install or update Go to the latest version (GO_VERSION=go1.x.y to pin)
  build                Build bin/server, bin/apikey, bin/sitectl
  env                  Create or edit $ENV_FILE
  service              Install the systemd service
  update               Install latest Go, rebuild, update Playwright (if enabled), validate configs and restart

Browser fetch engine (off by default; for sites with fetch.engine "browser")
  browser              Show status
  browser enable       Turn on: installs Node.js, the Playwright driver and Chromium into $PW_DIR,
                       and Xvfb + the $XVFB_SERVICE unit when Chrome runs in a window
  browser install      Install or update Playwright and the browser without changing settings
  browser disable      Turn off (keeps the files)

Search engines (off by default; for sites with fetch.engine "brave", "duckduckgo", "mojeek", "yahoo" or "yandex")
  search               Show status (never shows the API key)
  search enable brave  Turn on Brave; asks for your Brave Search API key without echoing it
                       and writes it to $ENV_FILE
  search enable duckduckgo
                       Turn on DuckDuckGo (no key); shows DuckDuckGo's terms first
  search enable mojeek|yahoo|yandex
                       Turn on that engine (no key; reads its results page); shows its terms first
  search disable <engine>
                       Turn off brave, duckduckgo, mojeek, yahoo or yandex

AI crawler (off by default; engines "ai-crawler-avalai" and "ai-crawler-cloudflare")
  ai                   Show both engines, the default engine and learned configs (never shows secrets)
  ai enable avalai     Turn on AvalAI; asks for base URL, model and the API key (input hidden) and
                       writes the key to $SECRETS_DIR/avalai_api_key
  ai enable cloudflare Turn on Cloudflare AI Gateway; asks for account id, gateway id, endpoint
                       and model, and the token (input hidden) into $SECRETS_DIR/cloudflare_api_token
  ai disable           Turn every AI engine off (unknown hosts get SITE_NOT_CONFIGURED again)
  ai disable avalai|cloudflare
                       Turn one engine off (clears its key or account id); the default engine
                       moves to the other one, or to none
  default-engine [ai-crawler-avalai|ai-crawler-cloudflare|none]
                       Engine for hosts without a rule (APP_DEFAULT_ENGINE); without an
                       argument it lists the engines in recommended order and asks
  learned [list]       List configs the AI crawler learned ($CONFIG_DIR/learned)
  learned show <domain>
  learned activate|reject|adopt <domain>
                       Serve it now / never relearn the host / move it to configs/sites/

API keys
  keys                 Interactive key menu
  key new [name] [ttl] Create one key (ttl like 8760h or never)
  key bulk             Create many keys into a CSV file
  key list             List keys
  key revoke [id]      Revoke a key (or prefix:<name-prefix>)
  pepper               Create a new pepper (invalidates every key)

Websites
  sites                Show and confirm the list in configs/active_sites.json
  validate             Check every site config

Swagger UI (off by default)
  swagger              Show status
  swagger enable       Turn on; asks for APP_OPS_USER and a password
  swagger disable      Turn off

Run
  restart              Restart the service and wait until it is ready
  status               systemctl status
  logs                 Follow the service log
  run                  Run the server in the foreground (no systemd); Chrome in a window goes on a hidden Xvfb display
  dev                  Development: xvfb-run -a go run ./cmd/server as your user (no sudo); installs Xvfb if missing
  xvfb                 Install Xvfb (sudo apt-get install -y xvfb), the hidden display for Chrome
EOF
}

main() {
  local cmd="${1:-help}"; shift || true
  case "$cmd" in
    install)  install_all ;;
    go)       install_go ;;
    build)    need_root build; build ;;
    env)      setup_env ;;
    pepper)   setup_pepper ;;
    service)  install_service ;;
    update)   need_root update; install_go; build; install_playwright; validate_sites; restart ;;
    browser|playwright)
      case "${1:-status}" in
        enable|on)   browser_enable ;;
        disable|off) browser_disable ;;
        install)     install_playwright force ;;
        *)           browser_status ;;
      esac ;;
    search)
      case "${1:-status}" in
        enable|on)   shift; search_enable "${1:-}" ;;
        disable|off) shift; search_disable "${1:-}" ;;
        *)           search_status ;;
      esac ;;
    ai)
      case "${1:-status}" in
        enable|on)   shift; ai_enable "${1:-}" ;;
        disable|off) shift; ai_disable "${1:-}" ;;
        *)           ai_status ;;
      esac ;;
    default-engine) ai_set_default_engine "${1:-}" ;;
    learned)  ai_learned "$@" ;;
    keys)     keys_menu ;;
    key)
      need_root key
      case "${1:-}" in
        new|generate) shift; key_generate "$@" ;;
        bulk)         key_bulk ;;
        list|ls)      shift; key_list "$@" ;;
        revoke|rm)    shift; key_revoke "$@" ;;
        *)            keys_menu ;;
      esac ;;
    sites)    need_root sites; setup_sites; validate_sites ;;
    validate) validate_sites ;;
    swagger)
      case "${1:-status}" in
        enable|on)   swagger_enable ;;
        disable|off) swagger_disable ;;
        *)           swagger_status ;;
      esac ;;
    restart)  restart ;;
    status)   systemctl status "$SERVICE_NAME" --no-pager; browser_headful && systemctl status "$XVFB_SERVICE" --no-pager || true ;;
    logs)     journalctl -u "$SERVICE_NAME" -f ;;
    run)      run_foreground ;;
    dev)      run_dev ;;
    xvfb)     need_root xvfb; install_xvfb ;;
    help|-h|--help) usage ;;
    *) usage; exit 2 ;;
  esac
}

main "$@"
