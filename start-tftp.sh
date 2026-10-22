#!/bin/sh
set -eu

in.tftpd --foreground --address 127.0.0.1:1069 --secure --user nobody /srv/tftp &
server_pid=$!
trap 'kill "$server_pid" 2>/dev/null || true' EXIT INT TERM
exec /usr/local/bin/tftp-monitor
