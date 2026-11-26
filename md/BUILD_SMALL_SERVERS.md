# Building on a small server (1–2 GB RAM)

`./deploy.sh build` can fail on a small VPS with a message that looks like a code error but is not:

```text
==> building server, apikey and sitectl
warn only 1109 MB free: building with GOGC=20 (slower, ~1.8 GB peak instead of ~2.7 GB)
github.com/openai/openai-go/v3: /usr/local/go/pkg/tool/linux_amd64/compile: signal: killed
error build failed for ./cmd/server
```

`signal: killed` is the **kernel OOM killer** (SIGKILL) stopping the Go compiler because the machine ran out of memory. It is not a Go error, not a compiler bug and not a problem in this repository. `deploy.sh` prints the same explanation under the failure.

This page explains why it happens, how to fix it with swap, and what to do when swap is not allowed.

## 1. Why one package needs so much memory

`internal/aicrawler` imports `github.com/openai/openai-go/v3` for the AI crawler's single chat-completions call. That module is one generated package of roughly 140 files, and the Go compiler compiles a package **as one unit**, so it needs the whole package in memory at once.

Measured on this project (`go build ./cmd/server`, `CGO_ENABLED=0`, cold cache):

| `GOGC` | Peak RSS of one `compile` | Notes |
|---|---|---|
| `100` (default) | **2.74 GB** | dies on a 2 GB server even with other compiles finished |
| `20` | **1.84 GB** | what `deploy.sh` switches to automatically under 4 GB free RAM |
| `10` | **1.70 GB** | hard floor is ~1.7 GB (live heap); going lower does not help |

Consequences:

- Freeing a few hundred MB is not enough. The compiler needs about 1.7 GB for that one package no matter how it is tuned.
- `-p 1` (one package at a time) does not lower the peak of a single compile; it only stops other compiles from adding to it.
- A machine with 1–2 GB of RAM and **no swap** cannot build this binary. The kernel kills the compiler every time.

## 2. Fix: add a swap file

A swap file lets the kernel page out while the compiler holds ~1.8 GB. The build becomes slow (expect 10–30 minutes on one vCPU) but it completes.

```bash
# run as root on the server

# 1. is there room for a swap file? (need ~size + 0.5 GB free on the root filesystem)
df -h /

# 2. create 4 GB of swap (fallocate is instant; dd is the fallback)
fallocate -l 4G /swapfile || dd if=/dev/zero of=/swapfile bs=1M count=4096 status=progress
chmod 600 /swapfile
mkswap /swapfile
swapon /swapfile

# 3. keep it after reboot
echo '/swapfile none swap sw 0 0' >> /etc/fstab

# 4. confirm
swapon --show
free -m
```

`free -m` should now show `Swap: 4096`, so the machine can give ~5 GB in total. Then rebuild:

```bash
cd /root/go-wordpress-extractor
sudo BUILD_JOBS=2 ./deploy.sh build
```

- `BUILD_JOBS=2` keeps two compiles in flight. More only adds disk thrashing on a small VPS.
- `GOGC=20` is applied automatically again because free RAM is still under 4 GB, so the peak is ~1.8 GB per compile.
- Run `free -m` in a second shell while it builds: swap in use is expected and healthy, not a sign of a hang.

If the size on disk is tight, 2 GB of swap is enough (the compile needs ~1.8 GB); 4 GB leaves room for the link step and for the rest of the system.

### Did the kernel really kill it?

```bash
dmesg -T | grep -iE 'killed process|out of memory' | tail -5
# Out of memory: Killed process 12345 (compile) total-vm:..., anon-rss:...
```

If `dmesg` prints nothing but the container was limited by a cgroup, check the limit instead:

```bash
cat /sys/fs/cgroup/memory.max 2>/dev/null || cat /sys/fs/cgroup/memory/memory.limit_in_bytes
```

A `memory.max` (cgroup v2) around 1–2 GB means the container is capped below what the compiler needs; swap inside the container then does not help, and the host limit must be raised.

## 3. When swap is not allowed

Some virtualisation types (OpenVZ, LXC without the swap capability, or a container with a memory cap) refuse to enable swap:

```bash
systemd-detect-virt          # openvz / lxc / kvm / none
swapon /swapfile             # swapon: /swapfile: swapon failed: Operation not permitted
```

