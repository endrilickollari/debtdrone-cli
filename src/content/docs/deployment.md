---
title: Documentation deployment
description: Build, publish, verify, and recover the DebtDrone CLI documentation site.
---

The DebtDrone CLI documentation is published at
[`https://cli.debtdrone.net`](https://cli.debtdrone.net) from this repository.
Documentation publishing is independent of CLI releases and does not create a
version tag or binary artifact.

## Deployment path

The **Deploy Documentation** workflow runs for documentation pull requests and
for matching changes merged into `main`:

1. install the versions locked in `package-lock.json`;
2. run `npm run check:docs` to build and validate the static site;
3. upload `docs-dist` as the workflow artifact; and
4. publish the artifact to the orphan `gh-pages` branch when the triggering ref
   is `main`.

Pull requests execute the same build and verification but never publish. The
workflow uses one concurrency group without canceling an active deployment, so
two merges cannot publish over each other midway through a run.

## Custom domain and discovery

The repository owns the public-site configuration that belongs in source:

- `astro.config.mjs` sets `https://cli.debtdrone.net` as the production site;
- `public/CNAME` preserves the GitHub Pages custom domain;
- `public/robots.txt` allows indexing and declares the sitemap;
- `public/llms.txt` provides curated entry points for agents; and
- Starlight generates canonical metadata, Pagefind search, and sitemap files.

Two external settings are already provisioned and must remain aligned with the
repository:

| Provider | Setting | Expected value |
|---|---|---|
| Cloudflare DNS | `CNAME` record for `cli` | `endrilickollari.github.io` |
| GitHub Pages | Custom domain | `cli.debtdrone.net` with HTTPS enforced |

Changes to those provider settings are not made by this repository. If the
domain changes, update the external settings, `astro.config.mjs`, `public/CNAME`,
`public/robots.txt`, `public/llms.txt`, and the workflow environment together.

## Verify a deployment

Run the production check locally before opening a pull request:

```bash title="Terminal · Build and verify documentation"
npm ci
npm run check:docs
```

After deployment, verify the workflow is green and check:

```bash title="Terminal · Verify the public endpoints"
curl -I https://cli.debtdrone.net/
curl -fsSL https://cli.debtdrone.net/robots.txt
curl -fsSL https://cli.debtdrone.net/sitemap-index.xml
curl -fsSL https://cli.debtdrone.net/llms.txt
```

The home page must return HTTPS `200`, canonical URLs and sitemap locations must
use the custom domain, and the documentation search must return a result for a
known term such as `max-complexity`.

## Roll back or recover

For an immediate rollback, open **Actions → Deploy Documentation**, select the
last known-good successful run, and choose **Re-run all jobs**. The run rebuilds
and republishes its original commit. Then revert the faulty change through a
pull request so `main` and the published site converge again.

Use the narrower recovery path when possible:

- **Build failed:** fix the failure on a feature branch. No deployment occurs,
  so the last successful site remains available.
- **Publish failed:** re-run the failed workflow. If the artifact expired, run
  the workflow manually from current `main`.
- **Custom domain returns 404:** confirm the `gh-pages` branch contains `CNAME`,
  then confirm the GitHub Pages custom domain and Cloudflare CNAME values above.
- **Certificate or redirect failed:** confirm **Enforce HTTPS** remains enabled
  in GitHub Pages and that the DNS record still resolves to GitHub Pages.
- **Search failed:** confirm `pagefind/pagefind.js`, `pagefind-entry.json`, and
  fragment files exist in the published branch, then rebuild from `main`.

Do not edit `gh-pages` manually. It is generated output and is replaced by the
next successful deployment.
