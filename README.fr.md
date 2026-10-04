# My API

Une passerelle d’API d’IA auto-hébergée pour vos comptes, modèles et applications.

[English](README.md) · [简体中文](README.zh_CN.md) · [繁體中文](README.zh_TW.md) · [Français](README.fr.md) · [日本語](README.ja.md)

My API réunit connexions amont, clés API clientes, permissions et suivi d’utilisation dans une console. L’usage personnel est prioritaire, avec partage contrôlé à une petite équipe. Les modules commerciaux sont désactivés sur les nouvelles installations : aucun rechargement d’un portefeuille interne n’est requis. Utilisateurs, permissions et limites restent actifs.

## Fonctions et limites

- Configurer des canaux et leurs modèles avec les adaptateurs existants : API compatibles OpenAI, Responses, Claude Messages, Gemini, Codex, entre autres. Les fonctionnalités dépendent de l’adaptateur et du compte amont.
- Attribuer une clé distincte à chaque application/utilisateur, avec modèles, profil d’accès et limites applicables. Conserver les identifiants amont sur le serveur.
- Consulter requêtes, erreurs, utilisation et observations de quota disponibles, sans confondre données manquantes, échecs et remise à zéro d’une fenêtre.
- Tester explicitement modèle, endpoint et mode streaming ; réutiliser les derniers paramètres ayant réussi lorsque cela s’applique.
- Choisir SQLite, MySQL ou PostgreSQL. La console est disponible en anglais, chinois simplifié/traditionnel, français, japonais, russe et vietnamien.

Les budgets stricts Token/USD concernent uniquement les chemins qualifiés Responses natifs officiels, en texte seul. L’USD exige aussi un tarif figé applicable et un niveau de service pris en charge. Alias, conversions, outils et multimodalité ne sont pas automatiquement éligibles. Le pourcentage Codex est un seuil de sécurité du compte/de sa fenêtre, pas un registre de consommation par clé. Le coût API équivalent d’un abonnement est indicatif, pas une facture réelle. Une utilisation inconnue ou estimée n’est pas un zéro constaté.

## Versions

État au 2026-10-04 :

- **Préversion publiée : `v0.2.0-beta.3`**, images Full et LAN historique pour Linux amd64/arm64. Le [registre de publication](docs/RELEASE_BETA_3.md) contient source, digests, signatures et limites de validation.
- **beta.4 : source de développement vérifiée, non publiée.** Découverte des modèles, correspondances explicites, aperçu du routage, distribution et journaux dans un périmètre limité ; absents des images beta.3.
- **beta.5 : candidat local uniquement.** Ordonnancement des comptes, refroidissement temporaire, basculement borné et explication des tentatives ne sont pas publiés. La vérification distante de ce candidat sur trois bases et Chromium reste à faire.

La préversion n’est pas une garantie d’aptitude à la production. OAuth réel, réinitialisation/429, rapprochement des factures et HTTPS du serveur cible restent partiellement ou non validés. La santé d’un conteneur ne valide pas un fournisseur réel. Les images par défaut restent beta.3, même depuis une branche de développement. L’installateur/mise à jour unifié Lite/Desktop n’est pas livré ; des artefacts desktop ne prouvent pas une validation sur appareil réel.

## Installer la préversion publiée

Parcours recommandé : **Full sur Linux, image Docker épinglée, SQLite, écoute locale derrière votre proxy HTTPS**.

Prérequis : Linux amd64/arm64, Git, Bash, Docker fonctionnel, Compose v2 prenant en charge `up --wait --wait-timeout`, accès au dépôt et à GitHub/GHCR, stockage persistant et origine HTTPS sous votre contrôle. Le proxy doit cibler `http://127.0.0.1:3000`. Les exemples de secrets nécessitent OpenSSL. Go, Bun, Node.js, Redis et serveur de base séparé ne sont pas nécessaires ici. Les limites de 2 CPU / 2 GiB sont des plafonds du conteneur, pas des minima matériels mesurés.

### 1. Récupérer les fichiers

Pour une installation neuve dans un nouveau dossier uniquement ; ne remplacez pas le `.env` d’une instance existante :

```bash
git clone --branch v0.2.0-beta.3 --single-branch https://github.com/ForceMind/MyAPI.git my-api
cd my-api
umask 077
cp deploy/.env.example deploy/.env
chmod 600 deploy/.env
```

