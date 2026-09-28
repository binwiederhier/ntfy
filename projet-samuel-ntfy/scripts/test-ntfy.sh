#!/usr/bin/env bash
# Envoie une notification de test (faux lead) sur un topic ntfy.
# Usage : bash scripts/test-ntfy.sh <topic> [serveur]
set -euo pipefail
topic="${1:?Usage : bash scripts/test-ntfy.sh <topic> [serveur]}"
server="${2:-https://ntfy.sh}"
curl -sf "$server/$topic" \
  -H "Title: Test - notification lead" \
  -H "Priority: high" \
  -H "Tags: rotating_light" \
  -H "Actions: view, Appeler, tel:+33600000000; view, Email, mailto:test@exemple.fr" \
  -d "Nom : Jean Test
Email : test@exemple.fr
Téléphone : +33600000000

Si tu vois ce message, l'app ntfy est bien abonnée." >/dev/null
echo "Notification de test envoyée sur $server/$topic"
