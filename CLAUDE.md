# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A tiny single-binary Go HTTP server (`serve-cloud-init`) that hands out per-node
cloud-init NoCloud seed data to Raspberry Pi nodes during NFS netboot. Pis fetch
their seed via `cmdline.txt`:

```
ds=nocloud;s=http://<kickstart_ip>:8000/<serial>/
```

The server is a thin wrapper around `http.FileServer` that adds request logging
and a `/healthcheck` endpoint. It serves
`<dir>/<serial>/{meta-data,user-data,network-config,vendor-data}`, where
`<serial>` is the Pi's CPU serial. Unknown serials → 404. `GET /healthcheck`
returns `200 OK` when `-dir` is statable and `503` otherwise — `/healthcheck`
is reserved (a node with serial `healthcheck` would shadow it).

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

This service is one of three coordinated repos. **Don't make changes here in
isolation when behavior is shared across them:**

- **tynet-cloud-init** (this repo) — serves seed data over HTTP at boot time.
  Distributed as a `.deb` published to GitHub Releases on tag push.
- **tynet-infra** — Ansible source of truth. Renders the runtime
  `<serial>/` seed tree on the kickstart host from inventory + `keys/*.pub`,
  installs `serve-cloud-init` via `apt` from a self-hosted aptly repo on
  `kickstart.tynet.us` (mirrored from GitHub Releases), and configures the
  service via `/etc/default/serve-cloud-init`.
- **tynet-img** — builds the Pi netboot image and provisions per-node TFTP
  (including the `cmdline.txt` that points here).

The runtime `cloud-init/` directory is **gitignored** — it only exists on the
kickstart host, populated by tynet-infra Ansible. Test fixtures live in
`testdata/cloud-init/` (serials `244634d3`, `a43386be`, plus `testnode`) and are
the only seed data this repo owns. If you change the on-disk layout (filenames,
directory shape, response semantics), the corresponding Ansible templates in
tynet-infra must be updated too.

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
