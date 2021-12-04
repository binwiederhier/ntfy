#!/bin/sh
set -e

# Restart systemd service if it was already running. Note that "deb-systemd-invoke try-restart" will
# only act if the service is already running. If it's not running, it's a no-op.
#
# TODO: This is only tested on Debian.
#
if [ "$1" = "configure" ] && [ -d /run/systemd/system ]; then
  systemctl --system daemon-reload >/dev/null || true
  if systemctl is-active -q ntfy.service; then
    echo "Restarting ntfy.service ..."
    if [ -x /usr/bin/deb-systemd-invoke ]; then
      deb-systemd-invoke try-restart ntfy.service >/dev/null || true
    else
      systemctl restart ntfy.service >/dev/null || true
    fi
  fi
fi