### 2. Configurer

Modifiez `deploy/.env`. Remplacez l’origine d’exemple par votre origine HTTPS exacte, sans chemin API ; l’installateur refuse les domaines de remplacement `example.com` :

```dotenv
MYAPI_IMAGE=ghcr.io/forcemind/myapi:v0.2.0-beta.3
MYAPI_BUILD_LOCAL=false
MYAPI_EDITION=full
MYAPI_BIND_ADDRESS=127.0.0.1
MYAPI_ALLOW_LAN=false
MYAPI_SESSION_COOKIE_SECURE=true
MYAPI_PUBLIC_URL=https://api.example.com
FULL_CONTENT_LOG_ENABLED=false
```

Pour une nouvelle instance, générez un `SESSION_SECRET` et copiez le résultat dans votre éditeur privé ; au moins 48 caractères sont requis :

```bash
openssl rand -hex 32
```

Pour l’échantillonnage de quota multi-comptes, générez un secret indépendant et affectez `active:v1:` suivi du résultat à `CHANNEL_QUOTA_IDENTITY_KEYS` :

```bash
openssl rand -base64 32 | tr '+/' '-_' | tr -d '=\n'
```

Un trousseau vide désactive l’échantillonnage dépendant de l’identité. Conservez le trousseau complet avec les sauvegardes ; partagez-le entre instances utilisant la même base. Ne régénérez pas les secrets lors d’une mise à niveau et ne les publiez jamais dans Git ou un signalement. `.env` contient des valeurs littérales `KEY=VALUE`, sans substitution shell. Les chemins par défaut sont `deploy/data/` et `deploy/logs/`, montés dans `/data` et `/app/logs`.

### 3. Démarrer

```bash
bash deploy/install.sh
```

Le script valide la configuration, récupère l’image, démarre et attend au plus 120 secondes la santé du conteneur. Il n’installe pas Docker, les certificats, le proxy ou les règles de pare-feu. Ouvrez votre origine HTTPS, initialisez le compte administrateur et vérifiez connexion, version et revision dans les informations système. Full utilise des cookies Secure : HTTP localhost n’est pas l’adresse de connexion recommandée.

Pour un usage local/privé, consultez le [guide LAN historique](docs/LAN_LITE.md) et utilisez `ghcr.io/forcemind/myapi-lan:v0.2.0-beta.3`. Le partage LAN doit être activé explicitement.

## Première requête

1. Dans Channels (`/channels`), choisissez fournisseur, endpoint et identifiants autorisés. Dans un conteneur, Codex utilise le parcours de connexion web ; les fichiers de connexion de l’hôte ne sont pas accessibles.
2. Récupérez les modèles si possible ou saisissez l’ID exact. Vérifiez modèles activés, groupe/profil et correspondances. La découverte n’accorde aucune permission et ne prouve pas le fonctionnement. Le parcours beta.4 amélioré exige sa source de développement.
3. Testez une petite requête avec modèle, endpoint et streaming explicites. Le test contacte l’amont et peut consommer du quota ou être facturé ; utilisez un contenu non sensible.
4. Créez une clé cliente limitée aux besoins de l’application, jamais une copie de la clé amont ou du jeton administrateur.
5. Pour un client compatible OpenAI, utilisez votre origine HTTPS suivie de `/v1`, la clé cliente et le nom public activé. Les autres protocoles utilisent leurs endpoints documentés. Commencez par une courte requête.
6. Consultez Usage Logs (`/usage-logs/common`) : statut, modèle, preuves d’utilisation et coûts applicables. Les builds de développement avec preuves de routage permettent aux administrateurs de vérifier la cible ; un aperçu ne garantit ni le prochain choix aléatoire ni l’admission d’une clé.

En cas d’échec, vérifiez endpoint, modèle, identifiants, permissions et quota avant d’augmenter les tentatives. Conservez les états inconnus jusqu’à vérification fiable.

## Maintenance et restauration

Le [modèle de déploiement](deploy/.env.example) et les [variables du service](.env.example) sont deux couches distinctes : une variable ajoutée à `.env` n’est pas automatiquement transmise au conteneur. Pour une base externe ou Redis, vérifiez [Compose](deploy/docker-compose.yml). SQLite est le choix par défaut. MySQL ≥ 5.7.8 et PostgreSQL ≥ 9.6 sont des bases de compatibilité ; utilisez une version entretenue appropriée et testez les migrations. Redis est facultatif ; les sessions et limites multi-nœuds exigent une configuration spécifique.

