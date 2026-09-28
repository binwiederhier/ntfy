# Leads Meta Ads → notification ntfy (via Make, piloté par Claude)

Chaque fois qu'un prospect remplit un **formulaire instantané Meta** (Facebook / Instagram Lead Ads),
une notification arrive sur ton téléphone dans l'app **ntfy**, en ~2 secondes, avec toutes ses réponses
et deux boutons : **Appeler** et **Email**.

```
Prospect remplit le formulaire Meta
   → Make (déclencheur « Facebook Lead Ads — New Lead », instantané)
   → Make (module HTTP) POST https://ntfy.sh/<ton-topic>
   → notification dans l'app ntfy
```

Pas d'API Facebook à configurer, pas de bot, pas de serveur, pas de code à héberger.
**Claude monte tout le scénario à ta place** via le MCP de Make. Toi, tu fais seulement
deux choses : te connecter à Facebook une fois dans Make, et t'abonner au topic dans l'app ntfy.

---

## Ce qu'il te faut

| Élément | Où | Coût |
|---|---|---|
| Un compte **Make** | [make.com](https://www.make.com) → *Get started free* | gratuit (1 000 opérations/mois ≈ 500 leads) |
| **Claude** avec le connecteur **Make** | claude.ai → *Paramètres → Connecteurs* → ajouter **Make** et se connecter | inclus |
| **Claude Code** (terminal ou app desktop) | [claude.com/claude-code](https://claude.com/claude-code), connecté avec le **même compte claude.ai** | inclus |
| Être **admin de la Page Facebook** qui porte le formulaire | Meta Business Suite | — |
| Un **formulaire instantané** publié sur cette Page | Ads Manager / Business Suite | — |
| L'app **ntfy** sur le téléphone | [Android](https://play.google.com/store/apps/details?id=io.heckel.ntfy) · [iOS](https://apps.apple.com/us/app/ntfy/id1625396347) | gratuit |

> Les connecteurs ajoutés sur claude.ai sont automatiquement disponibles dans Claude Code
> quand tu es connecté avec le même compte. Vérifie avec la commande `/mcp` : tu dois voir **Make**.

---

## Utilisation (5 minutes)

```bash
git clone -b projet-samuel-ntfy https://github.com/samuelpropro/ntfy.git
cd ntfy/projet-samuel-ntfy
claude
```

Puis dis simplement à Claude :

> **Branche les leads de ma campagne Meta sur ntfy.**

Claude lit `CLAUDE.md` (le mode opératoire de ce dossier) et va :

1. vérifier que Make est bien connecté et trouver ton équipe Make ;
2. vérifier ta connexion Facebook dans Make — s'il n'y en a pas, il te donne **un lien** :
   tu cliques, tu te connectes à Facebook, tu acceptes les autorisations (Claude ne voit jamais ton mot de passe) ;
3. te demander **le nom de la Page** et te lister ses formulaires pour que tu choisisses **celui de la campagne** ;
4. lire lui-même les questions du formulaire (nom, email, téléphone, questions personnalisées) ;
5. générer un **topic ntfy** secret ;
6. créer le webhook Facebook et le scénario Make, puis l'activer ;
7. t'envoyer une notification de test et te guider pour le test avec un vrai lead.

À la fin, il te donne le nom du topic à ajouter dans l'app ntfy.

## S'abonner dans l'app ntfy

1. Ouvre ntfy → bouton **+**
2. **Topic** : celui que Claude t'a donné (ex. `acme-leads-7k2m9x4q1pzt`)
3. Serveur : laisse `ntfy.sh`
4. **Subscribe**

Pour partager les leads avec un client ou un collègue : il s'abonne au **même topic**.

---

## À savoir avant de l'utiliser pour un client

- **Le topic est le mot de passe.** Sur `ntfy.sh`, n'importe qui qui connaît le nom du topic peut lire
  les notifications. Claude génère un nom aléatoire impossible à deviner — ne le publie nulle part
  (ni dans ce repo, ni dans un canal public). Pour verrouiller complètement : compte ntfy Pro
  (réservation du topic) ou serveur ntfy auto-hébergé.
- **Un scénario par formulaire.** Chaque formulaire a ses propres noms de champs. Nouveau formulaire
  = redemander à Claude, il refera le montage.
- **Un formulaire Meta publié n'est plus modifiable.** Si tu le changes, Meta en crée un nouveau :
  relance Claude pour rebrancher.
- **La connexion Facebook de Make expire** (~60 jours). Si les notifications s'arrêtent :
  Make → *Connections* → **Reauthorize**. Voir [docs/depannage.md](docs/depannage.md).
- **Coût Make** : 2 opérations par lead.

## Contenu du dossier

| Fichier | Rôle |
|---|---|
| `CLAUDE.md` | le mode opératoire que Claude suit, étape par étape |
| `templates/scenario-blueprint.json` | le modèle du scénario Make (déclencheur Facebook + HTTP ntfy) |
| `scripts/nouveau-topic.sh` | génère un nom de topic ntfy aléatoire |
| `scripts/test-ntfy.sh` | envoie une notification de test sur un topic |
| `docs/depannage.md` | pannes connues et leurs solutions |
| `docs/pourquoi-ce-montage.md` | les choix techniques, pour les curieux |
