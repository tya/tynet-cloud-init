# tynet-cloud-init

HTTP server for per-node cloud-init seed data on tynet.us infrastructure.

Serves files from `<dir>/<key>/{meta-data,user-data,network-config,vendor-data}`,
where `<key>` is whatever path segment names a Pi's seed directory. In
production it's the node's MAC (e.g. `dc-a6-32-8d-f3-ca`); historically
it was the CPU serial. The server doesn't care — anything matching a
directory under `-dir` works. Pi nodes booting via NFS netboot fetch their
seed data via `cmdline.txt`:

```
ds=nocloud;s=http://<kickstart_ip>:8000/<key>/
```

Also exposes `GET /healthcheck`: returns `200 OK` (`ok\n`) when `-dir` is
statable, `503 Service Unavailable` when the directory is missing or
unreadable. The path `/healthcheck` is reserved — a node keyed literally
`healthcheck` would shadow this endpoint.

## Build & test

```sh
make build         # local
make build-linux   # cross-compile for kickstart host (linux/arm64)
make deb           # build linux/arm64 .deb into dist/ (requires nfpm)
make test          # uses testdata/cloud-init/
```

`nfpm` is the only extra build dep (single static Go binary, no runtime deps):

```sh
# macOS
brew install goreleaser/tap/nfpm

# Debian/Ubuntu (goreleaser apt repo)
echo 'deb [trusted=yes] https://repo.goreleaser.com/apt/ /' | sudo tee /etc/apt/sources.list.d/goreleaser.list
sudo apt update && sudo apt install nfpm

# Any Linux (pinned tarball — matches CI)
NFPM_VERSION=2.46.3
curl -sSL "https://github.com/goreleaser/nfpm/releases/download/v${NFPM_VERSION}/nfpm_${NFPM_VERSION}_Linux_x86_64.tar.gz" \
  | sudo tar -xz -C /usr/local/bin nfpm

# Or via Go (you already have it for this repo)
go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest
```

## Per-node seed data

Seed files live in `<dir>/<key>/` at runtime — **rendered by
[tynet-infra](https://github.com/tya/tynet-infra) Ansible** from
inventory and `keys/*.pub`. Fixtures used by `go test` are in
`testdata/cloud-init/` (currently MAC-keyed: `dc-a6-32-8d-f3-ca`,
`dc-a6-32-80-2a-cc`, `52-55-55-60-97-49`).

## Releasing

Tag with semver and push:

```sh
git tag v0.1.0
git push origin v0.1.0
```

`.github/workflows/release.yml` runs on tag push, builds the arm64 `.deb`,
publishes it to a GitHub Release, and dispatches `new-release` into
[tya/tynet-apt](https://github.com/tya/tynet-apt). That repo's `ingest`
workflow drops the deb into its pool, regenerates the signed apt indexes,
and pushes to `gh-pages` — the GH-Pages-served apt repo at
`https://tya.github.io/tynet-apt`. On kickstart, the OS's native
`unattended-upgrades` (configured by tynet-infra, scoped to `origin=tynet`)
runs hourly, sees the new candidate, and `apt-get install`s it. End-to-end:
healthy tag goes live within ≤1 hour; nothing on the kickstart side needs
a manual bump.

## Deployment

Distributed as a Debian package (`tynet-cloud-init`) installed via apt by
the kickstart Ansible role in [tynet-infra](https://github.com/tya/tynet-infra).
The package ships a systemd unit that runs as the `tynet-cloud-init` system
user and reads runtime options from `/etc/default/tynet-cloud-init`:

```
OPTIONS="-dir /var/lib/tynet-cloud-init -addr :8000"
```

Ansible templates that file to point `-dir` at the rendered seed-data tree.

The .deb also installs `/usr/bin/tynet-cloud-init-probe`, a small POSIX
shell tool that fetches all four cloud-init files for a given key —
useful for verifying a node's seed data is being served correctly
without booting the node:

```sh
tynet-cloud-init-probe dc-a6-32-8d-f3-ca                             # default: localhost:8000
tynet-cloud-init-probe dc-a6-32-8d-f3-ca kickstart.tynet.us:8000     # explicit host
```

Pass `--check` for a smoke-test mode that asserts each response is
present and contains the same substrings the Go server's tests check
for (`#cloud-config`, `instance-id:`, `ssh-ed25519 `, etc.). Prints
`ok: <key> via <host>` and exits 0 on success; prints per-failure
diagnostics to stderr and exits 1 on any miss:

```sh
tynet-cloud-init-probe --check dc-a6-32-8d-f3-ca kickstart.tynet.us:8000
```

## Related

- [tynet-img](https://github.com/tya/tynet-img) — Pi netboot image build + per-node TFTP provisioning
- [tynet-infra](https://github.com/tya/tynet-infra) — Ansible source of truth for node identity, deploys this service
