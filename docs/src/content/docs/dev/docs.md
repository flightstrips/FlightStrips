---
title: Writing Docs Locally
description: How to run the docs site locally and preview changes while editing content.
---

Requires Node.js 22.x. From `docs/`:

```sh
npm ci
npm run dev
```

Dev server runs at `localhost:4321`. Keep it running while editing — changes hot-reload.

## Content

Pages live in `docs/src/content/docs/`. The file path maps directly to the route — `dev/foo.md` becomes `/dev/foo/`.

Each page needs frontmatter:

```md
---
title: Page Title
description: Short summary.
---
```

## Build

```sh
npm run build    # production output
npm run preview  # preview the built site
```
