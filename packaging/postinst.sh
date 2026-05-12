#!/bin/sh
set -e

if ! getent passwd tynet-cloud-init >/dev/null; then
    adduser --system --group --no-create-home \
        --home /var/lib/tynet-cloud-init \
        --shell /usr/sbin/nologin \
        tynet-cloud-init
fi

install -d -o tynet-cloud-init -g tynet-cloud-init -m 0755 /var/lib/tynet-cloud-init

if [ -d /run/systemd/system ]; then
    systemctl daemon-reload
    systemctl enable tynet-cloud-init.service
    systemctl restart tynet-cloud-init.service || true
fi
