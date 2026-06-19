# Commit Messages

When asked to write a commit message, use Release Please compatible Conventional Commits so the message is suitable for release notes.

- Format as `<type>(<scope>): <subject>` or `<type>: <subject>`.
- Use `feat`, `fix`, and `deps` for releasable changes.
- Use `chore` when a commit should not show up in release notes.
- Use scope when it adds meaning, but never use product scopes such as `backend`, `frontend`, `euroscope-plugin`, or `docs`.
- Prefer domain scopes such as `cdm`, `pdc`, `ecfmp`, `strip`, `clearance`, `session`, `websocket`, or `private-message`.
- Keep the subject imperative, concise, and without a trailing period.
- Use `!` for breaking changes when needed.
- Do not use other generic types such as `docs`, `refactor`, `test`, `build`, `ci`, `perf`, or `revert`.

Canonical examples and wording live in `docs/commit-messages.md`.
