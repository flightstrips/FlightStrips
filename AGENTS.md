# Production Deployment

Production runs on Docker Swarm, deployed by Portainer from the separate
[flightstrips/infrastructure](https://github.com/flightstrips/infrastructure) repository.
The authoritative stack is [`stacks/flightstrips.yml`](https://github.com/flightstrips/infrastructure/blob/main/stacks/flightstrips.yml).
This repository's `docker-compose.prod.yml` is a legacy configuration and is not used for production.
When changing production image tags, environment variables, resource paths, or mounts,
update the infrastructure stack as needed; changing the legacy Compose file alone does not update production.

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
