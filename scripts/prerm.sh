#!/bin/sh
set -e

# Stop systemd service
if [ -d /run/systemd/system ] && ( [ "$1" = remove ] || [ "$1" = "0" ] ); then
  echo "Stopping ntfy.service ..."
  if [ -x /usr/bin/deb-systemd-invoke ]; then
    deb-systemd-invoke stop 'ntfy.service' >/dev/null || true
  else
    systemctl stop ntfy >/dev/null 2>&1 || true
  fi
fi
