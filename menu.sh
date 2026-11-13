#!/usr/bin/env bash
# Menu for the API key pepper, operator login, client API keys, and the
# server. Installs nothing; uses only bash and `go run`.
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

run_server() {
  need_pepper || return 1
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

=== API keys ($ENV_FILE) ===
  1) Run server
  2) Delete all keys
  3) Show keys
  4) Set up all keys (pepper, operator login, client key)
  5) Create a client key
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
    0|q|Q) exit 0 ;;
    *) echo "Unknown choice: $choice" ;;
  esac
done
