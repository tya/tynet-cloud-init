# tynet-cloud-init

HTTP server for per-node cloud-init seed data on tynet.us infrastructure.

Serves files from `<dir>/<fqdn>/{meta-data,user-data,network-config,vendor-data}`.
The node is identified by **reverse-DNS-resolving the request's source IP**:
the PTR result (trailing dot stripped, e.g. `pi2.tynet.us`) names the seed
directory. PTR failure or no matching directory → 404. Historically the key
was the MAC address, and before that the CPU serial.

Pi nodes booting via NFS netboot fetch their seed data via `cmdline.txt`,
which is identical for every node:

```
ds=nocloud;s=https://cloud-init.tynet.us/
```

Two routes bypass the reverse-DNS lookup:

- `GET /healthcheck` → `200 OK` (`ok\n`) when `-dir` is statable,
  `503 Service Unavailable` when it's missing or unreadable.
- `GET /node/<fqdn>/<file>` → serves `<dir>/<fqdn>/<file>` directly, so
  any node can be probed from any host.

`healthcheck` and `node` are reserved — a node with either name would
shadow these routes.

TLS is terminated in the binary when both `-tls-cert` and `-tls-key` are
set (plain HTTP otherwise). The cert is loaded once at startup; rotation
needs a restart.

## Build & test

```sh
make build         # local
make build-linux   # cross-compile for kickstart host (linux/arm64)
make deb           # build linux/arm64 .deb into dist/ (requires nfpm)
make image         # build linux/arm64 container image (requires Docker buildx)
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

Seed files live in `<dir>/<fqdn>/` at runtime — **rendered by
[tynet-infra](https://github.com/tya/tynet-infra) Ansible** from
inventory and `keys/*.pub`. Fixtures used by `go test` are in
`testdata/cloud-init/`, keyed by FQDN (`pi2.tynet.us`, `pi3.tynet.us`,
`testnode.vm`).

## Releasing

Tag with semver and push:

```sh
git tag v0.1.0
git push origin v0.1.0
```

Two workflows run in parallel on tag push:

- `.github/workflows/image.yml` builds the linux/arm64 container image and
  pushes `:v0.X.Y` and `:latest` to `ghcr.io/tya/tynet-cloud-init`.
- `.github/workflows/release.yml` builds the arm64 `.deb`, publishes it to a
  GitHub Release, and dispatches `new-release` into
  [tya/tynet-apt](https://github.com/tya/tynet-apt) (the GH-Pages-served apt
  repo at `https://tya.github.io/tynet-apt`). The production cluster doesn't
  consume the `.deb`; it's kept for ad-hoc installs.

A tag push alone does not deploy anything — see below.

## Deployment

Production runs as a single pod in the microk8s cluster on kickstart,
deployed by ArgoCD from the Helm chart in
[`deploy/charts/tynet-cloud-init`](deploy/charts/tynet-cloud-init). The
Argo Application lives in tynet-infra (`gitops/cloud-init/application.yaml`)
and tracks this repo's `main` branch with **automated sync and prune**.

The chart pins a real image tag (not `:latest`). To roll out a release,
bump `image.tag` in `deploy/charts/tynet-cloud-init/values.yaml` and merge
to `main` — Argo syncs it within its polling interval (Argo default ≈3 min), no manual
`argocd app sync` needed. Anything else merged under the chart deploys the
same way, so treat chart PRs as deploys. Roll back by reverting the bump
and merging; there's no automated rollback.

The Service is `type: LoadBalancer` on MetalLB VIP `10.0.60.60` with
`externalTrafficPolicy: Local` (preserving the Pi's source IP for the
reverse-DNS lookup), port 443 → container 8443. The per-node seed tree at
`/var/lib/tynet-cloud-init` on kickstart is hostPath-mounted into the pod;
tynet-infra's `kickstart` role renders it.

`helm template deploy/charts/tynet-cloud-init` renders the chart locally;
`helm lint` catches structural mistakes before a PR.

### `.deb` (ad-hoc installs)

The package ships a systemd unit that runs as the `tynet-cloud-init` system
user and reads runtime options from `/etc/default/tynet-cloud-init`:

```
OPTIONS="-dir /var/lib/tynet-cloud-init -addr :8000"
```

The .deb also installs `/usr/bin/tynet-cloud-init-probe`, a small POSIX
shell tool that fetches all four cloud-init files for a node (via the
`/node/<fqdn>/` route) —
useful for verifying a node's seed data is being served correctly
without booting the node:

```sh
tynet-cloud-init-probe pi2.tynet.us                                   # default: localhost:8000
tynet-cloud-init-probe pi2.tynet.us https://cloud-init.tynet.us       # production
```

Pass `--check` for a smoke-test mode that asserts each response is
present and contains the same substrings the Go server's tests check
for (`#cloud-config`, `instance-id:`, `ssh-ed25519 `, etc.). Prints
`ok: <fqdn> via <host>` and exits 0 on success; prints per-failure
diagnostics to stderr and exits 1 on any miss:

```sh
tynet-cloud-init-probe --check pi2.tynet.us https://cloud-init.tynet.us
```

## Related

- [tynet-img](https://github.com/tya/tynet-img) — Pi netboot image build + per-node TFTP provisioning
- [tynet-infra](https://github.com/tya/tynet-infra) — Ansible source of truth for node identity and DNS; renders the seed tree and owns the ArgoCD Application that deploys this chart
