# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A tiny single-binary Go HTTP server (`serve-cloud-init`) that hands out per-node
cloud-init NoCloud seed data to Raspberry Pi nodes during NFS netboot. Pis fetch
their seed via `cmdline.txt`:

```
ds=nocloud;s=http://<kickstart_ip>:8000/<serial>/
```

The server is a thin wrapper around `http.FileServer` that adds request logging.
It serves `<dir>/<serial>/{meta-data,user-data,network-config,vendor-data}`,
where `<serial>` is the Pi's CPU serial. Unknown serials → 404.

## Common commands

```sh
make build         # local build
make build-linux   # cross-compile linux/arm64 for the kickstart Pi
make test          # go test -v . against testdata/cloud-init/
make clean

go test -run TestServeCloudInit/pi2_user-data_ssh_key   # single subtest
go run . -dir testdata/cloud-init -addr :8000           # ad-hoc local run
```

Module path is `github.com/tya/tynet-cloud-init`, Go 1.22, no third-party deps.

## Architecture and the broader system

This service is one of three coordinated repos. **Don't make changes here in
isolation when behavior is shared across them:**

- **tynet-cloud-init** (this repo) — serves seed data over HTTP at boot time.
- **tynet-infra** — Ansible source of truth. Renders the runtime
  `cloud-init/<serial>/` tree on the kickstart host from inventory + `keys/*.pub`,
  and deploys this binary as a systemd service on `kickstart.tynet.us:8000`.
- **tynet-img** — builds the Pi netboot image and provisions per-node TFTP
  (including the `cmdline.txt` that points here).

The runtime `cloud-init/` directory is **gitignored** — it only exists on the
kickstart host, populated by tynet-infra Ansible. Test fixtures live in
`testdata/cloud-init/` (serials `244634d3`, `a43386be`, plus `testnode`) and are
the only seed data this repo owns. If you change the on-disk layout (filenames,
directory shape, response semantics), the corresponding Ansible templates in
tynet-infra must be updated too.

## Conventions worth knowing

- `defaultDir()` resolves to `<exe_dir>/cloud-init`, not CWD — so the systemd
  unit can run without a `-dir` flag if seed data is colocated with the binary.
- The handler logs every request (`remoteAddr method path`); preserve this when
  refactoring — it's the only operational visibility into Pi boot attempts.
- Tests assert on **substring presence** in response bodies (`#cloud-config`,
  `instance-id:`, `ssh-ed25519 `) rather than exact content, so fixtures can
  evolve without churning tests. Follow that pattern for new assertions.
