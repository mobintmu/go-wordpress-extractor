#!/usr/bin/env bash
# Menu for the API key pepper, operator login, client API keys, the AI
# crawler (AvalAI, Cloudflare AI Gateway), and the server. Installs nothing; uses only bash and
# `go run`.
#
#   ./setup-keys.sh                    # uses .env
#   ./setup-keys.sh .env.development   # or another env file
set -uo pipefail

cd "$(dirname "$0")"
ENV_FILE="${1:-.env}"
MIN_PEPPER=32
MIN_PASSWORD=12

[[ -f "$ENV_FILE" ]] || { echo "error: $ENV_FILE not found" >&2; exit 1; }

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

# get_var NAME -> value of NAME in the env file (empty if missing)
get_var() {
  local line
  line=$(grep -E "^$1=" "$ENV_FILE" | tail -n1 || true)
  printf '%s' "${line#*=}"
}

# set_var NAME VALUE -> replace NAME=... in the env file, or append it.
# awk reads the value from the environment, so no character needs escaping.
set_var() {
  local tmp
  tmp=$(mktemp)
  K="$1" V="$2" awk '
    BEGIN { k = ENVIRON["K"]; v = ENVIRON["V"]; done = 0 }
    index($0, k "=") == 1 { print k "=" v; done = 1; next }
    { print }
    END { if (!done) print k "=" v }
  ' "$ENV_FILE" >"$tmp" && cat "$tmp" >"$ENV_FILE"   # cat keeps the file's permissions
  rm -f "$tmp"
}

ask_yes() { # ask_yes "question" default(y|n)
  local reply hint="[y/N]"
  [[ "$2" == y ]] && hint="[Y/n]"
  read -r -p "$1 $hint " reply
  reply="${reply:-$2}"
  [[ "$reply" =~ ^[Yy] ]]
}

apikey() { go run ./cmd/apikey "$@"; }

# Export every variable in the env file to this shell and its children.
load_env() {
  set -a
  # shellcheck disable=SC1090
  source "$ENV_FILE"
  set +a
}

backup_env() {
  local backup
  backup="$ENV_FILE.bak.$(date +%Y%m%d-%H%M%S)"
  cp -p "$ENV_FILE" "$backup"
  echo "Backed up $ENV_FILE to $backup"
}

keys_file() { printf '%s' "${APP_API_KEYS_FILE:-}"; }

