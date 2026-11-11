# Setting up keys from scratch

This guide creates every secret the service needs, in order. Each step is one or two commands.

## What you need

| # | Secret | Required? | What it is for | Where it goes |
|---|---|---|---|---|
| 1 | **Pepper** | Yes | A server secret. All other hashes are made with it. | `APP_API_KEY_PEPPER` in your env file |
| 2 | **API key** | Yes, at least one | Client applications send it in the `X-API-Key` header. | The key file (`APP_API_KEYS_FILE`) stores only its hash |
| 3 | **Operator password hash** | No | Opens Swagger UI and `/metrics`. Without it, both return 404. | `APP_OPS_USER` and `APP_OPS_PASSWORD_HASH` in your env file |
| 4 | **TLS certificate** | No | Serves HTTPS instead of HTTP. | `APP_TLS_CERT_FILE` and `APP_TLS_KEY_FILE` |

## The one rule

**Make every key and hash with the same pepper the server uses.** A key or password hash made with a different pepper never matches, and you get `401`.

That is why this guide uses **one env file** for everything. Pick one and stay with it:

- `.env.development`: used by `go run ./cmd/server` when you load nothing yourself.
- `.env`: used by Docker Compose, and by `go run` if you run `set -a; . ./.env; set +a` first.

Both the server and the `apikey` command print a **pepper fingerprint**, for example `pepper fingerprint: f883fb7f`. If the two fingerprints match, the pepper matches.

> If you change the pepper later, every existing API key and the operator password stop working. Make them again with the new pepper.

---

## A. Running with `go run`

All commands run from the project root. The examples use `.env.development`; to use `.env`, replace the file name everywhere.

### Step 1: Create the pepper

```bash
sed -i "s|^APP_API_KEY_PEPPER=.*|APP_API_KEY_PEPPER=$(go run ./cmd/apikey generate-pepper)|" .env.development
```

This writes a new random pepper into the file. Skip this step if the file already has the pepper you want to keep.

### Step 2: Load the pepper and key file into this terminal

```bash
export APP_API_KEY_PEPPER=$(grep '^APP_API_KEY_PEPPER=' .env.development | cut -d= -f2-)
export APP_API_KEYS_FILE=secrets/api_keys.json
```

The next steps read these two values. Run this again in every new terminal.

### Step 3: Create an API key

```bash
go run ./cmd/apikey generate --name my-client
```

Output:

```text
pepper fingerprint: f883fb7f (must match the server's startup log)
created key i7ne1jqz (my-client), expires 2027-11-11T00:10:10Z
key (shown once, cannot be recovered): gwp_i7ne1jqz_...
key file: secrets/api_keys.json
```

**Copy the `gwp_...` key now.** It is shown only once. The file stores only its hash.

To create many keys at once and write them to a CSV file instead of the screen:

```bash
go run ./cmd/apikey generate-bulk --count 10 --name-prefix app --plain-output secrets/keys.csv
```

### Step 4 (optional): Create the operator login for Swagger UI

```bash
H=$(echo 'my-operator-password' | go run ./cmd/apikey hash-ops-password)
sed -i -e "s|^APP_OPS_USER=.*|APP_OPS_USER=admin|" \
       -e "s|^APP_OPS_PASSWORD_HASH=.*|APP_OPS_PASSWORD_HASH=$H|" .env.development
```

Use your own password, at least 12 characters. You log in with `admin` and this password.

### Step 5 (optional): Create a TLS certificate for HTTPS

For local testing, a self-signed certificate is enough:

```bash
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -days 365 \
  -subj "/CN=localhost" -addext "subjectAltName=IP:127.0.0.1,DNS:localhost" \
  -keyout secrets/tls.key -out secrets/tls.crt
```

Then add these two lines to your env file:

```dotenv
APP_TLS_CERT_FILE=secrets/tls.crt
APP_TLS_KEY_FILE=secrets/tls.key
```

In production, use a real certificate, for example from Let's Encrypt.

### Step 6: Start and check

```bash
set -a; . ./.env.development; set +a
go run ./cmd/server
```

