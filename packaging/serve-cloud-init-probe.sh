#!/bin/sh
# serve-cloud-init-probe — fetch a node's cloud-init seed data over HTTP
# from a serve-cloud-init server. Useful for verifying that the per-node
# tree under -dir is being served correctly without booting the node.

set -eu

usage() {
    cat <<EOF
Usage: serve-cloud-init-probe <mac-or-serial> [host[:port]]

Fetches /<key>/{meta-data,user-data,network-config,vendor-data} from the
serve-cloud-init HTTP server and prints each response, separated by
section headers. Default host is localhost:8000; pass a hostname or full
URL to probe a different server.

  serve-cloud-init-probe dc-a6-32-8d-f3-ca
  serve-cloud-init-probe dc-a6-32-8d-f3-ca kickstart.tynet.us:8000
  serve-cloud-init-probe dc-a6-32-8d-f3-ca http://kickstart.tynet.us:8000
EOF
}

if [ $# -lt 1 ] || [ "$1" = "-h" ] || [ "$1" = "--help" ]; then
    usage
    [ $# -lt 1 ] && exit 2 || exit 0
fi

key=$1
host=${2:-localhost:8000}
case "$host" in
    http://*|https://*) ;;
    *) host="http://$host" ;;
esac

rc=0
for file in meta-data user-data network-config vendor-data; do
    url="$host/$key/$file"
    printf '===== %s =====\n' "$url"
    if ! curl -sS -f "$url"; then
        printf '\n(fetch failed)\n'
        rc=1
    fi
    printf '\n'
done

exit $rc
