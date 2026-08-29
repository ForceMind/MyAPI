<div align="center">

![MyAPI](/web/public/myapi-logo-v1.png)

# MyAPI

🍥 **Passerelle de modèles étendus de nouvelle génération et système de gestion d'actifs d'IA**

<p align="center">
  <a href="./README.zh_CN.md">简体中文</a> |
  <a href="./README.zh_TW.md">繁體中文</a> |
  <a href="./README.md">English</a> |
  <strong>Français</strong> |
  <a href="./README.ja.md">日本語</a>
</p>

<p align="center">
  <a href="https://raw.githubusercontent.com/ForceMind/MyAPI/main/LICENSE">
    <img src="https://img.shields.io/github/license/ForceMind/MyAPI?color=brightgreen" alt="license">
  </a><!--
  --><a href="https://github.com/ForceMind/MyAPI/releases/latest">
    <img src="https://img.shields.io/github/v/release/ForceMind/MyAPI?color=brightgreen&include_prereleases" alt="release">
  </a>
</p>

<p align="center">
  <a href="#-démarrage-rapide">Démarrage rapide</a> •
  <a href="#-fonctionnalités-clés">Fonctionnalités clés</a> •
  <a href="#-déploiement">Déploiement</a> •
  <a href="#-documentation">Documentation</a> •
  <a href="#-aide-support">Aide</a>
</p>

</div>

## Distribution auto-hébergée MyAPI

<img src="./web/public/myapi-logo-v1.png" alt="MyAPI" width="160" />

Ce dépôt fournit le CLI d'auto-hébergement **MyAPI** et une distribution
complète du code source basée sur la référence de compatibilité rc.25. La couche
de distribution utilise le slug machine my-api; les contrats API, SSE, base de
données et protocoles amont restent compatibles. Les mentions de licence,
NOTICE et d'attribution tierce restent dans les fichiers juridiques du dépôt.

```bash
npx @forcemind/myapi init ./myapi-source
npx @forcemind/myapi configure \
  --project-dir ./myapi-source \
  --public-url https://api.your-domain.com
npx @forcemind/myapi doctor --project-dir ./myapi-source
```

Consultez le [guide de distribution MyAPI](./docs/MYAPI_DISTRIBUTION.md) pour le
déploiement, l'adoption des données, la validation et la publication.

## 📝 Description du projet

> [!IMPORTANT]
> - Ce projet est exclusivement destiné aux scénarios de passerelle API d'IA légalement autorisés, d'authentification organisationnelle, de gestion multi-modèles, d'analyse d'utilisation, de comptabilisation des coûts et de déploiement privé.
> - Les utilisateurs doivent obtenir légalement les clés API, comptes, services de modèles et autorisations d'interface en amont, et doivent respecter les conditions d'utilisation en amont et les lois et réglementations applicables.
> - Les utilisateurs doivent s'assurer que leur utilisation est conforme aux conditions d'utilisation en amont et aux lois et réglementations applicables.
> - Lors de la fourniture de services d'IA générative au public, les utilisateurs doivent se conformer aux exigences réglementaires applicables et remplir toutes les obligations d'enregistrement, de licence, de sécurité du contenu, de vérification d'identité, de conservation des journaux, de fiscalité et d'autorisation en amont requises par leur juridiction.

---

## 🙏 Remerciements

MyAPI conserve les références tierces lorsque leurs API ou licences l'exigent.
Consultez LICENSE, NOTICE et les métadonnées des dépendances pour les mentions
complètes. Aucun client tiers ni contenu sponsorisé n'est activé par défaut.

---

## 🚀 Démarrage rapide

### Utilisation de Docker Compose (recommandé)

```bash
# Cloner le projet
git clone https://github.com/ForceMind/MyAPI.git my-api
cd my-api

# Modifier la configuration docker-compose.yml
nano docker-compose.yml

# Démarrer le service
docker-compose up -d
```

