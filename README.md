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
make test          # uses testdata/cloud-init/
```

## Per-node seed data

Seed files live in `cloud-init/<serial>/` at runtime — **rendered by
[tynet-infra](https://github.com/tya/tynet-infra) Ansible** from
inventory and `keys/*.pub`. The directory is gitignored here; fixtures
used by `go test` are in `testdata/cloud-init/`.

## Deployment

Managed as a systemd service by the kickstart Ansible role in tynet-infra.
The service runs `serve-cloud-init -dir <cloud_init_dir>` on
`kickstart.tynet.us:8000`.

## Related

- [tynet-img](https://github.com/tya/tynet-img) — Pi netboot image build + per-node TFTP provisioning
- [tynet-infra](https://github.com/tya/tynet-infra) — Ansible source of truth for node identity, deploys this service
