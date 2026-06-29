# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A tiny single-binary Go HTTP server (`tynet-cloud-init`) that hands out per-node
cloud-init NoCloud seed data to Raspberry Pi nodes during NFS netboot. Pis fetch
their seed via `cmdline.txt`:

```
ds=nocloud;s=https://cloud-init.tynet.us/
```

`cloud-init.tynet.us` resolves (via UniFi DNS) to a MetalLB VIP in front of a
single pod running in the microk8s cluster on kickstart. The Service is
`type: LoadBalancer` with `externalTrafficPolicy: Local`, which preserves the
Pi's real source IP — essential for what the handler does next.

Note there is no per-node key in the URL — `cmdline.txt` is identical for every
node. The server determines which node is calling by **reverse-DNS-resolving
the request's source IP**, taking the PTR result as the FQDN (trailing dot
stripped, e.g. `pi2.tynet.us.` → `pi2.tynet.us`), and serving
`<dir>/<fqdn>/{meta-data,user-data,network-config,vendor-data}`. PTR failure
or no matching directory → 404. Historically the key was the MAC address (e.g.
`dc-a6-32-8d-f3-ca`), and before that the CPU serial.

TLS is terminated in the binary itself (`-tls-cert` / `-tls-key`), loaded from
a `kubernetes.io/tls` Secret synced from the wildcard `*.tynet.us` cert on
`vpn.tynet.us`. The cert is loaded once at startup, so cert rotation rolls the
pod via a `checksum/tls` annotation on the Deployment.

Two paths bypass the reverse-DNS lookup:

- `GET /healthcheck` → `200 OK` when `-dir` is statable, `503` otherwise.
- `GET /node/<fqdn>/<file>` → serves `<dir>/<fqdn>/<file>` directly.
  Used by `tynet-cloud-init-probe` so operators can probe any node from any
  host without depending on their own source IP.

`healthcheck` and `node` are reserved hostnames — a node keyed literally
`healthcheck` or `node` would shadow these routes.

**Runtime dependency:** reverse DNS for the Pi DHCP range must work on the
kickstart host. Verify with `dig -x <pi-ip>` before deploying. tynet-infra
owns DHCP/DNS configuration.

## Common commands

```sh
make build         # local build
make build-linux   # cross-compile linux/arm64 for the kickstart Pi
make deb           # build linux/arm64 .deb into dist/ (requires nfpm: brew install goreleaser/tap/nfpm)
make image         # build linux/arm64 container image (requires Docker buildx)
make push-image    # build + push ghcr.io/tya/tynet-cloud-init:$(VERSION) and :latest
make test          # go test -v . against testdata/cloud-init/
make clean

go test -run TestServeCloudInit/pi2.tynet.us_user-data_ssh_key   # single subtest
go run . -dir testdata/cloud-init -addr :8000                    # ad-hoc local run (plain HTTP)
```

`make deb` derives `VERSION` from `git describe` (falling back to `0.0.0~dev`
on a repo with no tags); override for ad-hoc builds: `make deb VERSION=0.0.1`.
Releases happen via tag push: `git tag v0.1.0 && git push origin v0.1.0`
triggers two workflows in parallel — `.github/workflows/release.yml` builds the
`.deb` and attaches it to a GitHub Release, and `.github/workflows/image.yml`
builds and pushes the container image to `ghcr.io/tya/tynet-cloud-init`.

Module path is `github.com/tya/tynet-cloud-init`, Go 1.22, no third-party deps.

## Architecture and the broader system

This service is one of several coordinated repos. **Don't make changes here in
isolation when behavior is shared across them:**

- **tynet-cloud-init** (this repo) — serves seed data over HTTPS at boot time.
  Distributed as a container image at `ghcr.io/tya/tynet-cloud-init`
  (production) and as a `.deb` on GitHub Releases (for ad-hoc / fallback
  installs).
- **tynet-infra** — Ansible source of truth. The `cloud_init_cluster` role
  renders the workload manifests on kickstart, syncs the wildcard `*.tynet.us`
  cert from `vpn.tynet.us` into a `kubernetes.io/tls` Secret, and applies
  Namespace / Secret / Deployment / Service via `microk8s kubectl`. The
  Deployment is pinned to the kickstart node (`nodeSelector`) and hostPath-
  mounts the seed tree the `kickstart` role keeps rendering at
  `/var/lib/tynet-cloud-init/<fqdn>/`. Owns the UniFi DHCP/DNS that gives
  Pis working PTR records (essential) plus the `cloud-init.tynet.us` A record
  pointing at the MetalLB VIP (`10.0.60.60`, manual UniFi step).