<details>
<summary><strong>Utilisation des commandes Docker</strong></summary>

```bash
# Build the local image (for local development only)
docker build -t local/my-api:custom-rc25 .

# Utilisation de SQLite (par défaut)
docker run --name my-api -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  local/my-api:custom-rc25

# Utilisation de MySQL
docker run --name my-api -d --restart always \
  -p 3000:3000 \
  -e SQL_DSN="root:123456@tcp(localhost:3306)/oneapi" \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  local/my-api:custom-rc25
```

> **💡 Astuce:** `-v ./data:/data` sauvegardera les données dans le dossier `data` du répertoire actuel, vous pouvez également le changer en chemin absolu comme `-v /your/custom/path:/data`

</details>

---

🎉 Après le déploiement, visitez `http://localhost:3000` pour commencer à utiliser!

> [!WARNING]
> Lorsque vous exploitez ce projet en tant que service public d'IA générative ou service de revente d'API, les utilisateurs doivent d'abord remplir toutes les obligations requises en matière d'enregistrement, de licence, de sécurité du contenu, de vérification d'identité, de conservation des journaux, de fiscalité, de paiement et d'autorisation en amont.

📖 Pour plus de méthodes de déploiement, veuillez vous référer à [Guide de déploiement](./DEPLOYMENT_CUSTOM.md)

---

## 📚 Documentation

<div align="center">

### 📖 [Documentation officielle](https://github.com/ForceMind/MyAPI/tree/main/docs) | [![Demander à DeepWiki](https://deepwiki.com/badge.svg)](https://github.com/ForceMind/MyAPI/discussions)

</div>

**Navigation rapide:**

