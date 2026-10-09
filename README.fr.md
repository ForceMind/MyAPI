<p align="center">
  <img src="web/public/myapi-logo-v1.png" alt="MyAPI logo" width="144" />
</p>

# MyAPI

Une passerelle d’API d’IA auto-hébergée pour les services de modèles, l’accès des applications et le suivi d’utilisation.

[English](README.md) · [简体中文](README.zh_CN.md) · [繁體中文](README.zh_TW.md) · [Français](README.fr.md) · [日本語](README.ja.md)

MyAPI réunit connexions aux services de modèles, clés API clientes, permissions et suivi d’utilisation dans une console. L’usage personnel est prioritaire, avec partage contrôlé à une petite équipe. Les modules commerciaux sont désactivés sur les nouvelles installations : aucun rechargement d’un portefeuille interne n’est requis. Utilisateurs, permissions et limites restent actifs.


La désactivation commerciale ne modifie pas automatiquement les utilisateurs existants : leur politique sans portefeuille doit être choisie explicitement. Le premier Root d’une installation neuve l’active. Elle couvre actuellement les requêtes POST Chat/Responses/Responses compact éligibles sans query ; limites des clés et suivi restent actifs.

## Technologies

![Go](https://img.shields.io/badge/Go-00ADD8?style=flat-square&logo=go&logoColor=white)
![Gin](https://img.shields.io/badge/Gin-008ECF?style=flat-square)
![GORM](https://img.shields.io/badge/GORM-607D8B?style=flat-square)
![React 19](https://img.shields.io/badge/React-19-149ECA?style=flat-square&logo=react&logoColor=white)
![TypeScript](https://img.shields.io/badge/TypeScript-3178C6?style=flat-square&logo=typescript&logoColor=white)
![Rsbuild](https://img.shields.io/badge/Rsbuild-FF6B35?style=flat-square)

![Tailwind CSS 4](https://img.shields.io/badge/Tailwind_CSS-4-06B6D4?style=flat-square&logo=tailwindcss&logoColor=white)
![Base UI](https://img.shields.io/badge/Base_UI-111827?style=flat-square)
![Bun](https://img.shields.io/badge/Bun-14151A?style=flat-square&logo=bun&logoColor=white)
![SQLite](https://img.shields.io/badge/SQLite-003B57?style=flat-square&logo=sqlite&logoColor=white)
![MySQL](https://img.shields.io/badge/MySQL-4479A1?style=flat-square&logo=mysql&logoColor=white)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-4169E1?style=flat-square&logo=postgresql&logoColor=white)
![Redis](https://img.shields.io/badge/Redis-DC382D?style=flat-square&logo=redis&logoColor=white)
![Docker Compose](https://img.shields.io/badge/Docker-Compose-2496ED?style=flat-square&logo=docker&logoColor=white)

- **Serveur :** Go (`go.mod` : 1.25.1), routage HTTP avec Gin et accès aux données avec GORM
- **Console web :** React 19, TypeScript, Rsbuild, Tailwind CSS 4 et Base UI ; Bun gère les dépendances et scripts frontend
- **Stockage :** SQLite par défaut, ou MySQL/PostgreSQL ; Redis est facultatif pour le cache partagé et les limites de débit
- **Déploiement :** images Docker et Docker Compose ; l’installation par image ne nécessite pas de chaîne de compilation Go/frontend locale

### Parcours d’une requête

```text
Application + clé API → MyAPI : authentification et permissions
                     → Sélection modèle/canal → Service de modèles
                     ← Réponse / streaming    ←
                       Utilisation et erreurs
```

La console configure canaux, modèles, utilisateurs et clés. Le serveur Go vérifie les permissions, choisit un canal admissible, appelle le service configuré et enregistre les preuves d’utilisation disponibles. Les identifiants du service restent sur le serveur ; les limites de protocole et de budget décrites ci-dessous s’appliquent.

Les versions reflètent les manifestes du dépôt, sans garantie pour toutes les versions futures. Voir [dépendances serveur](go.mod), [dépendances frontend](web/package.json) et [construction du conteneur](Dockerfile).

## Fonctions et limites

- Configurer des canaux et leurs modèles avec les adaptateurs existants : API compatibles OpenAI, Responses, Claude Messages, Gemini, Codex, entre autres. Les fonctionnalités dépendent de l’adaptateur et du compte du fournisseur.
- Attribuer une clé distincte à chaque application/utilisateur, avec modèles, profil d’accès et limites applicables. Conserver les identifiants du fournisseur sur le serveur.
- Consulter requêtes, erreurs, utilisation et observations de quota disponibles, sans confondre données manquantes, échecs et remise à zéro d’une fenêtre.
- Tester explicitement modèle, endpoint et mode streaming ; réutiliser les derniers paramètres ayant réussi lorsque cela s’applique.
- Choisir SQLite, MySQL ou PostgreSQL. La console est disponible en anglais, chinois simplifié/traditionnel, français, japonais, russe et vietnamien.

Les budgets stricts Token/USD couvrent les chemins Responses natifs officiels qualifiés en texte seul et le Chat natif du modèle exact `gpt-6.1-sol`. Chat réserve prudemment la borne de contexte de 1 050 000 tokens ; un petit appel peut être refusé si le budget restant ne couvre pas cette borne. L’USD exige aussi un tarif figé applicable et un niveau de service pris en charge. Alias, conversions, outils et multimodalité ne sont pas automatiquement éligibles. Le pourcentage Codex est un seuil de sécurité du compte/de sa fenêtre, pas un registre de consommation par clé. Le coût API équivalent d’un abonnement est indicatif, pas une facture réelle. Une utilisation inconnue ou estimée n’est pas un zéro constaté.

## Versions

Au 2026-10-09, **[v0.2.0-beta.9 est publiée](https://github.com/ForceMind/MyAPI/releases/tag/v0.2.0-beta.9)** avec les images Full/LAN amd64/arm64. Elle inclut les fonctions bornées de routage/budget/politique beta.4–7, l’interface beta.8 et le chat beta.9 avec clé explicite et images/PDF limités. Voir les [preuves de publication](docs/RELEASE_BETA_9.md).

La mise à niveau Full ciblée, la restauration isolée et HTTPS ont été vérifiés ; OAuth réel, fenêtres/429 et factures restent des validations distinctes et limitées. Cette version reste une préversion. L’installation/mise à jour unifiée Lite/Desktop n’est pas livrée. Prochaine étape : [usage personnel et limites des clés](docs/PERSONAL_CORE_DELIVERY_CARD.md), puis simplification structurelle bornée.

## Installer la préversion publiée

Parcours recommandé : **Full sur Linux, image Docker épinglée, SQLite, écoute locale derrière votre proxy HTTPS**.

Prérequis : Linux amd64/arm64, Git, Bash, Docker fonctionnel, Compose v2 prenant en charge `up --wait --wait-timeout`, accès au dépôt et à GitHub/GHCR, stockage persistant et origine HTTPS sous votre contrôle. Le proxy doit cibler `http://127.0.0.1:3000`. Les exemples de secrets nécessitent OpenSSL. Go, Bun, Node.js, Redis et serveur de base séparé ne sont pas nécessaires ici. Les limites de 2 CPU / 2 GiB sont des plafonds du conteneur, pas des minima matériels mesurés.

### 1. Récupérer les fichiers

Pour une installation neuve dans un nouveau dossier uniquement ; ne remplacez pas le `.env` d’une instance existante :

```bash
git clone --branch v0.2.0-beta.9 --single-branch https://github.com/ForceMind/MyAPI.git my-api
cd my-api
umask 077
cp deploy/.env.example deploy/.env
chmod 600 deploy/.env
```

### 2. Configurer

Modifiez `deploy/.env`. Remplacez l’origine d’exemple par votre origine HTTPS exacte, sans chemin API ; l’installateur refuse les domaines de remplacement `example.com` :

```dotenv
MYAPI_IMAGE=ghcr.io/forcemind/myapi:v0.2.0-beta.9
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

Pour un usage local/privé, consultez le [guide LAN historique](docs/LAN_LITE.md) et utilisez `ghcr.io/forcemind/myapi-lan:v0.2.0-beta.9`. Le partage LAN doit être activé explicitement.

## Première requête

1. Dans Channels (`/channels`), choisissez fournisseur, endpoint et identifiants autorisés. Dans un conteneur, Codex utilise le parcours de connexion web ; les fichiers de connexion de l’hôte ne sont pas accessibles.
2. Récupérez les modèles si possible ou saisissez l’ID exact. Vérifiez modèles activés, groupe/profil et correspondances. La découverte n’accorde aucune permission et ne prouve pas le fonctionnement. Le parcours beta.4 amélioré exige sa source de développement.
3. Testez une petite requête avec modèle, endpoint et streaming explicites. Le test contacte le service de modèles et peut consommer du quota ou être facturé ; utilisez un contenu non sensible.
4. Créez une clé cliente limitée aux besoins de l’application, jamais une copie de la clé du fournisseur ou du jeton administrateur.
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

Avant toute mise à niveau, relevez version/digest, revision, chemins et configuration. Sauvegardez la base de façon cohérente (y compris le WAL SQLite si applicable), les secrets, le trousseau complet et les journaux nécessaires. Vérifiez la restauration sur une copie isolée, puis testez la version publiée cible. Exemple de précontrôle en lecture seule pour passer une ancienne instance à beta.9 :

```bash
node cli/myapi.mjs upgrade --project-dir ./upgrade-copy --version v0.2.0-beta.9 --dry-run --json
```

Suivez le [guide de répétition et restauration](docs/UPGRADE_REHEARSAL.md). La sauvegarde `.env` du CLI n’est pas une sauvegarde de base. Après une tentative de démarrage cible, le CLI ne redémarre pas automatiquement l’ancienne image : la base a pu migrer. Restaurez une sauvegarde vérifiée antérieure avant un binaire incompatible. Ne supprimez pas les volumes ni les écritures non résolues pour revenir en arrière ; évitez `latest` et les versions seulement planifiées.

## Sécurité et documentation

Gardez l’écoute locale et vérifiez HTTPS, proxys de confiance, inscriptions, rôles et clés avant partage. Utilisez uniquement les comptes/API autorisés, en respectant conditions du fournisseur et lois applicables ; les services publics peuvent imposer des obligations supplémentaires. Le modèle livré active les journaux de contenu complet, désactivés dans l’exemple ci-dessus. Avant activation, définissez droits, rétention et sauvegardes : le masquage ne garantit pas l’absence de données privées dans les prompts/réponses. L’échantillonnage peut contacter le service de modèles en arrière-plan. Excluez secrets, fichiers OAuth, bases et journaux privés de Git et des signalements.

- [Publication et preuves](docs/RELEASE_BETA_7.md) · [Déploiement](DEPLOYMENT_CUSTOM.md) · [LAN](docs/LAN_LITE.md)
- [Restauration](docs/UPGRADE_REHEARSAL.md) · [Validation d’installation](docs/R1_INSTALLATION_CHECK.md)
- [API relay](docs/openapi/relay.json) · [API de gestion](docs/openapi/api.json)
- [Analyse des quotas](docs/QUOTA_ANALYTICS.md) · [Usage organisationnel Claude](docs/CLAUDE_USAGE_REPORT.md)
- [Authentification](docs/authentication.md) · [Journaux de contenu](docs/FULL_CONTENT_LOGGING_CUSTOM.md)
- [Signaler un problème](https://github.com/ForceMind/MyAPI/issues) : version/revision, déploiement et reproduction expurgée

Contributeurs : [plan et comptes rendus de développement](docs/MYAPI_MASTER_PLAN.md). Certains guides détaillés sont en chinois.

## Licence et mentions légales

Licence [GNU AGPLv3](LICENSE). [NOTICE](NOTICE) précise les conditions additionnelles de l’article 7 et les attributions légales/d’interface obligatoires ; les [licences tierces](THIRD-PARTY-LICENSES.md) regroupent les mentions des dépendances. Préservez les mentions applicables, identifiez les modifications et respectez les obligations de fourniture du code source correspondant lors de la distribution ou de l’accès réseau aux versions modifiées. Les distributions desktop doivent aussi conserver les mentions Electron/Chromium applicables. Consultez les conditions complètes avant utilisation ou redistribution.
