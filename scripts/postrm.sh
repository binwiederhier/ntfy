#!/bin/sh
set -eu
systemctl stop ntfy >/dev/null 2>&1 || true
if [ "$1" = "purge" ]; then
  rm -rf /etc/ntfy
fi
