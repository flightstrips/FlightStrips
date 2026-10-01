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

Docker Desktop with the Linux engine, Go with the automatic 1.25.7 toolchain,
Node/npm, Python 3, and CMake/Visual Studio 2022 C++ Win32 tools. See the
[Windows development instructions](backend/Readme.md) for configuration paths,
frontend/plugin builds, native backend development and test commands.

### Running the Backend

From the repository root in PowerShell, build locally and start three pinned
NATS nodes, bootstrap resources separately, then start two complete backends:

```powershell
.\backend\local.ps1 init
.\backend\local.ps1 build
.\backend\local.ps1 brokers
.\backend\local.ps1 bootstrap
.\backend\local.ps1 start
```

The two backends listen on localhost:8090 and :8091. PostgreSQL and Redis are
not required. Preserve the shared effect key and broker volumes across restarts.
This integration is held from release pending local qualification.

### Running the Frontend

See [frontend/README.md](frontend/README.md) for development server setup.

## Key Features

- **WebSocket-based Communication** — Real-time synchronization between frontend, backend, and Euroscope
- **Multi-Session Support** — Multiple sessions can run for the same airport on one backend server
- **Euroscope Integration** — Two-way sync with Euroscope for flight plan management
- **Optimistic Concurrency** — Safe concurrent updates to strip data
- **Hoppies ACARS Integration** — Pilot data connectivity
- **CDM Support** — Collaborative Decision Making features including ECFMP and CTOT
- **Grafana Observability** — Cloud dashboards in `observability/grafana/dashboards/`, with a CPU and work-amplification runbook in [observability/README.md](observability/README.md)

## License

FlightStrips is licensed under the GPL-3.0 License. See [LICENSE](LICENSE) for details.
