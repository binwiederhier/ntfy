# Pourquoi ce montage

## Pourquoi Make plutôt que l'API Facebook

Recevoir les leads en direct via l'API Meta demande : une app Meta validée, un endpoint public
hébergé, le handshake du webhook `leadgen`, l'abonnement de chaque Page à l'app, et un token
System User. C'est des heures de configuration par client.

Make a déjà une app Meta validée. Une connexion OAuth (un clic) suffit, et le déclencheur
**Facebook Lead Ads → New Lead** est **instantané** (~2 s). Le MCP de Make permet en plus à Claude
de tout lire et tout créer sans passer par l'interface :

| Besoin | Outil MCP Make | Remplace |
|---|---|---|
| lister les Pages | `rpc_execute` → `facebook-lead-ads@2/extendedPagesRPC` | `GET /me/accounts` |
| lister les formulaires | `rpc_execute` → `facebook-lead-ads@2/Forms` | `GET /{page}/leadgen_forms` |
| lire les clés des champs | `rpc_execute` → `facebook-lead-ads@2/LeadInterface` | `GET /{form}?fields=questions` |
| brancher le webhook | `hooks_create` (`facebook-lead-ads-new-event`) | app Meta + endpoint + abonnement |
| connexion Facebook | `credential-requests_create` (lien OAuth) | token System User |

## Pourquoi ntfy plutôt que Telegram / Slack

- **Rien à créer** : pas de bot, pas de token, pas de workspace. Un nom de topic suffit.
- **Boutons d'action natifs** : *Appeler* (`tel:`) et *Email* (`mailto:`) directement dans la notification.
- **Priorité** : `urgent` fait sonner même en « ne pas déranger » sur Android.
- **Partage** : client et collègues s'abonnent au même topic.
- Open source (ce repo est un fork de [binwiederhier/ntfy](https://github.com/binwiederhier/ntfy)) :
  on peut auto-héberger le serveur si un client l'exige.

## Pourquoi un corps en texte brut (et pas du JSON)

ntfy accepte aussi une publication JSON (`POST https://ntfy.sh/` avec `{"topic":…,"message":…}`).
Mais si un prospect tape un guillemet `"` ou un retour à la ligne dans une réponse, le JSON généré
par Make devient invalide et la notification est perdue. En **texte brut** (`text/plain`) vers
`https://ntfy.sh/<topic>`, avec titre / priorité / boutons dans les **en-têtes**, rien ne peut casser.

Référence : `docs/publish.md` de ce repo (sections *Message title*, *Message priority*,
*Tags & emojis*, *Action buttons → Open website/app*, *Limitations*).

## Limites ntfy.sh à connaître

| Limite | Valeur sur ntfy.sh |
|---|---|
| Taille d'un message | 4 096 octets (largement assez pour un lead) |
| Messages par jour | 250 par visiteur, compté par IP. Les requêtes partent des serveurs de Make, dont les IP sont partagées : si des erreurs `429` apparaissent dans l'historique Make, passer par un compte ntfy (en-tête `Authorization: Bearer <token>`) ou un serveur ntfy perso |
| Titre | 1 Ko |
| Confidentialité | topic public : quiconque connaît le nom lit les messages |