- **tya/tynet-apt** — GH-Pages-served apt repo at
  `https://tya.github.io/tynet-apt`. Still receives `.deb` uploads on tag push
  via `repository_dispatch`, but the in-cluster deployment doesn't consume
  them. Kept for ad-hoc installs.
- **tynet-img** — builds the Pi netboot image and provisions TFTP. The
  `cmdline.txt` (`ds=nocloud;s=https://cloud-init.tynet.us/`) is identical
  for every node.

**Release flow.** `git push origin v0.X.Y` → `.github/workflows/image.yml`
builds the container image and pushes `:v0.X.Y` and `:latest` to ghcr.io
(in parallel, `release.yml` still ships the `.deb`). Picking up the new image
requires either (a) `microk8s kubectl rollout restart -n cloud-init
deployment/tynet-cloud-init` to re-pull `:latest`, or (b) bumping
`cloud_init_image_tag` in `roles/cloud_init_cluster/defaults/main.yml` and
re-running `make cloud-init-cluster`. There is no auto-rollout — broken
releases stay deployed until rolled back manually.

The runtime `cloud-init/` directory is **gitignored** — it only exists on the
kickstart host, populated by tynet-infra Ansible. Test fixtures live in
`testdata/cloud-init/` keyed by FQDN (`pi2.tynet.us`, `pi3.tynet.us`, `testnode.vm`) —
see the directory for the current set. They're the only seed data this repo
owns. If you change the on-disk layout (filenames, directory shape, response
semantics), the corresponding Ansible templates in tynet-infra must be
updated too.

## Conventions worth knowing

- `defaultDir()` resolves to `<exe_dir>/cloud-init`, not CWD — convenient for
  development. In the cluster the Deployment passes `-dir` explicitly
  (`/var/lib/tynet-cloud-init`), so this fallback isn't relied on in production.
- TLS is on iff *both* `-tls-cert` and `-tls-key` are set. The binary loads the
  cert once via `ListenAndServeTLS` and does **not** hot-reload — rotation
  needs a pod restart. The Deployment's `checksum/tls` annotation (a sha256
  of cert+key, recomputed by Ansible on every render) is what triggers it.
- The handler logs every request (`remoteAddr method path`) plus a follow-up
  `resolved <ip> -> <fqdn>` (or `reverse lookup failed for <ip>: …`) line;
  preserve both when refactoring — they're the only operational visibility
  into Pi boot attempts.
- The reverse-DNS lookup function is injected into `newHandler` so tests can
  stub it; production wires in `net.LookupAddr`. Keep it injectable.
- Tests assert on **substring presence** in response bodies (`#cloud-config`,
  `instance-id:`, `ssh-ed25519 `) rather than exact content, so fixtures can
  evolve without churning tests. Follow that pattern for new assertions.

## Packaging layout

Two parallel distribution channels:

- **Container image** (production) — built by `.github/workflows/image.yml`
  from the repo-root `Dockerfile`. Multi-stage golang→distroless/static,
  runs as nonroot UID 65532. The in-cluster Deployment in tynet-infra's
  `cloud_init_cluster` role pulls from `ghcr.io/tya/tynet-cloud-init`.
- **`.deb` package** (ad-hoc) — built by `.github/workflows/release.yml`
  using nfpm. Still useful for direct-on-host installs and for the
  apt-repo pipeline, but the production cluster no longer consumes it.

`packaging/` holds everything the `.deb` ships:

- `nfpm.yaml` — package definition (arm64, contents map, scripts).
- `tynet-cloud-init.service` — systemd unit (sources `OPTIONS` from
  `/etc/default/tynet-cloud-init`, runs as `tynet-cloud-init` system user).
- `tynet-cloud-init.default` — the `OPTIONS` env file (`config|noreplace`).
- `postinst.sh` / `prerm.sh` / `postrm.sh` — manage the system user and
  systemd lifecycle. POSIX `sh`, not bash.

If you change file paths, the systemd unit, or the user/group, audit the
tynet-infra `cloud_init_cluster` (image-side) and `kickstart` (seed-tree
side) roles at the same time.