Le CLI source demande Node.js ≥ 20. Ce guide utilise le CLI du dépôt récupéré et ne dépend pas de l’installation d’un paquet depuis NPM :

```bash
node cli/myapi.mjs help
node cli/myapi.mjs doctor --project-dir .
node cli/myapi.mjs status --project-dir .
node cli/myapi.mjs logs --project-dir .
```

`doctor` ne valide pas les fournisseurs réels ; les journaux peuvent être sensibles. `install`, `switch` et `rollback` ne sont pas des commandes disponibles.

Avant toute mise à niveau, relevez version/digest, revision, chemins et configuration. Sauvegardez la base de façon cohérente (y compris le WAL SQLite si applicable), les secrets, le trousseau complet et les journaux nécessaires. Vérifiez la restauration sur une copie isolée, puis testez la version publiée cible. Exemple de précontrôle en lecture seule pour passer une ancienne instance à beta.3 :

```bash
node cli/myapi.mjs upgrade --project-dir ./upgrade-copy --version v0.2.0-beta.3 --dry-run --json
```

Suivez le [guide de répétition et restauration](docs/UPGRADE_REHEARSAL.md). La sauvegarde `.env` du CLI n’est pas une sauvegarde de base. Après une tentative de démarrage cible, le CLI ne redémarre pas automatiquement l’ancienne image : la base a pu migrer. Restaurez une sauvegarde vérifiée antérieure avant un binaire incompatible. Ne supprimez pas les volumes ni les écritures non résolues pour revenir en arrière ; évitez `latest` et les versions seulement planifiées.

## Sécurité et documentation

Gardez l’écoute locale et vérifiez HTTPS, proxys de confiance, inscriptions, rôles et clés avant partage. Utilisez uniquement les comptes/API autorisés, en respectant conditions amont et lois applicables ; les services publics peuvent imposer des obligations supplémentaires. Le modèle livré active les journaux de contenu complet, désactivés dans l’exemple ci-dessus. Avant activation, définissez droits, rétention et sauvegardes : le masquage ne garantit pas l’absence de données privées dans les prompts/réponses. L’échantillonnage peut contacter l’amont en arrière-plan. Excluez secrets, fichiers OAuth, bases et journaux privés de Git et des signalements.

- [Publication et preuves](docs/RELEASE_BETA_3.md) · [Déploiement](DEPLOYMENT_CUSTOM.md) · [LAN](docs/LAN_LITE.md)
- [Restauration](docs/UPGRADE_REHEARSAL.md) · [Validation d’installation](docs/R1_INSTALLATION_CHECK.md)
- [API relay](docs/openapi/relay.json) · [API de gestion](docs/openapi/api.json)
- [Analyse des quotas](docs/QUOTA_ANALYTICS.md) · [Usage organisationnel Claude](docs/CLAUDE_USAGE_REPORT.md)
- [Authentification](docs/authentication.md) · [Journaux de contenu](docs/FULL_CONTENT_LOGGING_CUSTOM.md)
- [Signaler un problème](https://github.com/ForceMind/MyAPI/issues) : version/revision, déploiement et reproduction expurgée

Contributeurs : [plan et comptes rendus de développement](docs/MYAPI_MASTER_PLAN.md). Certains guides détaillés sont en chinois.

## Licence et crédits

My API est une distribution modifiée de travaux open source amont, avec des modifications de distribution par ForceMind. Licence [GNU AGPLv3](LICENSE) ; [NOTICE](NOTICE) précise les auteurs amont et les conditions additionnelles de l’article 7, notamment le crédit frontend visible et le lien vers le projet original dans les interfaces modifiées. Préservez les mentions et identifiez les modifications ; le changement de marque n’annule pas ces obligations. L’usage réseau d’une version modifiée peut aussi imposer la fourniture du code source correspondant.

Les [licences tierces](THIRD-PARTY-LICENSES.md) recensent les dépendances. Conservez les mentions applicables avec images, binaires, bundles frontend et packages desktop, dont celles d’Electron/Chromium le cas échéant. Consultez les conditions complètes avant utilisation ou redistribution.