Check the startup log:

- The `API keys loaded` line shows `keys: 1` (or more) and a `pepper_fingerprint` that **matches** step 3.
- `✅ Swagger UI: ...` appears if you did step 4.

Then call the API with your key:

```bash
curl -X POST http://127.0.0.1:4000/api/v1/extract \
  -H "X-API-Key: gwp_..." -H 'Content-Type: application/json' \
  -d '{"url": "https://hosseinibrothers.ir/کشمش/کشمش-پلویی-آفتابی"}'
```

Open Swagger UI at http://127.0.0.1:4000/swagger/index.html and log in with `admin` and your password.

---

## B. Running with Docker Compose

Docker Compose reads `.env` in the project root. The `apikey` tool runs inside the same image, so you do not need Go installed.

```bash
# 1. Create the secrets folder yourself, so it is owned by you and not by root
mkdir -p secrets

# 2. Create the pepper and put it in .env
#    (create .env first if it does not exist: touch .env)
P=$(docker compose run --rm apikey generate-pepper)
grep -q '^APP_API_KEY_PEPPER=' .env \
  && sed -i "s|^APP_API_KEY_PEPPER=.*|APP_API_KEY_PEPPER=$P|" .env \
  || echo "APP_API_KEY_PEPPER=$P" >> .env

# 3. Create an API key. It is shown once; copy it.
docker compose run --rm apikey generate --name my-client

# 4. Optional: operator login for Swagger UI
echo 'my-operator-password' | docker compose run --rm -T apikey hash-ops-password
#    Put the printed hash in .env:
#    APP_OPS_USER=admin
#    APP_OPS_PASSWORD_HASH=<printed hash>

# 5. Start
docker compose up --build
```

In Docker, the key file is `secrets/.env.apikeys`. The service checks it every 30 seconds, so keys you add or revoke later take effect without a restart.

---

## Everyday key commands

Run these with the same pepper loaded (step A2), or with `docker compose run --rm apikey ...` instead of `go run ./cmd/apikey ...`.

| Task | Command |
|---|---|
| List keys (never shows hashes or keys) | `go run ./cmd/apikey list` |
| Add a key for a new client | `go run ./cmd/apikey generate --name pricing-service` |
| Revoke one key | `go run ./cmd/apikey revoke --id i7ne1jqz` |
| Revoke a group of keys | `go run ./cmd/apikey revoke --name-prefix app` |
| Replace a key (rotation) | Generate a new key, move the client to it, then revoke the old one |

---

## Production checklist

- The **pepper** comes from the environment or a secret manager. Never put it in the key file or in git.
- The **same pepper** is set on the operator's machine (where keys are made) and on every server.
- The **key file** has permission `0600` and is copied to every server. Each server logs a checksum of the file after loading it; the checksums must match.
- The `secrets/` folder and any `*.csv` handover files stay out of git (both are in `.gitignore`).
- Deliver bulk CSV files to client owners through a secure channel, then delete them. The CSV is the only copy of those keys.
- Use **HTTPS** (step 5 with a real certificate). API keys and the operator password travel in headers.

## When something does not work

| You see | Cause | Fix |
|---|---|---|
| Server stops with `API_KEYS_FILE ... no such file` | No key file yet | Do step 3 |
| Server stops with `API_KEY_PEPPER must be at least 32 characters` | Pepper missing or too short | Do step 1 |
| `401` with a key you just made | The key was made with a different pepper | Compare the fingerprints from `apikey` and from the server log; make the key again with the server's pepper |
| `401` on Swagger UI | Wrong password, or the hash was made with a different pepper | The server log says which one. Make the hash again (step 4) |
| `404` on Swagger UI | `APP_OPS_USER` or `APP_OPS_PASSWORD_HASH` is empty | Do step 4 and restart. The startup log says whether Swagger is on |
| `apikey: command not found` | The tool is not installed | Use `go run ./cmd/apikey ...`, or install it with `go install ./cmd/apikey` |
| Docker says `bind source path does not exist` | No `secrets/` folder | `mkdir -p secrets` |
