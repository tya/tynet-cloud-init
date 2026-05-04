#!/bin/sh
set -e

if ! getent passwd serve-cloud-init >/dev/null; then
    adduser --system --group --no-create-home \
        --home /var/lib/serve-cloud-init \
        --shell /usr/sbin/nologin \
        serve-cloud-init
fi

install -d -o serve-cloud-init -g serve-cloud-init -m 0755 /var/lib/serve-cloud-init

if [ -d /run/systemd/system ]; then
    systemctl daemon-reload
    systemctl enable serve-cloud-init.service
    systemctl restart serve-cloud-init.service || true
fi
