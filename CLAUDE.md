# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A tiny single-binary Go HTTP server (`serve-cloud-init`) that hands out per-node
cloud-init NoCloud seed data to Raspberry Pi nodes during NFS netboot. Pis fetch
their seed via `cmdline.txt`:

```
ds=nocloud;s=http://<kickstart_ip>:8000/<key>/
```

The `<key>` is an opaque path segment matching a directory under `-dir`. In
production it's the node's MAC (e.g. `dc-a6-32-8d-f3-ca`); historically it was
the CPU serial. The server doesn't care — it's a thin wrapper around
`http.FileServer` that adds request logging and a `/healthcheck` endpoint. It
serves `<dir>/<key>/{meta-data,user-data,network-config,vendor-data}`. Unknown
keys → 404. `GET /healthcheck` returns `200 OK` when `-dir` is statable and
`503` otherwise — `/healthcheck` is reserved (a node keyed literally
`healthcheck` would shadow it).

## Common commands

```sh
make build         # local build
make build-linux   # cross-compile linux/arm64 for the kickstart Pi
make deb           # build linux/arm64 .deb into dist/ (requires nfpm: brew install goreleaser/tap/nfpm)
make test          # go test -v . against testdata/cloud-init/
make clean

go test -run TestServeCloudInit/pi2_user-data_ssh_key   # single subtest
go run . -dir testdata/cloud-init -addr :8000           # ad-hoc local run
```

`make deb` derives `VERSION` from `git describe` (falling back to `0.0.0~dev`
on a repo with no tags); override for ad-hoc builds: `make deb VERSION=0.0.1`.
Releases happen via tag push: `git tag v0.1.0 && git push origin v0.1.0`
triggers `.github/workflows/release.yml`, which builds the .deb and attaches it
to a GitHub Release.

Module path is `github.com/tya/tynet-cloud-init`, Go 1.22, no third-party deps.

## Architecture and the broader system

This service is one of four coordinated repos. **Don't make changes here in
isolation when behavior is shared across them:**

- **tynet-cloud-init** (this repo) — serves seed data over HTTP at boot time.
  Distributed as a `.deb` published to GitHub Releases on tag push.
- **tya/tynet-apt** — GH-Pages-served apt repo at
  `https://tya.github.io/tynet-apt`. Its `ingest.yml` workflow runs on
  `repository_dispatch` (fired by this repo's release workflow), drops the
  `.deb` into the pool, regenerates signed apt indexes, and pushes
  `gh-pages`.
- **tynet-deb-installer** (shipped from `tya/tynet-github-puller`) — runs on
  `kickstart.tynet.us` via a 60s systemd timer. `apt-get install`s the
  current candidate of each managed package, then runs a per-package
  healthcheck and auto-rolls-back on failure (`serve-cloud-init`'s
  healthcheck is `GET /healthcheck`).
- **tynet-infra** — Ansible source of truth. Renders the runtime `<key>/`
  seed tree on kickstart from inventory + `keys/*.pub`, configures the apt
  source pointing at `tya.github.io/tynet-apt`, templates the
  deb-installer's managed-package list. Configures the service via
  `/etc/default/serve-cloud-init`.
- **tynet-img** — builds the Pi netboot image and provisions per-node TFTP
  (including the `cmdline.txt` that points here).

**Release flow is fully autonomous for the steady state.** `git push origin
v0.X.Y` → release workflow publishes the `.deb` and dispatches into
`tya/tynet-apt` → ingest regenerates the GH-Pages apt repo within ~30s →
deb-installer auto-upgrades on its next tick (~60s) with healthcheck →
rollback on failure. No manual `make import-cloud-init-release` or pin
bumps. Be aware of this when shipping a release: a healthy tag goes live
within ~2 minutes; an unhealthy one rolls back automatically and the broken
`.deb` stays in the apt pool until pruned (drop the file from `pool/main/s/
serve-cloud-init/` on `tya/tynet-apt`'s `gh-pages` branch and re-run the
ingest workflow to rebuild indexes).

The runtime `cloud-init/` directory is **gitignored** — it only exists on the
kickstart host, populated by tynet-infra Ansible. Test fixtures live in
`testdata/cloud-init/` keyed by MAC (e.g. `dc-a6-32-8d-f3-ca`,
`dc-a6-32-80-2a-cc`, `52-55-55-60-97-49`) — see the directory for the
current set. They're the only seed data this repo owns. If you change the
on-disk layout (filenames, directory shape, response semantics), the
corresponding Ansible templates in tynet-infra must be updated too.

## Conventions worth knowing

- `defaultDir()` resolves to `<exe_dir>/cloud-init`, not CWD — convenient for
  development. The shipped systemd unit passes `-dir` explicitly via
  `/etc/default/serve-cloud-init`, so this fallback isn't relied on in production.
- The handler logs every request (`remoteAddr method path`); preserve this when
  refactoring — it's the only operational visibility into Pi boot attempts.
- Tests assert on **substring presence** in response bodies (`#cloud-config`,
  `instance-id:`, `ssh-ed25519 `) rather than exact content, so fixtures can
  evolve without churning tests. Follow that pattern for new assertions.

## Packaging layout

`packaging/` holds everything the `.deb` ships and how it's built:

- `nfpm.yaml` — package definition (arm64, contents map, scripts). `${VERSION}`
  is interpolated by nfpm from the env var the Makefile sets.
- `serve-cloud-init.service` — systemd unit. Runs as a dedicated
  `serve-cloud-init` system user, sources `OPTIONS` from
  `/etc/default/serve-cloud-init`. Generic — no host-specific values.
- `serve-cloud-init.default` — the `OPTIONS` env file. Marked `config|noreplace`
  in nfpm.yaml so Ansible-managed edits survive `apt upgrade`.
- `postinst.sh` / `prerm.sh` / `postrm.sh` — create the system user, manage
  systemd enable/start/stop. Must be POSIX `sh`, not bash.

If you change file paths, the systemd unit, or the user/group, audit the
tynet-infra `kickstart` role at the same time — its `/etc/default/serve-cloud-init`
template and seed-data dir permissions assume these conventions.
