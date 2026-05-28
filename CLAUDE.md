# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A tiny single-binary Go HTTP server (`tynet-cloud-init`) that hands out per-node
cloud-init NoCloud seed data to Raspberry Pi nodes during NFS netboot. Pis fetch
their seed via `cmdline.txt`:

```
ds=nocloud;s=http://<kickstart_ip>:8000/
```

Note there is no per-node key in the URL — `cmdline.txt` is identical for every
node. The server determines which node is calling by **reverse-DNS-resolving
the request's source IP**, taking the PTR result as the FQDN (trailing dot
stripped, e.g. `pi2.tynet.us.` → `pi2.tynet.us`), and serving
`<dir>/<fqdn>/{meta-data,user-data,network-config,vendor-data}`. PTR failure
or no matching directory → 404. Historically the key was the MAC address (e.g.
`dc-a6-32-8d-f3-ca`), and before that the CPU serial.

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
make test          # go test -v . against testdata/cloud-init/
make clean

go test -run TestServeCloudInit/pi2.tynet.us_user-data_ssh_key   # single subtest
go run . -dir testdata/cloud-init -addr :8000                    # ad-hoc local run
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
- **tya/tynet-apt** — GH-Pages-served apt repo at
  `https://tya.github.io/tynet-apt`. Its `ingest.yml` workflow runs on
  `repository_dispatch` (fired by this repo's release workflow), drops the
  `.deb` into the pool, regenerates signed apt indexes, and pushes
  `gh-pages`.
- **tynet-infra** — Ansible source of truth. Renders the runtime
  `<fqdn>/` seed tree on kickstart from inventory + `keys/*.pub`,
  owns the DHCP/DNS config that gives Pis working PTR records, configures
  the apt source pointing at `tya.github.io/tynet-apt`, and configures
  Ubuntu's native `unattended-upgrades` (scoped to `origin=tynet`, hourly
  cadence) to install/upgrade tynet packages. Configures the service via
  `/etc/default/tynet-cloud-init`.
- **tynet-img** — builds the Pi netboot image and provisions TFTP. Since
  the `cmdline.txt` carries no per-node key anymore, the same image/cmdline
  is used for every Pi.

**Release flow is fully autonomous for the steady state.** `git push origin
v0.X.Y` → release workflow publishes the `.deb` and dispatches into
`tya/tynet-apt` → ingest regenerates the GH-Pages apt repo within ~30s →
on kickstart, `apt-daily-upgrade.timer` fires within ≤1h, `unattended-upgrade`
picks up the new candidate, `apt-get install`s it. No manual
`make import-cloud-init-release` or pin bumps; total wall-clock from tag to
upgraded service is ≤1 hour. If a release ships a broken binary, the operator
notices via service monitoring (no auto-rollback); fix is to prune the bad
.deb from `pool/main/t/tynet-cloud-init/` on `tya/tynet-apt`'s `gh-pages`
branch and re-run the ingest workflow to rebuild indexes, then publish a
follow-up release.

The runtime `cloud-init/` directory is **gitignored** — it only exists on the
kickstart host, populated by tynet-infra Ansible. Test fixtures live in
`testdata/cloud-init/` keyed by FQDN (`pi2.tynet.us`, `pi3.tynet.us`, `testnode.vm`) —
see the directory for the current set. They're the only seed data this repo
owns. If you change the on-disk layout (filenames, directory shape, response
semantics), the corresponding Ansible templates in tynet-infra must be
updated too.

## Conventions worth knowing

- `defaultDir()` resolves to `<exe_dir>/cloud-init`, not CWD — convenient for
  development. The shipped systemd unit passes `-dir` explicitly via
  `/etc/default/tynet-cloud-init`, so this fallback isn't relied on in production.
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

`packaging/` holds everything the `.deb` ships and how it's built:

- `nfpm.yaml` — package definition (arm64, contents map, scripts). `${VERSION}`
  is interpolated by nfpm from the env var the Makefile sets.
- `tynet-cloud-init.service` — systemd unit. Runs as a dedicated
  `tynet-cloud-init` system user, sources `OPTIONS` from
  `/etc/default/tynet-cloud-init`. Generic — no host-specific values.
- `tynet-cloud-init.default` — the `OPTIONS` env file. Marked `config|noreplace`
  in nfpm.yaml so Ansible-managed edits survive `apt upgrade`.
- `postinst.sh` / `prerm.sh` / `postrm.sh` — create the system user, manage
  systemd enable/start/stop. Must be POSIX `sh`, not bash.

If you change file paths, the systemd unit, or the user/group, audit the
tynet-infra `kickstart` role at the same time — its `/etc/default/tynet-cloud-init`
template and seed-data dir permissions assume these conventions.
