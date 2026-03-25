---
title: Writing Docs Locally
description: How to run the docs site locally and preview changes while editing content.
---

### 1. Install Node.js (Node 22.x)

1. Download and install Node.js **22.x** from the official site: https://nodejs.org/
2. During installation, leave the default option enabled to install `npm`.

### 2. Validate Node.js and npm are installed

Open a **new terminal** and run:

```sh
node -v
npm -v
```

You should see versions that start with `v22.` for Node, and a numeric version for npm (for example `10.x`).


## Run the docs site locally

From the repo root:

```sh
cd docs
npm ci
npm run dev
```

The site will start a local server (Astro dev). Keep it running while you edit docs.

## Edit docs content

Starlight renders pages from this folder:

- `docs/src/content/docs/`

The path and filename determine the route. (For example, `dev/foo.md` becomes a page under `/dev/foo/`.)

### Page frontmatter

Most pages include frontmatter like:

```md
---
title: My Page Title
description: Optional short summary for the page.
---
```

## Useful commands

```sh
# Build production output
npm run build

# Preview the built site locally
npm run preview
```