In that case the machine cannot give ~1.8 GB to one process, and the build must happen somewhere else.

## 4. Build elsewhere and copy `bin/`

`CGO_ENABLED=0` binaries are static, so they run on the server whatever it has installed. The only requirement is the same OS and CPU architecture (`amd64` = `x86_64`, `arm64` = `aarch64`).

On a machine with enough RAM (your laptop or CI):

```bash
cd go-wordpress-extractor
mkdir -p bin
for c in server apikey sitectl; do
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -buildvcs=false -trimpath -ldflags="-s -w" -o bin/$c ./cmd/$c
done

rsync -av bin/ root@<server>:/root/go-wordpress-extractor/bin/
```

On the server:

```bash
cd /root/go-wordpress-extractor
chmod 755 bin bin/*

sudo ./deploy.sh service      # only if the systemd unit is not installed yet
sudo ./deploy.sh restart
```

Notes:

- Check the architecture first with `uname -m` on the server; use `GOARCH=arm64` for `aarch64`.
- Copy **all three** binaries. `deploy.sh` calls `bin/apikey` for the pepper and the API keys, so the pepper setup works again as soon as `bin/apikey` exists.
- Do **not** run `./deploy.sh install` or `./deploy.sh update` on the server afterwards: both rebuild from source and will hit the same OOM. Use `service` and `restart`, or `git pull` plus a fresh `rsync` of `bin/`.

## 5. Build knobs `deploy.sh` understands

| Variable | Default | Effect |
|---|---|---|
| `BUILD_JOBS=N` | one package per CPU | passes `-p N` to `go build`; limits how many compiles run at once. Does not lower a single compile's peak. |
| `BUILD_GOGC=N\|off` | `100`, or `20` automatically when `MemAvailable` < 4 GB | `GOGC` for the build. `20` cuts the peak from ~2.7 GB to ~1.8 GB at the cost of speed. `off` uses the most memory. |

Practical guidance by RAM:

| Server | Advice |
|---|---|
| ≥ 4 GB free | nothing to do, `./deploy.sh build` uses the default `GOGC` |
| 2–4 GB free | automatic `GOGC=20` is enough; `BUILD_JOBS=2` if other services are running |
| 1–2 GB, with swap | `sudo BUILD_JOBS=2 ./deploy.sh build`, keep the swap file enabled permanently |
| 1–2 GB, no swap | not possible; add swap or cross-build (section 4) |

Both values are validated: `BUILD_JOBS` must be a positive integer, `BUILD_GOGC` a number or `off`.

## 6. Optional: remove the cause

The 2.7 GB compile comes from one dependency used for one HTTP call. `internal/aicrawler` sends a single OpenAI-compatible chat-completions request; replacing the `openai-go` usage with a small `net/http` client (request struct, two headers, JSON decode, error mapping) removes the whole generated package from the build and drops the peak need to well under 1 GB. This is a code change, not a configuration change, and it is not required if swap is available.

## 7. Two things that commonly follow the build

- **Pepper.** After a successful build, `sudo ./deploy.sh pepper` generates `APP_API_KEY_PEPPER` (32 random bytes, 43-char base64url) into `.env.production`, mode `600`. Without a working `bin/apikey` you can bootstrap it by hand: `sed -i "s|^APP_API_KEY_PEPPER=.*|APP_API_KEY_PEPPER=$(openssl rand -hex 32)|" .env.production` (any 32+ character secret works). Details: [SETUP_KEYS.md](SETUP_KEYS.md).
- **Permissions.** The service runs as the unprivileged user `gwp`. If the project lives in `/root` (for example `/root/go-wordpress-extractor`), that user cannot read `bin/`, `configs/` or `.env.production`, and the service fails to start with "permission denied". Move the project to `/opt/go-wordpress-extractor` (or `/home/...`) before installing the unit; `deploy.sh` warns about this.

## Appendix: how to measure it yourself

```bash
# largest single process of one full build, in KB (needs the `time` package)
/usr/bin/time -v go build -a -o /tmp/x ./cmd/server 2>&1 | grep -E 'Maximum resident|Elapsed'

# RSS of every running compile while a build is in progress
ps -o rss=,args= -C compile | sort -n | tail -3

# swap and memory at a glance
free -m; swapon --show; vmstat 2 5
```
