# FlightStrips

FlightStrips is a VATSIM air traffic control management system that provides controllers with a digital flight strip interface, inspired by real-world systems. It features a web-based interface, Euroscope plugin integration, and Hoppies ACARS connectivity for realistic ATC operations.

## Architecture

FlightStrips is a full-stack application with three main components:

### Backend (`backend/`)
- **Language**: Go
- **Database**: PostgreSQL
- **Communication**: WebSocket (JSON-based events)
- **Purpose**: Manages strip data, controller sessions, and synchronization between frontend and Euroscope

### Frontend (`frontend/`)
- **Stack**: React + TypeScript + Vite
- **Purpose**: Web-based interface for controllers to manage flight strips and operations

### Euroscope Plugin (`euroscope-plugin/`)
- **Language**: C++
- **Purpose**: Integration with Euroscope for two-way synchronization of flight plan data and strip information

## Documentation

### Getting Started with the Docs

- **[Backend Architecture](backend/Architecture.md)** — Database design, WebSocket communication, session management, and multi-server considerations
- **[Events Specification](events.md)** — Complete specification of all WebSocket events for frontend and Euroscope communication
- **[Backend Setup](backend/Readme.md)** — Instructions for running the backend API
- **[Frontend Setup](frontend/README.md)** — React/TypeScript development setup
- **[Euroscope Plugin](euroscope-plugin/README.md)** — Plugin development and integration notes

### Quick Reference

| Component | Purpose | Docs |
|-----------|---------|------|
| Backend API | WebSocket server, data management, event routing | [backend/Architecture.md](backend/Architecture.md) |
| Frontend UI | ATC controller interface | [frontend/README.md](frontend/README.md) |
| Euroscope Plugin | Real-time data sync with Euroscope | [euroscope-plugin/README.md](euroscope-plugin/README.md) |
| Events Protocol | Communication specification | [events.md](events.md) |

## Local Development

### Prerequisites
- Docker and Docker Compose
- Go 1.21+ (for backend development)
- Node.js 18+ (for frontend development)
- CMake (for Euroscope plugin)

### Running the Backend

Start only the database:
```sh
docker compose --profile database up --build -d
```

Or start the complete stack:
```sh
docker compose --profile all up --build -d
```

### Running the Frontend

See [frontend/README.md](frontend/README.md) for development server setup.

## Key Features

- **WebSocket-based Communication** — Real-time synchronization between frontend, backend, and Euroscope
- **Multi-Session Support** — Multiple sessions can run for the same airport on one backend server
- **Euroscope Integration** — Two-way sync with Euroscope for flight plan management
- **Optimistic Concurrency** — Safe concurrent updates to strip data
- **Hoppies ACARS Integration** — Pilot data connectivity
- **CDM Support** — Collaborative Decision Making features including ECFMP and CTOT
- **Grafana Observability** — Cloud dashboards in `observability/grafana/dashboards/`

## License

FlightStrips is licensed under the GPL-3.0 License. See [LICENSE](LICENSE) for details.
