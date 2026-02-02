# FlightStrips Development Guide

FlightStrips is a realistic vATC management system for VATSIM inspired by the real-world NITOS system. The project consists of three main components: a Go backend, a React frontend, and a C++ EuroScope plugin.

## Project Structure

- **backend/** - Go API server handling WebSocket communication and database operations
- **frontend/** - React/TypeScript web application for controller interface
- **euroscope-plugin/** - C++ plugin for EuroScope integration

## Build, Test & Lint

### Backend (Go)

```bash
cd backend

# Run with Docker Compose (full stack)
docker compose --profile all up --build -d

# Run database only for local development
docker compose --profile database up --build -d

# Run server locally
go run cmd/server/main.go

# Run tests
go test ./...

# Run specific package tests
go test ./internal/pdc -v
```

### Frontend (React/TypeScript)

```bash
cd frontend

# Install dependencies
npm install

# Development server
npm run dev

# Build for production
npm run build

# Lint
npm run lint

# Preview production build
npm run preview
```

Requires Node.js 22.16 (see `engines` in package.json).

### EuroScope Plugin (C++)

Built with CMake using C++20. Requires Visual Studio with Win32 platform support.

```bash
cd euroscope-plugin
# Build using CMake with Win32 platform
```

Plugin is inspired by the [VATSIM-UK Controller Plugin](https://github.com/VATSIM-UK/uk-controller-plugin).

## Architecture

### Communication Protocol

Both frontend and EuroScope communicate with the backend via **WebSocket connections using plain JSON events**. Events are unique between frontend and EuroScope.

**Connection Flow:**
1. Connect to the appropriate WebSocket endpoint
2. Send `token` event within 2 seconds: `{"type": "token", "token": ""}`
3. Backend validates token or disconnects client
4. Backend sends periodic `ping` messages; client must respond with `pong`

See [events.md](../events.md) for complete event specifications.

### Data Model

All data is stored in PostgreSQL and belongs to a **session**. Each session has:
- **Name**: Environment identifier (`LIVE`, `PLAYBACK-xxx`, `Sweatbox`)
- **Airport**: ICAO code (e.g., `EKCH`)

This allows multiple sessions for the same airport on one backend server.

Database queries are generated using **sqlc** from SQL in `queries/`.

#### Optimistic Concurrency

Frontend updates use optimistic concurrency via version numbers to prevent concurrent overwrites when multiple controllers are on the same position.

**Important:** EuroScope is the source of truth and always increments versions. Position updates do NOT increment versions.

#### Session Cleanup

Sessions without EuroScope connections are cleaned up after ~5 minutes of inactivity. This allows old data to persist briefly for controller shift changes.

#### Strip Ordering

Controllers must see strips in the same order. Ordering uses an `INT32` system with 100-position spacing initially:
- Strip A: `0`
- Strip B: `100`  
- Strip C: `200`

Moving Strip C between A and B: `(100 - 0) / 2 = 50`

If spacing exhausts, recalculate all positions with 100-space gaps. With 5-minute session resets, 1000-space gaps provide ample room before hitting `INT32` limits.

### Master Client Selection

Only one EuroScope client per session acts as **master** for high-frequency updates. Non-master clients send limited config events only.

Master is determined by a **priority list based on primary frequency** (not callsign). Example for EKCH:
1. EKCH_A_TWR (118.105)
2. EKCH_D_TWR
3. EKCH_C_TWR
4. EKCH_A_GND
...and so on

### Flight Plan Updates

When the frontend updates a flight plan, the **EuroScope client associated with that user** must perform the update in EuroScope to maintain proper synchronization.

### Multi-Server Support

The architecture is designed for future multi-server support using Redis pub/sub:
- Minimal in-memory state
- Well-structured communication interfaces
- Broadcast capability across all connected clients

Currently single-server, but built with multi-server patterns in mind.

## Frontend State Management

- **Zustand** for global state management
- State stores in `frontend/src/store/`
- WebSocket integration via `frontend/src/api/websocket.ts`
- Airport-specific config (e.g., `store/ekch.ts`)

## Backend Organization

- `cmd/` - Entry points (`server`, `migrate`)
- `internal/` - Private application code
  - `cdm/` - Collaborative Decision Making
  - `euroscope/` - EuroScope WebSocket handlers
  - `frontend/` - Frontend WebSocket handlers
  - `pdc/` - Pre-Departure Clearance
  - `server/` - HTTP server setup
  - `services/` - Business logic
  - `shared/` - Shared utilities
  - `websocket/` - WebSocket infrastructure
- `pkg/` - Public library code (e.g., `constants`)
- `queries/` - SQL queries for sqlc
- `migrations/` - Database migrations

## Key Conventions

- **Two-way sync**: Changes in frontend or EuroScope propagate through backend to all clients
- **Event-driven**: All communication is asynchronous via WebSocket events
- **Session-scoped**: Data isolation per airport/environment session
- **EuroScope as source of truth**: Position, squawk, and strip data from EuroScope overrides frontend changes
- **Frequency-based priorities**: Master client selection uses primary frequency, not callsign

## Integration

### vACDM

The backend acts as the master for vACDM (Virtual Airport Collaborative Decision Making), updating vACDM based on EuroScope events rather than using an ES plugin. Future versions may implement a custom CDM system.

### Hoppies ACARS

Integrated for pre-departure clearance coordination and communication.
