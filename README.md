# tynet-cloud-init

HTTP server for per-node cloud-init seed data on tynet.us infrastructure.

Serves files from `<dir>/<serial>/{meta-data,user-data,network-config,vendor-data}`,
keyed by Pi serial number. Pi nodes booting via NFS netboot fetch their
seed data via `cmdline.txt`:

```
ds=nocloud;s=http://<kickstart_ip>:8000/<serial>/
```

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

Seed files live in `<dir>/<serial>/` at runtime — **rendered by
[tynet-infra](https://github.com/tya/tynet-infra) Ansible** from
inventory and `keys/*.pub`. Fixtures used by `go test` are in
`testdata/cloud-init/`.

## Releasing

Tag with semver and push:

```sh
git tag v0.1.0
git push origin v0.1.0
```

`.github/workflows/release.yml` runs on tag push, builds the arm64 `.deb`,
and publishes it to a GitHub Release. The kickstart host pulls the .deb
from there into its local apt mirror.

## Deployment

Distributed as a Debian package (`serve-cloud-init`) installed via apt by
the kickstart Ansible role in [tynet-infra](https://github.com/tya/tynet-infra).
The package ships a systemd unit that runs as the `serve-cloud-init` system
user and reads runtime options from `/etc/default/serve-cloud-init`:

```
OPTIONS="-dir /var/lib/serve-cloud-init -addr :8000"
```

Ansible templates that file to point `-dir` at the rendered seed-data tree.

The .deb also installs `/usr/bin/serve-cloud-init-probe`, a small POSIX
shell tool that fetches all four cloud-init files for a given key and
prints them with section headers — useful for verifying a node's seed
data is being served correctly without booting the node:

```sh
serve-cloud-init-probe dc-a6-32-8d-f3-ca                             # default: localhost:8000
serve-cloud-init-probe dc-a6-32-8d-f3-ca kickstart.tynet.us:8000     # explicit host
```

## Related

- [tynet-img](https://github.com/tya/tynet-img) — Pi netboot image build + per-node TFTP provisioning
- [tynet-infra](https://github.com/tya/tynet-infra) — Ansible source of truth for node identity, deploys this service
