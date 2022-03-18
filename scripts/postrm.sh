#!/bin/sh
set -e

# Delete the config if package is purged
if [ "$1" = "purge" ] || [ "$1" = "0" ]; then
  id ntfy >/dev/null 2>&1 && userdel ntfy
  rm -f /etc/ntfy/server.yml /etc/ntfy/client.yml
  rmdir /etc/ntfy || true
fi

