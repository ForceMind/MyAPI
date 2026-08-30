# MyAPI website browser acceptance

The repository now includes a deterministic Chromium smoke test for the static
website. It serves `website/` from a local, in-process HTTP server and checks the
mobile-width workflow that previously could only be reviewed statically:

- MyAPI title and brand are visible;
- the mobile menu starts closed, opens from the Menu button, and closes after an
  anchor navigation;
- the light-theme toggle updates the document state;
- a full-page mobile screenshot is emitted for review.

The check runs from `.github/workflows/website-browser.yml` on website changes
and can also be started with `workflow_dispatch`. GitHub Actions installs the
pinned Playwright runner and Chromium, then uploads the screenshot as a short
retention artifact. This workflow does not publish the website or change a
deployment.

Running locally requires Node.js and Playwright:

```sh
npm install --no-save --ignore-scripts playwright@1.55.0
npx playwright install chromium
node tools/website/browser-smoke.mjs
```

The local environment used for repository checks may not contain a browser; in
that case the static check remains useful, while the Actions run is the
authoritative browser evidence. Real-device touch, dynamic address-bar and
screen-reader acceptance remain platform-owner checks.
