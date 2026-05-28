#!/bin/sh
# tynet-cloud-init-probe — fetch a node's cloud-init seed data over HTTP
# from a tynet-cloud-init server. Useful for verifying that the per-node
# tree under -dir is being served correctly without booting the node.

set -eu

usage() {
    cat <<EOF
Usage: tynet-cloud-init-probe [--check] <fqdn> [host[:port]]

Fetches /node/<fqdn>/{meta-data,user-data,network-config,vendor-data}
from the tynet-cloud-init HTTP server. Default host is localhost:8000;
pass a hostname or full URL to probe a different server.

The /node/ prefix is the server's explicit-override route, used so the
probe can ask about any node regardless of which IP the request comes
from (normal cloud-init traffic resolves the node via reverse DNS).

Without --check, prints each response separated by section headers.
With --check, runs substring assertions on each response and exits
non-zero if any are missing — suitable for scripts and post-deploy
smoke tests. The required substrings mirror the Go server's tests.

  tynet-cloud-init-probe pi2.tynet.us
  tynet-cloud-init-probe pi2.tynet.us kickstart.tynet.us:8000
  tynet-cloud-init-probe --check pi2.tynet.us kickstart.tynet.us:8000
EOF
}

check_mode=0
if [ "${1:-}" = "--check" ] || [ "${1:-}" = "-c" ]; then
    check_mode=1
    shift
fi

if [ $# -lt 1 ] || [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ]; then
    usage
    [ $# -lt 1 ] && exit 2 || exit 0
fi

key=$1
host=${2:-localhost:8000}
case "$host" in
    http://*|https://*) ;;
    *) host="http://$host" ;;
esac

if [ "$check_mode" -eq 0 ]; then
    rc=0
    for file in meta-data user-data network-config vendor-data; do
        url="$host/node/$key/$file"
        printf '===== %s =====\n' "$url"
        if ! curl -sS -f "$url"; then
            printf '\n(fetch failed)\n'
            rc=1
        fi
        printf '\n'
    done
    exit $rc
fi

# --check mode: assert each response is non-empty and contains the
# required substrings (mirroring the Go server's tests in main_test.go).
fail=0

check_present() {
    file=$1
    shift
    url="$host/node/$key/$file"
    body=$(curl -sS -f "$url" 2>/dev/null) || {
        printf 'FAIL %s: fetch failed (HTTP error or unreachable)\n' "$url" >&2
        fail=1
        return
    }
    if [ -z "$body" ]; then
        printf 'FAIL %s: empty body\n' "$url" >&2
        fail=1
        return
    fi
    for substr in "$@"; do
        if ! printf '%s' "$body" | grep -qF "$substr"; then
            printf 'FAIL %s: missing substring %s\n' "$url" "$substr" >&2
            fail=1
        fi
    done
}

check_present meta-data 'instance-id:' 'local-hostname:'
check_present user-data '#cloud-config' 'ssh-ed25519 '
check_present network-config
check_present vendor-data

if [ "$fail" -ne 0 ]; then
    exit 1
fi

printf 'ok: %s via %s\n' "$key" "$host"
