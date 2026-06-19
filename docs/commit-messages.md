# Commit Message Policy

Use Release Please compatible Conventional Commits for all commit messages that may end up in release notes.

## Format

Use:

```text
<type>(<scope>): <subject>
```

Scope is optional:

```text
<type>: <subject>
```

## Allowed types

Use these types:

- `feat`
- `fix`
- `deps`
- `chore`

Use `!` for breaking changes when needed:

```text
feat(pdc)!: require reviewed route before web clearance issue
```

Use `chore` when a commit should not show up as a user-facing release-note change.

For this repo, do not use other generic types such as `docs`, `refactor`, `test`, `build`, `ci`, `perf`, or `revert` for release-note commit titles.

## Scope rules

Use a scope when it adds domain meaning.

Do not use product or package scopes such as:

- `backend`
- `frontend`
- `euroscope-plugin`
- `docs`

Prefer feature or domain scopes such as:

- `cdm`
- `pdc`
- `ecfmp`
- `strip`
- `clearance`
- `session`
- `websocket`
- `private-message`

If a change spans multiple products, still scope by the domain or behavior being changed, not by the product directory.

## Subject rules

- Use imperative mood.
- Keep it concise and release-note friendly.
- Start with the change itself, not with filler words.
- Do not end the subject with a period.

## Examples

- `feat(cdm): show startup status in strip dialog`
- `fix(pdc): prefer SID over stale vectored departure text`
- `deps(front-strip): bump zustand to 6.0.1`
- `chore(ecfmp): rename injected test helpers`
- `fix(private-message): route sends through controller CID`
- `feat(ecfmp)!: require explicit restriction acknowledgment`
