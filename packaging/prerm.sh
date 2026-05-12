#!/bin/sh
set -e

if [ -d /run/systemd/system ]; then
    systemctl stop tynet-cloud-init.service || true
    systemctl disable tynet-cloud-init.service || true
fi
