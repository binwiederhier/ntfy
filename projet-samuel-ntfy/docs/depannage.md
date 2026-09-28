# Dépannage

Pannes rencontrées en production, de la plus probable à la plus rare.

## Plus aucune notification n'arrive

1. **La connexion Facebook de Make a expiré** (~60 jours) ou a perdu ses permissions.
   Make → *Connections* → la connexion Facebook affiche **Reauthorize** → cliquer, se reconnecter.
   C'est la cause n°1. Aucune erreur n'est visible côté Meta.
2. **Le scénario a été désactivé.** Make coupe un scénario après plusieurs erreurs d'affilée
   (`maxErrors: 3`). Make → *Scenarios* → le réactiver, puis regarder *History* pour comprendre l'erreur.
3. **Quota Make épuisé** (plan Free : 1 000 opérations/mois, 2 par lead). Make → *Organization* → usage.
4. **Le téléphone ne reçoit plus**, alors que Make est vert : tester l'app seule avec
   `bash scripts/test-ntfy.sh <topic>`. Sur Android, désactiver l'optimisation de batterie pour ntfy.
   Sur iOS, vérifier que les notifications de l'app sont autorisées.

## La notification arrive, mais des lignes sont vides

Les **clés de champ** ne correspondent pas au formulaire. Causes typiques :
- la campagne utilise **un autre formulaire** que celui branché (ex. une copie `…-copy`) ;
- le formulaire a été créé en français : les clés sont `nom_complet`, `e-mail`, `numéro_de_téléphone`,
  pas `full_name`, `email`, `phone_number`.

→ Demande à Claude : « Les lignes sont vides, revérifie les champs du formulaire ». Il relit les
clés réelles (RPC `LeadInterface`) et corrige le scénario.

## Tu vois `{{1.data...}}` en clair dans la notification

Expression mal écrite (souvent une clé avec tiret/accent/apostrophe sans backticks).
Syntaxe correcte : ``{{1.data.`numéro_de_téléphone`}}``.

## La Page n'apparaît pas / « aucun formulaire »

- Le compte Facebook connecté à Make n'est **pas admin** de la Page → se faire ajouter admin,
  puis *Reauthorize* la connexion dans Make.
- **Deux Pages portent le même nom** (une dans le Business Manager, une dans le compte perso).
  Make peut voir la mauvaise. Vérification visuelle : https://developers.facebook.com/tools/lead-ads-testing
  → menu déroulant *Page* : si le nom apparaît deux fois, supprimer ou renommer le doublon, puis *Reauthorize*.
- Les **conditions Lead Ads** n'ont pas été acceptées sur la Page : l'outil de test Meta l'indique
  dans les diagnostics.

## Le run Make échoue en `BundleValidationError`

Il manque un champ obligatoire dans le module HTTP (vu en prod : `followAllRedirects`).
Le template de ce repo l'inclut déjà.

## Le titre de la notification affiche des `?`

Accents ou emojis dans l'en-tête `Title`. Les en-têtes HTTP passent mal en UTF-8 : garder le titre
sans accents. Les accents dans le corps du message ne posent aucun problème.

## Le lead de test Meta ne se crée pas

L'outil de test n'accepte qu'**un lead de test par formulaire**. Cliquer **Supprimer le prospect**
avant **Créer un prospect**. Un lead de test ne peut être supprimé que par le compte qui l'a créé.