| Catégorie | Lien |
|------|------|
| 🚀 Guide de déploiement | [Documentation d'installation](./DEPLOYMENT_CUSTOM.md) |
| ⚙️ Configuration de l'environnement | [Variables d'environnement](./DEPLOYMENT_CUSTOM.md) |
| 📡 Documentation de l'API | [Documentation de l'API](./docs/openapi/relay.json) |
| ❓ FAQ | [FAQ](https://github.com/ForceMind/MyAPI/discussions) |
| 💬 Interaction avec la communauté | [Canaux de communication](https://github.com/ForceMind/MyAPI/discussions) |

---

## ✨ Fonctionnalités clés

> Pour les fonctionnalités détaillées, veuillez vous référer à [Présentation des fonctionnalités](https://github.com/ForceMind/MyAPI/tree/main/docs) |

### 🎨 Fonctions principales

| Fonctionnalité | Description |
|------|------|
| 🎨 Nouvelle interface utilisateur | Conception d'interface utilisateur moderne |
| 🌍 Multilingue | Prend en charge le chinois simplifié, le chinois traditionnel, l'anglais, le français et le japonais |
| 🔄 Compatibilité des données | Compatible avec les migrations de données existantes |
| 📈 Tableau de bord des données | Console visuelle et analyse statistique |
| 🔒 Gestion des permissions | Regroupement de jetons, restrictions de modèles, gestion des utilisateurs |

### 💰 Comptabilisation et facturation des usages autorisés

- ✅ Rechargement interne et allocation de quotas pour les scénarios légalement autorisés (EPay, Stripe)
- ✅ Comptabilisation des coûts par requête, par utilisation et par hit de cache au niveau organisationnel
- ✅ Statistiques de facturation du cache pour OpenAI, Azure, DeepSeek, Claude, Qwen et les modèles pris en charge
- ✅ Politiques de facturation flexibles pour la gestion interne ou les clients entreprise autorisés

### 🔐 Autorisation et sécurité

- 😈 Connexion par autorisation Discord
- 🤖 Connexion par autorisation LinuxDO
- 📱 Connexion par autorisation Telegram
- 🔑 Authentification unifiée OIDC

### 🚀 Fonctionnalités avancées

**Prise en charge des formats d'API:**
- ⚡ [OpenAI Responses](./docs/openapi/relay.json)
- ⚡ [OpenAI Realtime API](./docs/openapi/relay.json) (y compris Azure)
- ⚡ [Claude Messages](./docs/openapi/relay.json)
- ⚡ [Google Gemini](./docs/openapi/relay.json)
- 🔄 [Modèles Rerank](./docs/openapi/relay.json) (Cohere, Jina)

**Routage intelligent:**
- ⚖️ Sélection aléatoire pondérée des canaux
- 🔄 Nouvelle tentative automatique en cas d'échec
- 🚦 Limitation du débit du modèle pour les utilisateurs

**Conversion de format:**
- 🔄 **OpenAI Compatible ⇄ Claude Messages**
- 🔄 **OpenAI Compatible → Google Gemini**
- 🔄 **Google Gemini → OpenAI Compatible** - Texte uniquement, les appels de fonction ne sont pas encore pris en charge
- 🚧 **OpenAI Compatible ⇄ OpenAI Responses** - En développement
- 🔄 **Fonctionnalité de la pensée au contenu**

**Prise en charge de l'effort de raisonnement:**

<details>
<summary>Voir la configuration détaillée</summary>

**Modèles de la série OpenAI :**
- `o3-mini-high` - Effort de raisonnement élevé
- `o3-mini-medium` - Effort de raisonnement moyen
- `o3-mini-low` - Effort de raisonnement faible
- `gpt-5-high` - Effort de raisonnement élevé
- `gpt-5-medium` - Effort de raisonnement moyen
- `gpt-5-low` - Effort de raisonnement faible

**Modèles de pensée de Claude:**
- `claude-3-7-sonnet-20250219-thinking` - Activer le mode de pensée

**Modèles de la série Google Gemini:**
- `gemini-2.5-flash-thinking` - Activer le mode de pensée
- `gemini-2.5-flash-nothinking` - Désactiver le mode de pensée
- `gemini-2.5-pro-thinking` - Activer le mode de pensée
- `gemini-2.5-pro-thinking-128` - Activer le mode de pensée avec budget de pensée de 128 tokens
- Vous pouvez également ajouter les suffixes `-low`, `-medium` ou `-high` aux modèles Gemini pour fixer le niveau d’effort de raisonnement (sans suffixe de budget supplémentaire).

</details>

---

## 🤖 Prise en charge des modèles

> Pour les détails, veuillez vous référer à [Documentation de l'API - Interface de passerelle](./docs/openapi/relay.json)

| Type de modèle | Description | Documentation |
|---------|------|------|
| 🤖 OpenAI-Compatible | Modèles compatibles OpenAI | [Documentation](./docs/openapi/relay.json) |
| 🤖 OpenAI Responses | Format OpenAI Responses | [Documentation](./docs/openapi/relay.json) |
| 🎨 Midjourney-Proxy | [Midjourney-Proxy(Plus)](https://github.com/novicezk/midjourney-proxy) | [Documentation](./docs/openapi/relay.json) |
| 🎵 Suno-API | [Suno API](https://github.com/Suno-API/Suno-API) | [Documentation](./docs/openapi/relay.json) |
| 🔄 Rerank | Cohere, Jina | [Documentation](./docs/openapi/relay.json) |
| 💬 Claude | Format Messages | [Documentation](./docs/openapi/relay.json) |
| 🌐 Gemini | Format Google Gemini | [Documentation](./docs/openapi/relay.json) |
| 🔧 Dify | Mode ChatFlow | - |
| 🎯 Amont personnalisé | Configuration des points d'accès amont légalement autorisés | - |

### 📡 Interfaces prises en charge

<details>
<summary>Voir la liste complète des interfaces</summary>

- [Interface de discussion (Chat Completions)](./docs/openapi/relay.json)
- [Interface de réponse (Responses)](./docs/openapi/relay.json)
- [Interface d'image (Image)](./docs/openapi/relay.json)
- [Interface audio (Audio)](./docs/openapi/relay.json)
- [Interface vidéo (Video)](./docs/openapi/relay.json)
- [Interface d'incorporation (Embeddings)](./docs/openapi/relay.json)
- [Interface de rerank (Rerank)](./docs/openapi/relay.json)
- [Conversation en temps réel (Realtime)](./docs/openapi/relay.json)
- [Discussion Claude](./docs/openapi/relay.json)
- [Discussion Google Gemini](./docs/openapi/relay.json)

</details>

---

## 🚢 Déploiement

> [!TIP]
> **Image Docker par défaut :** `ghcr.io/forcemind/myapi:v0.1.1` (construite par GitHub Actions ; modifiez `MYAPI_IMAGE` pour mettre à niveau). Définissez `MYAPI_BUILD_LOCAL=true` pour construire localement.

La création d’un tag de version `vX.Y.Z` déclenche automatiquement la construction et la
publication de l’image multi-architecture GHCR ; mettez `MYAPI_IMAGE` à jour avant le déploiement.

### 📋 Exigences de déploiement

| Composant | Exigence |
|------|------|
| **Base de données locale** | SQLite (Docker doit monter le répertoire `/data`)|
| **Base de données distante | MySQL ≥ 5.7.8 ou PostgreSQL ≥ 9.6 |
| **Moteur de conteneur** | Docker / Docker Compose |
| **Architecture système** | 64 bits uniquement (amd64 / arm64) ; les systèmes 32 bits ne sont pas pris en charge |

### ⚙️ Configuration des variables d'environnement

<details>
<summary>Configuration courante des variables d'environnement</summary>

| Nom de variable | Description | Valeur par défaut |
|--------|------|--------|
| `SESSION_SECRET` | Secret de signature d’authentification, identique sur tous les nœuds | - |
| `SESSION_COOKIE_SECURE` | `false`/non défini désactive l’OriginGuard de refresh/logout pour les proxys HTTP locaux ; `true` active le cookie Secure et le contrôle strict de l’Origin | `false` |
| `SESSION_COOKIE_TRUSTED_URL` | Obligatoire en mode Secure : Origins HTTPS exactes autorisées pour refresh/logout, séparées par des virgules ; ce n’est pas une liste CORS relay | - |
| `TRUSTED_PROXIES` | Variable absente/vide : approuve le bouclage, les réseaux RFC 1918 et l’ULA IPv6 avec un avertissement au démarrage ; `none` n’approuve aucun proxy ; une liste IP/CIDR explicite remplace les valeurs par défaut | `127.0.0.0/8, ::1, 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, fc00::/7` |
| `USER_SESSION_ACTIVE_LIMIT` | Nombre maximal de Sessions de connexion actives par utilisateur | `50` |
| `USER_SESSION_ISSUANCE_LIMIT` | Nombre maximal de Sessions créées par utilisateur dans la fenêtre, y compris les Sessions révoquées | `100` |
| `USER_SESSION_ISSUANCE_WINDOW_SECONDS` | Fenêtre de comptage des Sessions ; limitée à la durée de conservation des Sessions révoquées si elle est supérieure | `86400` |
| `USER_SESSION_REVOKED_RETENTION_DAYS` | Conservation en jours des Sessions révoquées pour l’audit et le comptage | `7` |
| `USER_SESSION_HOURLY_ALERT_THRESHOLD` | Seuil global horaire déclenchant uniquement une alerte, sans bloquer les connexions | `5000` |
| `CRYPTO_SECRET` | Secret HMAC des clés de cache ; les nœuds partageant Redis doivent utiliser la même valeur effective | Par défaut, `SESSION_SECRET` |
| `SQL_DSN` | Chaine de connexion à la base de données | - |
| `REDIS_CONN_STRING` | Chaine de connexion Redis | - |
| `STREAMING_TIMEOUT` | Délai d'expiration du streaming (secondes) | `300` |
| `STREAM_SCANNER_MAX_BUFFER_MB` | Taille max du buffer par ligne (Mo) pour le scanner SSE ; à augmenter quand les sorties image/base64 sont très volumineuses (ex. images 4K) | `64` |
| `MAX_REQUEST_BODY_MB` | Taille maximale du corps de requête (Mo, comptée **après décompression** ; évite les requêtes énormes/zip bombs qui saturent la mémoire). Dépassement ⇒ `413` | `32` |
| `AZURE_DEFAULT_API_VERSION` | Version de l'API Azure | `2025-04-01-preview` |
| `ERROR_LOG_ENABLED` | Interrupteur du journal d'erreurs | `false` |
| `PYROSCOPE_URL` | Adresse du serveur Pyroscope | - |
| `PYROSCOPE_APP_NAME` | Nom de l'application Pyroscope | `my-api` |
| `PYROSCOPE_BASIC_AUTH_USER` | Utilisateur Basic Auth Pyroscope | - |
| `PYROSCOPE_BASIC_AUTH_PASSWORD` | Mot de passe Basic Auth Pyroscope | - |
| `PYROSCOPE_MUTEX_RATE` | Taux d'échantillonnage mutex Pyroscope | `5` |
| `PYROSCOPE_BLOCK_RATE` | Taux d'échantillonnage block Pyroscope | `5` |
| `HOSTNAME` | Nom d'hôte tagué pour Pyroscope | `my-api` |

📖 **Configuration complète:** [Documentation des variables d'environnement](./DEPLOYMENT_CUSTOM.md)

</details>

### 🔧 Méthodes de déploiement

<details>
<summary><strong>Méthode 1: Docker Compose (recommandé)</strong></summary>

```bash
# Cloner le projet
git clone https://github.com/ForceMind/MyAPI.git my-api
cd my-api

# Modifier la configuration
nano docker-compose.yml

# Démarrer le service
docker-compose up -d
```

</details>

<details>
<summary><strong>Méthode 2: Commandes Docker</strong></summary>

**Utilisation de SQLite:**
```bash
docker run --name my-api -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  local/my-api:custom-rc25
```

**Utilisation de MySQL:**
```bash
docker run --name my-api -d --restart always \
  -p 3000:3000 \
  -e SQL_DSN="root:123456@tcp(localhost:3306)/oneapi" \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  local/my-api:custom-rc25
```

> **💡 Explication du chemin:**
> - `./data:/data` - Chemin relatif, données sauvegardées dans le dossier data du répertoire actuel
> - Vous pouvez également utiliser un chemin absolu, par exemple : `/your/custom/path:/data`

</details>

<details>
<summary><strong>Méthode 3: Panneau BaoTa</strong></summary>

1. Installez le panneau BaoTa (version ≥ 9.2.0)
2. Recherchez **MyAPI** dans le magasin d'applications
3. Installation en un clic

📖 [Tutoriel avec des images](./docs/BT.md)

</details>

### ⚠️ Considérations sur le déploiement multi-machines

> [!WARNING]
> - Tous les nœuds doivent utiliser la même base de données principale et la même valeur `SESSION_SECRET` ; sinon les Access Tokens, sessions Refresh et flux d’authentification temporaires ne peuvent pas être vérifiés de façon cohérente.
> - Les nœuds connectés au même Redis doivent aussi utiliser le même `CRYPTO_SECRET`, faute de quoi les empreintes de clé de cache diffèrent et les entrées partagées ne peuvent pas être réutilisées de façon cohérente.

La base de données fait autorité pour les Sessions de connexion et pour les limites actives/d’émission par utilisateur. Les entrées Session de Redis sont des caches de courte durée dont le TTL suit `SYNC_FREQUENCY` (60 secondes par défaut), sans jamais dépasser la durée de vie restante de la Session.

| Topologie Redis | Propagation des Sessions | Limitation de débit |
| --- | --- | --- |
| Redis partagé | Les révocations et publications de version se propagent normalement immédiatement | Les quotas Redis sont partagés entre les nœuds |
| Redis indépendant par nœud | Les nœuds se resynchronisent depuis la base dans le délai effectif de `SYNC_FREQUENCY` ; un nouveau Token issu d’une rotation peut recevoir temporairement une réponse 401 sur un nœud dont le cache est obsolète | Chaque nœud possède son propre quota ; la capacité agrégée peut donc atteindre environ la limite configurée multipliée par le nombre de nœuds |
| Sans Redis | Chaque validation de Session consulte directement la base de données | Les limites en mémoire sont indépendantes sur chaque nœud |

Réduire `SYNC_FREQUENCY` raccourcit la fenêtre d’obsolescence avec des Redis indépendants, mais ajoute une lecture de Session par clé primaire, par SID actif, par nœud et par TTL. Ces garanties donnent une obsolescence bornée à l’authentification Session ; les limites et les autres caches du plan de contrôle adossés à Redis restent dépendants de la topologie.

Consultez [Authentification utilisateur et sessions de connexion](./docs/authentication.md) pour les contrats de token, de vérification Origin et de PAT.

### 🔄 Nouvelle tentative de canal et cache

**Configuration de la nouvelle tentative:** `Paramètres → Paramètres de fonctionnement → Paramètres généraux → Nombre de tentatives en cas d'échec`

**Configuration du cache:**
- `REDIS_CONN_STRING`: Cache Redis (recommandé)
- `MEMORY_CACHE_ENABLED`: Cache mémoire

---

## 🔗 Projets connexes

### Projets en amont

| Projet | Description |
|------|------|
| [Midjourney-Proxy](https://github.com/novicezk/midjourney-proxy) | Prise en charge de l'interface Midjourney |

### Outils d'accompagnement

Le panneau d'administration MyAPI permet de consulter les quotas de clés et les
journaux d'audit. La version par défaut ne fait pas la promotion d'outils tiers.

---

## 💬 Aide et support

### 📖 Ressources de documentation

| Ressource | Lien |
|------|------|
| 📘 FAQ | [FAQ](https://github.com/ForceMind/MyAPI/discussions) |
| 💬 Interaction avec la communauté | [Canaux de communication](https://github.com/ForceMind/MyAPI/discussions) |
| 🐛 Commentaires sur les problèmes | [Commentaires sur les problèmes](https://github.com/ForceMind/MyAPI/issues) |
| 📚 Documentation complète | [Documentation officielle](https://github.com/ForceMind/MyAPI/tree/main/docs) |

### 🤝 Guide de contribution

Bienvenue à toutes les formes de contribution!

- 🐛 Signaler des bogues
- 💡 Proposer de nouvelles fonctionnalités
- 📝 Améliorer la documentation
- 🔧 Soumettre du code

---

## 📜 Licence

Ce projet est sous licence [GNU Affero General Public License v3.0 (AGPLv3)](./LICENSE).

MyAPI est une distribution indépendante, avec des adaptateurs de compatibilité pour les données et contrats d’API existants.

Les obligations d'attribution et de l'article 7 de l'AGPLv3 sont documentées
dans LICENSE et NOTICE; consultez-les avant de redistribuer une version modifiée.
Si votre organisation ne peut pas accepter l'AGPLv3, demandez conseil à votre
équipe juridique avant toute utilisation.


---

<div align="center">

### 💖 Merci d'utiliser MyAPI

Si ce projet vous est utile, bienvenue à nous donner une ⭐️ Étoile！

**[Documentation officielle](https://github.com/ForceMind/MyAPI/tree/main/docs)** • **[Commentaires sur les problèmes](https://github.com/ForceMind/MyAPI/issues)** • **[Dernière version](https://github.com/ForceMind/MyAPI/releases)**

<sub>Construit avec ❤️ par ForceMind</sub>

</div>
