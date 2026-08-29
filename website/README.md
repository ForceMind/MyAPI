# MyAPI website

This directory contains the independent, dependency-free MyAPI product site. It is intentionally separate from the React administration console so the marketing page never increases the LAN image footprint.

## Preview

From the repository root:

```bash
python3 -m http.server 4173 --directory website
```

Open <http://localhost:4173>. The page only needs a static file server; no secrets, backend, or build service is required.

The logo is a checked-in copy of the maintained asset in `web/public/myapi-logo-v1.png`. Keeping a regular PNG (rather than a symlink) makes Windows checkouts and GitHub Pages publishing reliable; update both files when the brand asset changes.

Publishing is deliberately not wired to the application image workflow yet. Add a separate, reviewed workflow when a domain and publishing target are confirmed.
