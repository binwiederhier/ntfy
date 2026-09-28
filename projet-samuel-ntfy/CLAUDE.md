# Mode opératoire — Leads Meta Ads → ntfy via Make

Tu es dans le projet **projet-samuel-ntfy**. Quand l'utilisateur te demande de brancher les leads
d'une campagne Meta (Facebook/Instagram Lead Ads) sur ntfy, suis **exactement** les étapes ci-dessous.
Tout passe par le **MCP de Make** (outils `mcp__*Make__*`). **N'utilise jamais l'API Graph de Facebook**,
ni curl vers graph.facebook.com : tout ce dont tu as besoin (Pages, formulaires, champs) se lit
à travers la connexion Facebook de Make.

Parle à l'utilisateur en français, simplement : il n'est pas forcément technique.

---

## Règles non négociables

1. **Tu ne saisis jamais de mot de passe ni de token.** La connexion Facebook se fait par l'utilisateur,
   via le lien que tu lui donnes (étape 2).
2. **Tu ne modifies ni ne supprimes jamais un scénario, un webhook ou une connexion existants**
   sans demande explicite. Tu **crées** un nouveau scénario.
3. **Un scénario par formulaire**, formulaire **verrouillé** dans le webhook (jamais `formId` vide).
4. **Le statut « Success » de Make ne prouve rien.** Tu valides en relisant le contenu réellement
   reçu par ntfy (étape 8).
5. **Ne jamais écrire le nom du topic, les IDs Make ou des données de lead dans un fichier du repo.**
   Le repo est public ; le topic est un mot de passe.
6. Tu ne peux pas créer de compte Make à la place de l'utilisateur (inscription + vérification email).
   S'il n'en a pas : lui dire d'en créer un gratuit sur https://www.make.com, puis d'ajouter le connecteur
   Make dans claude.ai → Paramètres → Connecteurs, puis de relancer Claude Code.

---

## Étape 0 — Pré-vol

- Vérifie que les outils Make sont disponibles (ex. `environment_get`). S'ils sont absents ou différés,
  charge-les via ToolSearch (`select:` avec : `environment_get`, `connections_list`,
  `credential-requests_create`, `credential-requests_get`, `rpc_execute`, `hook-config_get`, `hooks_create`,
  `validate_module_configuration`, `validate_blueprint_schema`, `scenarios_create`, `scenarios_activate`,
  `scenarios_get`, `executions_list`, `executions_get-detail`).
- Si aucun outil Make n'existe : arrête-toi et explique le point 6 des règles.
- `environment_get` → note `zone` (ex. `eu1.make.com`), `organizationId` et `teamId`. Prends l'équipe de
  type `standard` ; s'il y en a plusieurs, demande laquelle (les `private` sont des espaces perso).

## Étape 1 — Les questions à poser (en une seule fois)

Pose ces questions groupées, avec `AskUserQuestion` si disponible :