need_pepper() {
  if (( ${#APP_API_KEY_PEPPER} < MIN_PEPPER )); then
    echo "APP_API_KEY_PEPPER is missing or too short. Run 'Set up all keys' first."
    return 1
  fi
}

# ---------------------------------------------------------------------------
# Actions
# ---------------------------------------------------------------------------

setup_pepper() {
  local pepper
  pepper=$(get_var APP_API_KEY_PEPPER)
  PEPPER_CHANGED=0
  if (( ${#pepper} >= MIN_PEPPER )); then
    echo "APP_API_KEY_PEPPER is already set."
    echo "Changing it makes every existing API key and the operator password hash invalid."
    ask_yes "Generate a new pepper anyway?" n || return 0
  else
    echo "APP_API_KEY_PEPPER is missing or shorter than $MIN_PEPPER characters; generating one."
  fi
  pepper=$(apikey generate-pepper) || { echo "error: could not generate a pepper"; return 1; }
  set_var APP_API_KEY_PEPPER "$pepper"
  PEPPER_CHANGED=1
  echo "New pepper written. Keys made with the old pepper no longer work."
}

setup_ops_password() {
  local ops_user ops_hash reply pw1 pw2
  ops_user=$(get_var APP_OPS_USER)
  ops_hash=$(get_var APP_OPS_PASSWORD_HASH)
  if (( PEPPER_CHANGED )) || [[ -z "$ops_hash" ]]; then
    echo "The operator password must be hashed with the current pepper."
  elif ! ask_yes "Set a new operator password?" n; then
    return 0
  fi

  read -r -p "Operator user [${ops_user:-admin}]: " reply
  ops_user="${reply:-${ops_user:-admin}}"
  while true; do
    read -r -s -p "Operator password (at least $MIN_PASSWORD characters): " pw1; echo
    read -r -s -p "Repeat password: " pw2; echo
    if [[ "$pw1" != "$pw2" ]]; then
      echo "Passwords do not match."
    elif (( ${#pw1} < MIN_PASSWORD )); then
      echo "Password is too short."
    else
      break
    fi
  done
  ops_hash=$(printf '%s\n' "$pw1" | APP_API_KEY_PEPPER="$(get_var APP_API_KEY_PEPPER)" apikey hash-ops-password)
  local rc=$?
  unset pw1 pw2
  (( rc == 0 )) || { echo "error: could not hash the password"; return 1; }
  set_var APP_OPS_USER "$ops_user"
  set_var APP_OPS_PASSWORD_HASH "$ops_hash"
  echo "Operator login saved for user '$ops_user'."
}

create_key() {
  need_pepper || return 1
  local name days expires
  read -r -p "Client name (letters, digits, '-', '_') [my-client]: " name
  while true; do
    read -r -p "Valid for how many days? (number, or 'never') [365]: " days
    days="${days:-365}"
    if [[ "$days" == never ]]; then
      expires=never; break
    elif [[ "$days" =~ ^[0-9]+$ ]] && (( days > 0 )); then
      expires="$(( days * 24 ))h"; break
    fi
    echo "Enter a whole number of days greater than 0, or 'never'."
  done
  if apikey generate --name "${name:-my-client}" --expires-in "$expires"; then
    echo "Copy the key now; it is not shown again."
    echo "Restart a running server so it picks up the new key."
  fi
}

setup_all() {
  backup_env
  echo
  setup_pepper || return 1
  echo
  setup_ops_password || return 1
  load_env
  echo
  if ask_yes "Create a client API key now?" n; then
    create_key
  fi
}

show_keys() {
  local f
  f=$(keys_file)
  if [[ -z "$f" || ! -f "$f" ]]; then
    echo "No key file yet (${f:-APP_API_KEYS_FILE is not set})."
    return 0
  fi
  echo "Keys in $f:"
  apikey list
}

delete_keys() {
  local f confirm backup
  f=$(keys_file)
  if [[ -z "$f" || ! -f "$f" ]]; then
    echo "No key file to delete from (${f:-APP_API_KEYS_FILE is not set})."
    return 0
  fi
  show_keys
  echo
  ask_yes "Remove ALL client API keys?" n || return 0
  read -r -p "Type 'remove all' to confirm: " confirm
  if [[ "$confirm" != "remove all" ]]; then
    echo "Not confirmed; keys left unchanged."
    return 0
  fi
  backup="$f.bak.$(date +%Y%m%d-%H%M%S)"
  cp -p "$f" "$backup"
  printf '{\n  "version": 1,\n  "keys": []\n}\n' >"$f"
  chmod 600 "$f"
  echo "All keys removed. Old file saved as $backup (delete it if you don't need it)."
  echo "Restart any running server, and copy this file to every instance."
}

# Ask for a proxy host and port, export http_proxy/https_proxy for this
# menu session (go run and other tools started from here use them), and
# optionally save APP_PROXY_HOST/APP_PROXY_PORT, which the server's own
# fetches use. The server ignores http_proxy/https_proxy on purpose.
set_proxy() {
  local cur_host cur_port host port
  cur_host=$(get_var APP_PROXY_HOST)
  cur_port=$(get_var APP_PROXY_PORT)
  echo "Current: http_proxy=${http_proxy:-<unset>}  APP_PROXY_HOST=${cur_host:-<unset>}  APP_PROXY_PORT=${cur_port:-<unset>}"
  echo "Enter 'none' as the host to remove the proxy."
  while true; do
    read -r -p "Proxy host (example: 127.0.0.1) [${cur_host:-127.0.0.1}]: " host
    host="${host:-${cur_host:-127.0.0.1}}"
    if [[ "$host" == none ]]; then
      unset http_proxy https_proxy HTTP_PROXY HTTPS_PROXY
      if ask_yes "Also remove APP_PROXY_HOST/APP_PROXY_PORT from $ENV_FILE (server fetches directly)?" y; then
        set_var APP_PROXY_HOST ""
        set_var APP_PROXY_PORT ""
        unset APP_PROXY_HOST APP_PROXY_PORT
        echo "Proxy removed from $ENV_FILE."
      fi
      echo "http_proxy and https_proxy unset for this session."
      return 0
    fi
    # A host name or IP address only: no scheme, port, path, or spaces.
    if [[ "$host" =~ ^[A-Za-z0-9.-]+$ ]]; then
      break
    fi
    echo "Enter a host name or IP address only, for example 127.0.0.1 (no http://, no port)."
  done
  while true; do
    read -r -p "Proxy port (example: 3067) [${cur_port:-3067}]: " port
    port="${port:-${cur_port:-3067}}"
    if [[ "$port" =~ ^[0-9]+$ ]] && (( port >= 1 && port <= 65535 )); then
      break
    fi
    echo "Enter a port between 1 and 65535, for example 3067."
  done

  export https_proxy="http://$host:$port"
  export http_proxy="http://$host:$port"
  echo "Exported for this session:"
  echo "  export https_proxy=$https_proxy"
  echo "  export http_proxy=$http_proxy"

  if ask_yes "Save as APP_PROXY_HOST/APP_PROXY_PORT in $ENV_FILE so the server's fetches use it?" y; then
    set_var APP_PROXY_HOST "$host"
    set_var APP_PROXY_PORT "$port"
    export APP_PROXY_HOST="$host" APP_PROXY_PORT="$port"
    echo "Saved. Restart a running server to use the proxy."
  fi
}

AI_KEY_FILE="secrets/avalai_api_key"
CF_TOKEN_FILE="secrets/cloudflare_api_token"
CF_DEFAULT_MODEL="@cf/deepseek-ai/deepseek-v4.1-flash-1017"

# secret_source VAR FILE_VAR -> where a secret comes from, never the secret.
secret_source() {
  if [[ -n "$(get_var "$1")" ]]; then echo "set in $ENV_FILE (hidden)"
  elif [[ -s "$(get_var "$2")" ]]; then echo "from file $(get_var "$2")"
  else echo "not set"; fi
}

# default_engine -> APP_DEFAULT_ENGINE as the server resolves it.
default_engine() {
  local v; v="$(get_var APP_DEFAULT_ENGINE)"
  if [[ -n "$v" ]]; then echo "$v"
  elif [[ "$(get_var APP_AI_CRAWLER_ENABLED)" == true ]]; then echo ai-crawler-avalai
  else echo none; fi
}

# APP_AI_CRAWLER_DEFAULT_RULE was replaced by APP_DEFAULT_ENGINE; the server
# refuses to start while it is set, so it is rewritten here.
migrate_default_rule() {
  grep -qE '^APP_AI_CRAWLER_DEFAULT_RULE=' "$ENV_FILE" || return 0
  local old engine tmp
  old="$(get_var APP_AI_CRAWLER_DEFAULT_RULE)"
  engine=none
  [[ "$old" != false && "$(get_var APP_AI_CRAWLER_ENABLED)" == true ]] && engine="$(v=$(get_var APP_DEFAULT_ENGINE); echo "${v:-ai-crawler-avalai}")"
  tmp=$(mktemp)
  grep -vE '^APP_AI_CRAWLER_DEFAULT_RULE=' "$ENV_FILE" >"$tmp"; cat "$tmp" >"$ENV_FILE"; rm -f "$tmp"
  unset APP_AI_CRAWLER_DEFAULT_RULE
  set_var APP_DEFAULT_ENGINE "$engine"
  echo "Replaced APP_AI_CRAWLER_DEFAULT_RULE=$old with APP_DEFAULT_ENGINE=$engine in $ENV_FILE."
}

ai_status() {
  echo "AI crawler: $([[ "$(get_var APP_AI_CRAWLER_ENABLED)" == true ]] && echo ENABLED || echo off)   learning: $(v=$(get_var APP_AI_CRAWLER_LEARN_SELECTORS); echo "${v:-true}")"
  echo "  ai-crawler-avalai"
  echo "    base URL         : $(v=$(get_var APP_AI_CRAWLER_BASE_URL); echo "${v:-https://api.avalai.ir/v1}")"
  echo "    model            : $(v=$(get_var APP_AI_CRAWLER_MODEL); echo "${v:-deepseek-v4.1-flash}")"
  echo "    API key          : $(secret_source APP_AI_CRAWLER_API_KEY APP_AI_CRAWLER_API_KEY_FILE)"
  echo "  ai-crawler-cloudflare"
  echo "    account / gateway: $(v=$(get_var APP_AI_CRAWLER_CLOUDFLARE_ACCOUNT_ID); echo "${v:-not set}") / $(v=$(get_var APP_AI_CRAWLER_CLOUDFLARE_GATEWAY_ID); echo "${v:-default}")"
  echo "    endpoint         : $(v=$(get_var APP_AI_CRAWLER_CLOUDFLARE_ENDPOINT); echo "${v:-rest}")"
  echo "    model            : $(v=$(get_var APP_AI_CRAWLER_CLOUDFLARE_MODEL); echo "${v:-$CF_DEFAULT_MODEL}")"
  echo "    API token        : $(secret_source APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN_FILE)"
  echo "  default engine     : $(default_engine)  (APP_DEFAULT_ENGINE: hosts without a rule)"
}

# ai_turn_on turns the AI crawler on and asks whether to learn selectors.
ai_turn_on() {
  set_var APP_AI_CRAWLER_ENABLED true
  if ask_yes "Learn CSS selectors from AI results (proven hosts need no LLM calls)?" y; then
    set_var APP_AI_CRAWLER_LEARN_SELECTORS true
  else
    set_var APP_AI_CRAWLER_LEARN_SELECTORS false
  fi
}

# Set up AvalAI. The key is typed hidden and written to
# secrets/avalai_api_key (git-ignored), never into the env file, because
# .env.development and .env.test are committed.
setup_avalai() {
  echo "ai-crawler-avalai sends the content of each page it serves to AvalAI."
  ask_yes "Set up and turn on ai-crawler-avalai?" y || return 0

  local base model key
  read -r -p "OpenAI-compatible base URL [$(v=$(get_var APP_AI_CRAWLER_BASE_URL); echo "${v:-https://api.avalai.ir/v1}")]: " base
  base="${base:-$(v=$(get_var APP_AI_CRAWLER_BASE_URL); echo "${v:-https://api.avalai.ir/v1}")}"
  if [[ "$base" != https://* ]]; then
    echo "The base URL must start with https://."
    return 1
  fi
  read -r -p "Model [$(v=$(get_var APP_AI_CRAWLER_MODEL); echo "${v:-deepseek-v4.1-flash}")]: " model
  model="${model:-$(v=$(get_var APP_AI_CRAWLER_MODEL); echo "${v:-deepseek-v4.1-flash}")}"

  if [[ -s "$AI_KEY_FILE" ]] && ask_yes "An API key is already in $AI_KEY_FILE. Keep it?" y; then
    :
  else
    read -r -s -p "AvalAI API key (input hidden): " key; echo
    key="$(printf '%s' "$key" | tr -d '[:space:]')"
    if [[ -z "$key" ]]; then
      echo "No key entered; nothing changed."
      return 1
    fi
    mkdir -p secrets
    ( umask 077; printf '%s\n' "$key" >"$AI_KEY_FILE" )
    unset key
    echo "Key saved to $AI_KEY_FILE (git-ignored, mode 600)."
  fi

  set_var APP_AI_CRAWLER_BASE_URL "$base"
  set_var APP_AI_CRAWLER_MODEL "$model"
  set_var APP_AI_CRAWLER_API_KEY_FILE "$AI_KEY_FILE"
  ai_turn_on
  if ask_yes "Serve every host without a rule with ai-crawler-avalai (APP_DEFAULT_ENGINE)?" y; then
    set_var APP_DEFAULT_ENGINE ai-crawler-avalai
  fi
  [[ -n "$(get_var APP_DEFAULT_ENGINE)" ]] || set_var APP_DEFAULT_ENGINE none
}

# Set up Cloudflare AI Gateway. The token goes alone into
# secrets/cloudflare_api_token (git-ignored).
setup_cloudflare() {
  echo "ai-crawler-cloudflare sends the content of each page it serves to Cloudflare AI Gateway and"
  echo "the model behind it. The default model, DeepSeek V4.1 Flash, runs on Cloudflare (Workers AI)"
  echo "and is billed to the Cloudflare account. The token needs \"Workers AI Read\" and \"AI Gateway Run\"."
  ask_yes "Set up and turn on ai-crawler-cloudflare?" y || return 0

  local account gateway endpoint model model_def token
  read -r -p "Account id (32 hex characters) [$(get_var APP_AI_CRAWLER_CLOUDFLARE_ACCOUNT_ID)]: " account
  account="${account:-$(get_var APP_AI_CRAWLER_CLOUDFLARE_ACCOUNT_ID)}"
  account="$(printf '%s' "$account" | tr -d '[:space:]' | tr '[:upper:]' '[:lower:]')"
  if [[ ! "$account" =~ ^[0-9a-f]{32}$ ]]; then
    echo "The account id must be 32 hex characters."
    return 1
  fi
  read -r -p "Gateway id [$(v=$(get_var APP_AI_CRAWLER_CLOUDFLARE_GATEWAY_ID); echo "${v:-default}")]: " gateway
  gateway="${gateway:-$(v=$(get_var APP_AI_CRAWLER_CLOUDFLARE_GATEWAY_ID); echo "${v:-default}")}"
  read -r -p "Endpoint, rest or compat [$(v=$(get_var APP_AI_CRAWLER_CLOUDFLARE_ENDPOINT); echo "${v:-rest}")]: " endpoint
  endpoint="${endpoint:-$(v=$(get_var APP_AI_CRAWLER_CLOUDFLARE_ENDPOINT); echo "${v:-rest}")}"
  if [[ "$endpoint" != rest && "$endpoint" != compat ]]; then
    echo "The endpoint must be rest or compat."
    return 1
  fi
  model_def="$(v=$(get_var APP_AI_CRAWLER_CLOUDFLARE_MODEL); echo "${v:-$CF_DEFAULT_MODEL}")"
  [[ "$endpoint" == compat && "$model_def" == "$CF_DEFAULT_MODEL" ]] && model_def="workers-ai/$CF_DEFAULT_MODEL"
  read -r -p "Model [$model_def]: " model
  model="${model:-$model_def}"

  if [[ -s "$CF_TOKEN_FILE" ]] && ask_yes "A token is already in $CF_TOKEN_FILE. Keep it?" y; then
    :
  else
    read -r -s -p "Cloudflare API token (input hidden): " token; echo
    token="$(printf '%s' "$token" | tr -d '[:space:]')"
    if [[ -z "$token" ]]; then
      echo "No token entered; nothing changed."
      return 1
    fi
    mkdir -p secrets
    ( umask 077; printf '%s\n' "$token" >"$CF_TOKEN_FILE" )
    unset token
    echo "Token saved to $CF_TOKEN_FILE (git-ignored, mode 600)."
  fi

  set_var APP_AI_CRAWLER_CLOUDFLARE_ACCOUNT_ID "$account"
  set_var APP_AI_CRAWLER_CLOUDFLARE_GATEWAY_ID "$gateway"
  set_var APP_AI_CRAWLER_CLOUDFLARE_ENDPOINT "$endpoint"
  set_var APP_AI_CRAWLER_CLOUDFLARE_MODEL "$model"
  set_var APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN_FILE "$CF_TOKEN_FILE"
  ai_turn_on
  if ask_yes "Serve every host without a rule with ai-crawler-cloudflare (APP_DEFAULT_ENGINE)?" n; then
    set_var APP_DEFAULT_ENGINE ai-crawler-cloudflare
  fi
  [[ -n "$(get_var APP_DEFAULT_ENGINE)" ]] || set_var APP_DEFAULT_ENGINE none
}

# avalai_ok / cloudflare_ok: the engine's own settings are complete (the
# server's rule; APP_AI_CRAWLER_ENABLED must also be true for it to run).
avalai_ok() {
  [[ -n "$(get_var APP_AI_CRAWLER_API_KEY)" || -s "$(get_var APP_AI_CRAWLER_API_KEY_FILE)" ]]
}
cloudflare_ok() {
  [[ -n "$(get_var APP_AI_CRAWLER_CLOUDFLARE_ACCOUNT_ID)" ]] &&
    [[ -n "$(get_var APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN)" || -s "$(get_var APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN_FILE)" ]]
}

# recommended_engine: the configured AvalAI engine first (the existing
# default; Cloudflare's live check, proposal step 1, is still open), then
# Cloudflare, then none.
recommended_engine() {
  if avalai_ok; then echo ai-crawler-avalai
  elif cloudflare_ok; then echo ai-crawler-cloudflare
  else echo none; fi
}

# Set up the default engine: the engine for websites with no site config
# and no active learned config (APP_DEFAULT_ENGINE). Lists the engines that
# can serve such websites, marks the recommended and the current one, and
# offers to set up an engine that is not configured yet.
setup_default_engine() {
  migrate_default_rule
  local cur rec av cf choice engine
  cur="$(default_engine)"
  rec="$(recommended_engine)"
  av="not configured"; avalai_ok && av="configured"
  cf="not configured"; cloudflare_ok && cf="configured"
  mark() { # mark engine -> " (recommended, current)" ...
    local m=()
    [[ "$1" == "$rec" ]] && m+=("recommended")
    [[ "$1" == "$cur" ]] && m+=("current")
    (( ${#m[@]} )) && printf ' (%s)' "$(IFS=,; echo "${m[*]}" | sed 's/,/, /g')"
  }
  cat <<MENU
Default engine: serves every website that has no file in configs/sites/ and
no active learned config. Engines, in recommended order:

  1) ai-crawler-avalai$(mark ai-crawler-avalai)
       [$av] LLM through AvalAI ($(v=$(get_var APP_AI_CRAWLER_MODEL); echo "${v:-deepseek-v4.1-flash}")).
       The existing default. Needs an AvalAI API key.
  2) ai-crawler-cloudflare$(mark ai-crawler-cloudflare)
       [$cf] LLM through Cloudflare AI Gateway ($(v=$(get_var APP_AI_CRAWLER_CLOUDFLARE_MODEL); echo "${v:-$CF_DEFAULT_MODEL}")).
       Default model runs on Cloudflare (Workers AI): about \$3.60 per 1,000 pages,
       billed to the Cloudflare account. Needs an account id and an API token.
  3) none$(mark none)
       Unknown websites fail with SITE_NOT_CONFIGURED. No page is sent to an
       LLM and nothing is learned. Configured sites still work.

  Not offered: http, browser and the search engines need a site config's
  selectors, so they cannot serve a website without one.

MENU
  read -r -p "Choose 1-3 [Enter keeps $cur]: " choice
  case "$choice" in
    1|ai-crawler-avalai) engine=ai-crawler-avalai ;;
    2|ai-crawler-cloudflare) engine=ai-crawler-cloudflare ;;
    3|none) engine=none ;;
    "") echo "Kept APP_DEFAULT_ENGINE=$cur."; return 0 ;;
    *) echo "Unknown choice: $choice"; return 1 ;;
  esac

  if [[ "$engine" == ai-crawler-avalai ]] && ! avalai_ok; then
    ask_yes "ai-crawler-avalai is not configured. Set it up now?" y && setup_avalai
    avalai_ok || { echo "ai-crawler-avalai is still not configured; APP_DEFAULT_ENGINE stays $cur."; return 1; }
  fi
  if [[ "$engine" == ai-crawler-cloudflare ]] && ! cloudflare_ok; then
    ask_yes "ai-crawler-cloudflare is not configured. Set it up now?" y && setup_cloudflare
    cloudflare_ok || { echo "ai-crawler-cloudflare is still not configured; APP_DEFAULT_ENGINE stays $cur."; return 1; }
  fi
  if [[ "$engine" != none && "$(get_var APP_AI_CRAWLER_ENABLED)" != true ]]; then
    if ask_yes "The AI crawler is off (APP_AI_CRAWLER_ENABLED). Turn it on?" y; then
      set_var APP_AI_CRAWLER_ENABLED true
    else
      echo "The server does not start with an AI default engine while the AI crawler is off; APP_DEFAULT_ENGINE stays $cur."
      return 1
    fi
  fi
  set_var APP_DEFAULT_ENGINE "$engine"
  load_env
  echo "APP_DEFAULT_ENGINE=$engine. Restart a running server to apply."
}

# ai_off_engine avalai|cloudflare clears one engine's credentials; the
# default engine moves to the other one, or to none.
ai_off_engine() {
  local engine="$1"
  if [[ "$engine" == ai-crawler-avalai ]]; then
    set_var APP_AI_CRAWLER_API_KEY ""
    set_var APP_AI_CRAWLER_API_KEY_FILE ""
  else
    set_var APP_AI_CRAWLER_CLOUDFLARE_ACCOUNT_ID ""
    set_var APP_AI_CRAWLER_CLOUDFLARE_API_TOKEN ""
  fi
  if [[ "$(default_engine)" == "$engine" ]]; then
    set_var APP_DEFAULT_ENGINE "$(recommended_engine)"
    echo "APP_DEFAULT_ENGINE was $engine; it is now $(get_var APP_DEFAULT_ENGINE)."
  fi
  if ! avalai_ok && ! cloudflare_ok; then
    set_var APP_AI_CRAWLER_ENABLED false
    set_var APP_DEFAULT_ENGINE none
    echo "No AI engine is left; the AI crawler is off."
  fi
  echo "$engine off. Secret files in secrets/ are kept."
}

# preflight_default_engine reports what would stop the server at startup.
preflight_default_engine() {
  local d; d="$(default_engine)"
  case "$d" in
    none) return 0 ;;
    ai-crawler-avalai) avalai_ok || { echo "APP_DEFAULT_ENGINE is ai-crawler-avalai, but no AvalAI key is set (menu 9)."; return 1; } ;;
    ai-crawler-cloudflare) cloudflare_ok || { echo "APP_DEFAULT_ENGINE is ai-crawler-cloudflare, but the account id or token is missing (menu 9)."; return 1; } ;;
    *) echo "APP_DEFAULT_ENGINE \"$d\" is not valid (menu 9)."; return 1 ;;
  esac
  [[ "$(get_var APP_AI_CRAWLER_ENABLED)" == true ]] || { echo "APP_DEFAULT_ENGINE is $d, but APP_AI_CRAWLER_ENABLED is not true (menu 9)."; return 1; }
}

# AI crawler submenu: AvalAI, Cloudflare, default engine, off.
setup_ai() {
  migrate_default_rule
  local c
  while true; do
    echo
    ai_status
    echo
    echo "  a) AvalAI (ai-crawler-avalai): set up and turn on"
    echo "  c) Cloudflare AI Gateway (ai-crawler-cloudflare): set up and turn on"
    echo "  d) Set up the default engine (APP_DEFAULT_ENGINE), with the recommended list"
    echo "  x) Turn AvalAI off (clears its key setting)"
    echo "  y) Turn Cloudflare off (clears its account id)"
    echo "  o) Turn the whole AI crawler off"
    echo "  Enter) Back"
    read -r -p "Choose: " c || return 0
    case "$c" in
      a|A) setup_avalai ;;
      c|C) setup_cloudflare ;;
      d|D) setup_default_engine ;;
      x|X) ai_off_engine ai-crawler-avalai ;;
      y|Y) ai_off_engine ai-crawler-cloudflare ;;
      o|O)
        set_var APP_AI_CRAWLER_ENABLED false
        set_var APP_DEFAULT_ENGINE none
        echo "AI crawler off; unknown hosts get SITE_NOT_CONFIGURED." ;;
      "") load_env; echo "Restart a running server to apply."; return 0 ;;
      *) echo "Unknown choice: $c" ;;
    esac
    load_env
  done
}

