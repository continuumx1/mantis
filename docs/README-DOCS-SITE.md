# Building the docs site locally

This isn't part of the published site (see `mkdocs.yml`'s `exclude_docs`) —
it's for whoever's editing the site itself.

The site is [MkDocs](https://www.mkdocs.org/) with the
[Material](https://squidfunk.github.io/mkdocs-material/) theme, config at
the repo root (`mkdocs.yml`), content in `docs/*.md`. It deploys to GitHub
Pages automatically via `.github/workflows/docs.yml` on every push to
`main` that touches `docs/` or `mkdocs.yml` — **except that workflow needs
one manual, one-time setup step it can't do itself: repo Settings → Pages →
Source → "GitHub Actions".** Until that's set, the workflow will build
successfully but fail at the deploy step with a clear message pointing
back here.

## Local setup

```bash
python3 -m venv .venv-mkdocs
source .venv-mkdocs/bin/activate
pip install mkdocs mkdocs-material
```

(`.venv-mkdocs/` and `/site/` — the build output — are both gitignored;
neither belongs in source control.)

## Commands

```bash
mkdocs serve              # live-reloading local server, http://127.0.0.1:8000
mkdocs build --strict     # what CI runs — fails on broken links/nav, not just warns
```

## Adding a page

1. Add the `.md` file under `docs/`.
2. Add it to the `nav:` list in `mkdocs.yml` — a page not in `nav` still
   builds (and is reachable by direct URL) but won't show in the sidebar,
   and `--strict` will flag it.
3. `mkdocs build --strict` before committing — it catches broken internal
   links (`](some-page.md)`) and bad anchors, which are easy to introduce
   when restructuring content.

## Keeping this in sync with the rest of the repo

Several pages intentionally mirror content that also lives elsewhere
(`README.md`'s Roadmap, `charts/mantis/values.yaml`'s defaults, the
version numbers in `Chart.yaml`). Each page says where its source of
truth is when that's the case — if they drift, the other file is the one
that's stale, not this site.
