#!/usr/bin/env bash
# Génère un nom de topic ntfy difficile à deviner : <client>-leads-<12 caractères aléatoires>
# Usage : bash scripts/nouveau-topic.sh "Acme Fenêtres"
set -eu
client="${1:-client}"
# Accents -> lettres simples (iconv de macOS écrit « ê » en « ^e » : on retire ces marques)
ascii=$(printf '%s' "$client" | iconv -f UTF-8 -t ASCII//TRANSLIT 2>/dev/null | tr -d "^'\"~\`" || true)
slug=$(printf '%s' "$ascii" | tr '[:upper:]' '[:lower:]' | tr -cs 'a-z0-9' '-' | sed 's/^-*//; s/-*$//' | cut -c1-30)
suffix=$(LC_ALL=C tr -dc 'a-z0-9' </dev/urandom 2>/dev/null | head -c 12 || true)
echo "${slug:-client}-leads-${suffix}"