1. **Nom du client / de la campagne** (sert à nommer le scénario et le topic). Ex. « Acme Fenêtres ».
2. **Nom de la Page Facebook** qui diffuse la publicité (ou un mot-clé du nom).
3. **Le compte Facebook utilisé est-il admin de cette Page ?** (sinon la Page n'apparaîtra pas).
4. **Topic ntfy** : en générer un nouveau (recommandé) ou réutiliser un existant ?
5. **Priorité** de la notification : `high` (son + vibration, recommandé) ou `urgent`
   (sonnerie longue, passe outre le « ne pas déranger » sur Android).

Le **formulaire** se choisit à l'étape 3, sur liste.

## Étape 2 — Connexion Facebook dans Make

```
connections_list(teamId, type=["facebook"])
```

- **Une ou plusieurs connexions** → prends la plus récente dont `expire` est dans le futur ;
  s'il y en a plusieurs, montre `name` + `metadata.value` (nom du compte FB) et demande laquelle.
- **Aucune, ou toutes expirées** → crée une demande d'autorisation :
  ```
  credential-requests_create(teamId, name="Connexion Facebook — leads ntfy",
    credentials=[{appName:"facebook-lead-ads", appModules:["NewLeadMultiple"],
                  description:"Autoriser Make à recevoir les leads de vos formulaires Meta"}])
  ```
  La réponse contient `publicUri` : donne **ce lien** à l'utilisateur : « Clique, connecte-toi avec le compte Facebook admin de la Page,
  accepte toutes les autorisations, puis reviens me dire “c'est fait” ». Ensuite relance
  `connections_list` pour récupérer l'ID de la nouvelle connexion.

Note l'ID : `CONN_ID`.

## Étape 3 — Trouver la Page et le formulaire (sans API Facebook)

**Page** :
```
rpc_execute(appName="facebook-lead-ads", appVersion=2, rpcName="extendedPagesRPC",
            data={"__IMTCONN__": CONN_ID, "query": "<mot-clé du nom de la Page>"})
```
→ liste de `{label, value}` ; `value` = `PAGE_ID`.
- Résultat vide : essaie un mot-clé plus court. Toujours vide → le compte FB n'est pas admin de la Page,
  ou une **Page homonyme** existe (voir `docs/depannage.md`). Deux résultats au même nom → demande laquelle.

**Formulaire** :
```
rpc_execute(appName="facebook-lead-ads", appVersion=2, rpcName="Forms",
            data={"__IMTCONN__": CONN_ID, "pageId": PAGE_ID})
```
→ montre la liste à l'utilisateur et demande **le formulaire utilisé par la campagne**
(indice : les copies s'appellent `…-copy`). `value` = `FORM_ID`.

## Étape 4 — Lire les champs du formulaire

```
rpc_execute(appName="facebook-lead-ads", appVersion=2, rpcName="LeadInterface",
            data={"__IMTCONN__": CONN_ID, "pageId": PAGE_ID, "formId": FORM_ID})
```
Dans la réponse, l'élément `name == "data"` a un `spec` : chaque entrée = un champ du formulaire
(`name` = clé technique, `label` = libellé lisible). **Ce sont ces clés qu'il faut mapper**, elles
varient selon le formulaire :

| Origine du formulaire | Clés typiques |
|---|---|
| créé en anglais / via API | `full_name`, `email`, `phone_number` |
| créé dans l'interface en français | `nom_complet`, `e-mail`, `numéro_de_téléphone` |
| question personnalisée | le libellé slugifié, ex. `combien_de_bornes_installez-vous_par_mois_aujourd'hui_?` |

Repère : `KEY_PHONE` (téléphone), `KEY_EMAIL` (email), `KEY_NAME` (nom). S'il en manque un, retire
le bouton ou la ligne correspondante.

**Syntaxe Make** : `{{1.data.<clé>}}`. Si la clé contient autre chose que lettres/chiffres/`_`
(tiret, accent, apostrophe, `?`, espace), entoure-la de backticks : ``{{1.data.`e-mail`}}``.

## Étape 5 — Topic ntfy

Nouveau topic : `bash scripts/nouveau-topic.sh "<nom-client>"` → ex. `acme-leads-7k2m9x4q1pzt`.
Règles ntfy : `[-_A-Za-z0-9]`, 64 caractères max. Note-le : `TOPIC`.

Envoie tout de suite un test pour que l'utilisateur s'abonne :
`bash scripts/test-ntfy.sh TOPIC` puis explique-lui : app ntfy → **+** → topic `TOPIC` → Subscribe.
Attends qu'il confirme avoir **reçu** la notification de test avant de continuer
(ça valide l'app, indépendamment de Make).

## Étape 6 — Créer le webhook Facebook

```
hooks_create(teamId, name="<Client> - Leads Meta vers ntfy",
             typeName="facebook-lead-ads-new-event",
             data={"__IMTCONN__": CONN_ID, "pageId": PAGE_ID, "formId": FORM_ID})
```
→ `HOOK_ID`. (Structure vérifiée via `hook-config_get("facebook-lead-ads-new-event")`.)

## Étape 7 — Créer et activer le scénario

1. Pars de `templates/scenario-blueprint.json`. Remplace :
   - `"__HOOK_ID__"` → le **nombre** `HOOK_ID` (pas une chaîne) ;
   - `__CLIENT__` → nom du client ;
   - `__TOPIC__` → `TOPIC` ;
   - `__PRIORITY__` → `high` ou `urgent` ;
   - `__ACTIONS__` → `view, Appeler, tel:{{1.data.KEY_PHONE}}; view, Email, mailto:{{1.data.KEY_EMAIL}}`
     (retire la partie correspondante si un champ manque ; s'il n'en reste aucun, supprime l'en-tête `Actions`) ;
   - `__BODY__` → une ligne par champ de l'étape 4, au format `<Libellé> : {{1.data.<clé>}}`,
     séparées par `\n`, puis `\n\nRappeler sous 24 h`.
     Libellés courts en français (« Nom », « Email », « Téléphone », puis la question raccourcie).
2. **Pas d'accent ni d'emoji dans le titre** (`Title`) : les en-têtes HTTP passent mal en UTF-8.
   Les accents sont OK dans le corps.
3. Valide le module HTTP :
   `validate_module_configuration(organizationId, teamId, appName="http", appVersion=3,
   moduleName="ActionSendData", parameters=<flow[1].parameters>, mapper=<flow[1].mapper>)`
   → doit renvoyer `valid: true`. (Le champ `followAllRedirects` est obligatoire : sans lui,
   le run échoue en `BundleValidationError`.)
4. `validate_blueprint_schema(blueprint)`.
5. `scenarios_create(teamId, blueprint, scheduling={"type":"immediately"})` → `SCENARIO_ID`.
6. `scenarios_activate(SCENARIO_ID)`.
7. `scenarios_get(SCENARIO_ID)` → vérifie `isActive: true`, `islinked: true`, `isinvalid: false`.

## Étape 8 — Test avec un vrai lead (l'utilisateur agit, toi tu vérifies)

Tu ne peux pas créer de lead de test toi-même. Guide l'utilisateur :

1. Ouvrir https://developers.facebook.com/tools/lead-ads-testing
2. Choisir la **Page**, puis le **formulaire** `FORM_ID`
3. S'il existe déjà un lead de test : **Supprimer le prospect**, puis **Créer un prospect**
4. Dire « c'est fait »

Puis vérifie **les deux côtés** :
- `executions_list(SCENARIO_ID)` → dernière exécution `status: 1`, `type: auto`, `instant: true` ;
- le contenu reçu par ntfy :
  `curl -s "https://ntfy.sh/TOPIC/json?poll=1&since=10m"`
  → lis le champ `message` : **chaque ligne doit contenir une valeur**, et aucune trace de `{{`.
  Un lead de test contient des valeurs factices (`<test lead: dummy data for full_name>`) : c'est normal.

Ligne vide ou `{{…}}` visible → une clé est fausse : relis l'étape 4, corrige le blueprint
(`scenarios_get` → modifier → `scenarios_update` avec le blueprint **complet**), reteste.

## Étape 9 — Récapitulatif à donner à l'utilisateur

- Nom du scénario Make + lien : `https://<zone>/<teamId>/scenarios/<SCENARIO_ID>`
  (`zone` tel que renvoyé par `environment_get`, ex. `eu1.make.com`).
- Le **topic** à ajouter dans l'app (et à partager avec qui doit recevoir les leads).
- Rappels : un formulaire = un scénario ; reconnexion Facebook dans ~60 jours
  (Make → Connections → Reauthorize) ; 2 opérations Make par lead ; faire un **vrai** lead depuis la pub
  sur mobile une fois les ads actives (accents, formats réels).

---

## Variante : ajouter ntfy à un scénario existant (ex. déjà vers Telegram)

Seulement si l'utilisateur le demande explicitement. `scenarios_get` → ajoute le module HTTP du
template **en dernier** dans `flow` (nouvel `id`), en réutilisant les **mêmes expressions de mapping**
que le module existant (elles sont déjà justes pour ce formulaire) → `scenarios_update` avec le
blueprint complet. Pour tester sans créer de nouveau lead : `scenarios_replay` d'une exécution
récente marquée `isReplayable: true` (préviens que le module existant renverra aussi le message).
