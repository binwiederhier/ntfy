#!/bin/sh
set -e

# Restart systemd service if it was already running. Note that "deb-systemd-invoke try-restart" will
# only act if the service is already running. If it's not running, it's a no-op.
#
if [ "$1" = "configure" ] || [ "$1" -ge 1 ]; then
  if [ -d /run/systemd/system ]; then
    # Create ntfy user/group
    groupadd -f ntfy
    id ntfy >/dev/null 2>&1 || useradd --system --no-create-home -g ntfy ntfy
    chown ntfy:ntfy /var/cache/ntfy /var/cache/ntfy/attachments /var/lib/ntfy
    chmod 700 /var/cache/ntfy /var/cache/ntfy/attachments /var/lib/ntfy

    # Hack to change permissions on cache file
    configfile="/etc/ntfy/server.yml"
    if [ -f "$configfile" ]; then
      cachefile="$(cat "$configfile" | perl -n -e'/^\s*cache-file: ["'"'"']?([^"'"'"']+)["'"'"']?/ && print $1')" # Oh my, see #47
      if [ -n "$cachefile" ]; then
        chown ntfy:ntfy "$cachefile" || true
        chmod 600 "$cachefile" || true
      fi
    fi

    # Restart services
    systemctl --system daemon-reload >/dev/null || true
    if systemctl is-active -q ntfy.service; then
      echo "Restarting ntfy.service ..."
      if [ -x /usr/bin/deb-systemd-invoke ]; then
        deb-systemd-invoke try-restart ntfy.service >/dev/null || true
      else
        systemctl restart ntfy.service >/dev/null || true
      fi
    fi
    if systemctl is-active -q ntfy-client.service; then
      echo "Restarting ntfy-client.service (system) ..."
      if [ -x /usr/bin/deb-systemd-invoke ]; then
        deb-systemd-invoke try-restart ntfy-client.service >/dev/null || true
      else
        systemctl restart ntfy-client.service >/dev/null || true
      fi
    fi
    
    # inform user about systemd user service
    echo
    echo "------------------------------------------------------------------------"
    echo "ntfy includes a systemd user service."
    echo "To enable it, run following commands as your regular user (not as root):"
    echo
    echo "  systemctl --user enable ntfy-client.service"
    echo "  systemctl --user start ntfy-client.service"
    echo "------------------------------------------------------------------------"
    echo
  fi
fi
