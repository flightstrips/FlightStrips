# FSRADIS

FSRADIS is a standalone, local-only prototype project that can later be integrated into FlightStrips.

It contains:

- A React + TypeScript frontend
- A simple mock backend (Express) with in-memory data updates

## Why this is safe for FlightStrips

- It lives in its own folder: `FSRADIS/`
- It has separate dependencies and scripts
- No imports or runtime coupling with existing FlightStrips services

## Project structure

- `frontend/` - Vite + React + TypeScript UI
- `backend/` - Express mock API

## Quick start

1. Install backend dependencies

```bash
cd backend
npm install
npm run dev
```

2. In a second terminal, install and run frontend

```bash
cd frontend
npm install
npm run dev
```

3. Open the frontend URL shown by Vite (usually `http://localhost:5173`)

## Mock API endpoints

- `GET /api/health`
- `GET /api/strips`
- `POST /api/strips/:id/status` with JSON body `{ "status": "ACTIVE" }`

Example:

```bash
curl -X POST http://localhost:4000/api/strips/s1/status \
	-H "Content-Type: application/json" \
	-d '{"status":"ACTIVE"}'
```

## Notes for later implementation

- Backend currently stores strip updates in memory only.
- Restarting the backend resets data from `backend/data/mockStrips.json`.
- This is intentional for quick UI and API prototyping before production integration.
