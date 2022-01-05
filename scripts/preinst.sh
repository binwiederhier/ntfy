#!/bin/sh
set -e

if [ "$1" = "install" ] || [ "$1" = "upgrade" ] || [ "$1" -gt 1 ]; then
  # Migration of old to new config file name
  oldconfigfile="/etc/ntfy/config.yml"
  configfile="/etc/ntfy/server.yml"
  if [ -f "$oldconfigfile" ] && [ ! -f "$configfile" ]; then
    mv "$oldconfigfile" "$configfile" || true
  fi
fi
