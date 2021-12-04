#!/bin/sh
set -e

# Delete the config if package is purged
if [ "$1" = "purge" ]; then
  echo "Deleting /etc/ntfy ..."
  rm -rf /etc/ntfy || true
fi
