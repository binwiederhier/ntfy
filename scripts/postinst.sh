#!/bin/sh
set -e

# Restart systemd service if it was already running. Note that "deb-systemd-invoke try-restart" will
# only act if the service is already running. If it's not running, it's a no-op.
#
# TODO: This is only tested on Debian.
#
if [ "$1" = "configure" ] && [ -d /run/systemd/system ]; then
  # Create ntfy user/group
  id ntfy >/dev/null 2>&1 || useradd --system --no-create-home ntfy
  chown ntfy.ntfy /var/cache/ntfy
  chmod 700 /var/cache/ntfy

  # Hack to change permissions on cache file
  configfile="/etc/ntfy/server.yml"
  if [ -f "$configfile" ]; then
    cachefile="$(cat "$configfile" | perl -n -e'/^\s*cache-file: ["'"'"']?([^"'"'"']+)["'"'"']?/ && print $1')" # Oh my, see #47
    if [ -n "$cachefile" ]; then
      chown ntfy.ntfy "$cachefile" || true
      chmod 600 "$cachefile" || true
    fi
  fi

  # Restart service
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
