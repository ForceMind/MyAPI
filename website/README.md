# MyAPI website

This directory contains the independent MyAPI product site. It has no build
dependencies and can be served as plain static files; the optional web font
falls back to system fonts when external font loading is unavailable. It is
intentionally separate from the React administration console so the marketing
page never increases the LAN image footprint.

## Preview

From the repository root:

```bash
python3 -m http.server 4173 --directory website
```

Open <http://localhost:4173>. The page only needs a static file server; no secrets, backend, or build service is required.

## Share metadata

`index.html` declares an English document language, one canonical URL, and
matching Open Graph/Twitter title, description, URL, and logo metadata. Until a
public hosting domain is explicitly approved, the canonical URL intentionally
points to the maintained website source on GitHub rather than inventing a
deployment hostname. If hosting moves to an approved domain, update the
canonical, Open Graph, and Twitter URLs together and rerun `npm run website:check`.

The logo is a checked-in copy of the maintained asset in `web/public/myapi-logo-v1.png`. Keeping a regular PNG (rather than a symlink) makes Windows checkouts and GitHub Pages publishing reliable; update both files when the brand asset changes.

## Validation

Run the dependency-free checks from the repository root before publishing or
reviewing a website change:

```bash
npm run website:check
```

The check validates the page structure, responsive metadata, logo parity,
legacy-brand exclusions, keyboard focus styling, and theme/menu accessibility
attributes. The theme follows the browser preference until a visitor chooses a
theme; private browsing environments may disable persistence without breaking
the page.

## Reviewable release artifact

The repository includes a `Package MyAPI static website` GitHub Actions
workflow at `.github/workflows/website-artifact.yml`. It runs automatically on
`main` when the website, its checks, branding checks, package metadata, or the
workflow changes; you can also choose **Run workflow** from the Actions tab
and select a retention period from 1 to 14 days. The workflow runs
`npm run website:check`, then uploads three reviewable files:

- `myapi-website-<commit>.tar.gz`, a deterministic archive of `website/`;
- its `.sha256` checksum; and
- `website-artifact-metadata.txt`, recording the commit and retention period.

This workflow only creates a review artifact. It does not publish to GitHub
Pages, a CDN, Docker, or any external domain, and it has no write token.
After downloading the artifact, verify it locally with:

```bash
sha256sum --check myapi-website-<commit>.tar.gz.sha256
tar -tzf myapi-website-<commit>.tar.gz
```

Publishing remains a separate, explicitly reviewed decision once a hosting
target and domain are confirmed.
