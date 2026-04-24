---
name: flightstrips-live-integration-testing
description: Use this skill when validating FlightStrips frontend and backend changes end-to-end in the running app with Chrome DevTools, direct database checks, and the user's existing live-reload setup.
metadata:
  author: flightstrips
  version: "1.0.0"
  category: validation
---

# FlightStrips live integration testing

Use this skill when a FlightStrips change must be proven in the running system, not just by unit tests, linting, or builds.

## Use this skill for

- Validating frontend behavior together with backend effects
- Testing end-to-end flows in the live app
- Checking controller (`/app`) and pilot (`/pilot`) views together when relevant
- Verifying rendered UI, websocket-driven updates, and persisted backend state
- Rechecking timing-sensitive UI states with Chrome DevTools when needed

## Hard constraints

1. Reuse the user's already running frontend and backend when the user says they are up.
2. Do **not** launch a second frontend, backend, database, browser, or alternate app instance.
3. Use the existing Chrome/DevTools session when one is already running or has been restarted by the user.
4. If the user says not to switch test method, do not pivot to another approach without involving them first.
5. If credentials or other user input are needed, ask at runtime; never store or commit secrets in the repo.
6. Treat live reload as the expected workflow. If changes do not appear, debug the running setup rather than starting duplicates.
7. Do not stop at code changes alone; finish only after the exact requested behavior is validated live.
8. Avoid disruptive recovery steps first. Do not restart services, reset the scenario, reset the database, or swap validation method unless the user asks for it or approves it.

## Standard workflow

1. Use Chrome DevTools against the running FlightStrips app.
2. Prefer `http://localhost:8080` over `127.0.0.1:8080` if auth or callback behavior differs.
3. Use `/app` for controller-side behavior and `/pilot` for pilot-side behavior when the scenario spans both sides.
4. If a flow is represented in more than one controller view, use the view that is most authoritative for the feature under test.
5. If a UI state is brief or timing-sensitive, use DevTools scripting to sample styles or state directly instead of relying only on visual timing by eye.
6. After UI validation, confirm the backing state when useful by querying the database directly instead of guessing.
7. If the user reported a specific path, reproduce that exact path before trying variants.

## Database verification

- The skill may inspect the database directly to verify persisted state and narrow down frontend/backend mismatches.
- Use repo configuration to find the database connection details, especially:
  - `backend/.env`
  - `docker-compose.prod.yml`
  - other repo Docker or backend config files if relevant
- Prefer direct DB checks over restarting services when investigating whether a change persisted.
- Use DB inspection to answer questions like:
  - did the backend write the expected state?
  - is the frontend showing stale data?
  - did a later backend step clear or overwrite the expected data?

## Live reload assumptions

- If Go files under `backend/` are changed, assume the running backend will live reload.
- Do not manually restart the backend just because backend Go files changed.
- If the frontend dev server is already running, assume frontend changes should also appear through its normal live-reload flow.
- If a change does not show up, first verify the running app actually reloaded before considering any restart.

## Runtime guidance

- If the browser was closed and restarted by the user, reconnect to that restarted session instead of creating a replacement browser session if possible.
- If the user has already provided a preferred validation path, follow it.
- If a live scenario is reset, start from a fresh test path instead of relying on stale state.
- If a form autofills stale values from a prior lookup, correct them before submitting the live test.
- If the app appears stale:
  1. refresh the relevant page,
  2. verify live reload picked up the change,
  3. inspect backing state directly,
  4. only then ask the user before doing anything more disruptive.

## What good validation looks like

- The exact user-reported path is exercised end-to-end in the live app.
- The rendered UI matches the requested behavior.
- The backend state matches the UI behavior when checked directly.
- Any timing-sensitive highlight, validation, or transition is checked long enough to prove persistence or expiry.
- No unnecessary service restarts or environment resets were performed.
- If the user imposed testing constraints, the work stayed inside those constraints.
