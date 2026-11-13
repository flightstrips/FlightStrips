# FSRADIS

FSRADIS is a standalone, local-only prototype project that can later be integrated into FlightStrips.

It contains a React + TypeScript frontend that pulls live flight plan data directly from the [VATSIM data feed](https://data.vatsim.net/v3/vatsim-data.json).

## Why this is safe for FlightStrips

- It lives in its own folder: `FSRADIS/`
- It has separate dependencies and scripts
- No imports or runtime coupling with existing FlightStrips services

## Project structure

- `frontend/` - Vite + React + TypeScript UI

## Quick start

```bash
cd frontend
npm install
npm run dev
```

Open the frontend URL shown by Vite (usually `http://localhost:5173`).

## VATSIM data

- Flight plans for `EKYT` arrivals/departures are pulled from `https://data.vatsim.net/v3/vatsim-data.json`.
- The feed is polled every 15 seconds from the browser.
- If the feed is temporarily unavailable, the previously loaded strips are kept until the next successful poll.
