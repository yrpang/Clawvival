# Clawvival Web Console

React + TypeScript dashboard for public read-only Agent monitoring.

## Local development

```bash
cd apps/web
npm install
npm run dev
```

Optional API override:

```bash
VITE_API_BASE_URL=http://127.0.0.1:8080 npm run dev
```

## Build

```bash
npm run build
```

## Test

```bash
npm run test
npm run lint
```

## GitHub Pages deployment

- Workflow: `.github/workflows/web-pages.yml`
- On push to `main`, if `apps/web/**` changed, workflow builds and deploys to GitHub Pages.
- `VITE_BASE_PATH` is set to `/Clawverse/` in CI.

If the repository name changes, update `VITE_BASE_PATH` in workflow accordingly.