# Review the configs the AI crawler learned (configs/learned/).
learned_menu() {
  local action domain
  go run ./cmd/sitectl learned list || return 1
  echo
  read -r -p "Action: show, activate, reject, adopt, or Enter to go back: " action
  [[ -z "$action" ]] && return 0
  case "$action" in
    show|activate|reject|adopt) ;;
    *) echo "Unknown action: $action"; return 1 ;;
  esac
  read -r -p "Domain (example: wayfair.com): " domain
  [[ -n "$domain" ]] || { echo "No domain given."; return 1; }
  go run ./cmd/sitectl learned "$action" "$domain"
}

run_server() {
  need_pepper || return 1
  migrate_default_rule
  preflight_default_engine || return 1
  load_env
  echo "Starting the server with $ENV_FILE (Ctrl+C to stop)..."
  exec go run ./cmd/server
}

# ---------------------------------------------------------------------------
# Menu
# ---------------------------------------------------------------------------

load_env
PEPPER_CHANGED=0

while true; do
  cat <<EOF

=== API keys and AI crawler ($ENV_FILE) ===
  1) Run server
  2) Delete all keys
  3) Show keys
  4) Set up all keys (pepper, operator login, client key)
  5) Create a client key
  6) Set proxy (host and port)
  7) AI crawler: AvalAI, Cloudflare, default engine
  8) Learned configs (list, show, activate, reject, adopt)
  9) Default engine for websites without a rule (recommended list)
  0) Exit
EOF
  read -r -p "Choose: " choice || exit 0
  echo
  case "$choice" in
    1) run_server ;;
    2) delete_keys ;;
    3) show_keys ;;
    4) setup_all ;;
    5) create_key ;;
    6) set_proxy ;;
    7) setup_ai ;;
    8) learned_menu ;;
    9) setup_default_engine ;;
    0|q|Q) exit 0 ;;
    *) echo "Unknown choice: $choice" ;;
  esac
done
