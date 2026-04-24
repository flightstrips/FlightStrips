---
name: flightstrips-release-commit-messages
description: Use this skill when drafting or creating FlightStrips commit messages so they use the right conventional commit scope for release notes.
metadata:
  author: flightstrips
  version: "1.0.0"
  category: workflow
---

# FlightStrips release commit messages

Use Conventional Commits: `type(scope): summary`.
Do not use top-level app names like `frontend`, `backend`, or `euroscope-plugin` as the scope; use the real subsystem instead.
Prefer narrow scopes like `pdc`, `strips`, `websocket`, `auth`, `sessions`, or `plugin-sync`.
Keep the summary short, imperative, and release-note friendly.
Use `type(scope)!: summary` plus a `BREAKING CHANGE:` footer for breaking changes.
