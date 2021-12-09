#!/bin/sh
set -e

# Delete the config if package is purged
if [ "$1" = "purge" ]; then
  id ntfy >/dev/null 2>&1 && userdel ntfy
  rm -f /etc/ntfy/config.yml
  rmdir /etc/ntfy || true
fi

