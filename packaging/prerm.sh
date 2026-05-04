#!/bin/sh
set -e

if [ -d /run/systemd/system ]; then
    systemctl stop serve-cloud-init.service || true
    systemctl disable serve-cloud-init.service || true
fi
