# FlightStrips

**Shared electronic flight strips for VATSIM air traffic control.**

FlightStrips gives controllers a shared browser-based strip board that stays in sync with EuroScope. Each strip shows the flight plan and the details controllers need as an aircraft moves through the airport, including its stand, runway, clearance status, and the controller currently working it. Updates are shared across connected positions in real time.

Developed by VATSIM Scandinavia for Copenhagen Airport (EKCH), FlightStrips draws on the real-world NITOS electronic strip system. Each position has a layout suited to its work. Clearance Delivery can clear a departure, Apron can take the strip for pushback and taxi, and Tower can follow it through takeoff.

[Open the web app](https://flightstrips.dk/app) · [Controller documentation](https://docs.flightstrips.dk/getting-started/intro/) · [Plugin releases](https://github.com/flightstrips/FlightStrips/releases)

## What controllers can do

- **Organize arrivals and departures.** Move strips between bays, record clearances, and view stand and runway assignments.
- **Coordinate handoffs.** See who owns each strip, transfer it to the next controller, and track pending coordination. Supported flight plan edits and tracking actions sync back to EuroScope. [Ownership and handoffs](https://docs.flightstrips.dk/concepts/ownership/)
- **Handle pre-departure clearances.** Issue and track digital clearances through the web or Hoppie's ACARS. Requests that need controller review are flagged on the board. [Pre-departure clearance guide](https://docs.flightstrips.dk/concepts/pre-departure-clearance/)
- **Manage departure flow.** Use Airport Collaborative Decision Making (A-CDM) to track target off-block and start-up times, calculated take-off times (CTOTs), and applicable ECFMP restrictions.
- **Sequence arrivals.** Use the arrival manager (AMAN) to review the landing sequence and see time-to-gain or time-to-lose guidance in EuroScope at supported positions.
- **Call other controllers.** Use the optional VACS integration from the EuroScope panel or the strip board, including when the board is open on a separate laptop or tablet. [VACS setup](https://docs.flightstrips.dk/reference/vacs/)

The project also includes a [pilot-facing electronic flight bag](https://flightstrips.dk/efb) with flight information, ATIS, charts, and access to digital pre-departure clearances where available.

## Get started as a controller

1. Load the FlightStrips plugin and connect EuroScope on the position and primary frequency you intend to work. The [EuroScope setup guide](https://docs.flightstrips.dk/getting-started/es-plugin/) covers sector-file packages and manual installation.
2. Open the FlightStrips panel with `.fs open`, sign in with your VATSIM account, and wait for the panel to show **Connected**.
3. Select **Open App**, sign in with the same VATSIM account, and check your position, layout, and runway setup. [First-session checklist](https://docs.flightstrips.dk/getting-started/first-session/)

## How it works

| Component | What it does |
| --- | --- |
| **EuroScope plugin** (`euroscope-plugin/`, C++) | Sends your controller position and live flight data to the backend and applies supported changes in EuroScope. [Plugin notes](euroscope-plugin/README.md) |
| **Backend** (`backend/`, Go and PostgreSQL) | Manages controller sessions, stores shared strip state, and broadcasts changes to connected clients over WebSockets. [Backend architecture](backend/Architecture.md) |
| **Web app** (`frontend/`, React, TypeScript, and Vite) | Displays the strip board for your controller position and sends your actions to the backend. [Frontend notes](frontend/README.md) |

The backend matches the web app to your EuroScope connection through your VATSIM account. Separate sessions keep live traffic, Sweatbox training, and playback independent. Version checks protect against overwrites when controllers edit the same strip concurrently.

## Local development

Use Docker with Compose for local backend services. For native builds, check the Go version in [`backend/go.mod`](backend/go.mod), the Node.js requirement in [`frontend/package.json`](frontend/package.json), and the Windows x86 plugin build instructions in the [local development guide](https://docs.flightstrips.dk/dev/local-dev/).

From the repository root, start PostgreSQL and the backend API. Compose also runs the database migrations.

```sh
cd backend
docker compose --profile all up --build -d
```

Use `docker compose --profile database up --build -d` from `backend/` to run PostgreSQL and the migrations while running the Go API locally. See the [backend setup notes](backend/Readme.md).

Start the frontend development server in a separate terminal from the repository root.

```sh
cd frontend
npm ci
npm run dev
```

### Developer references

- [WebSocket events and EuroScope protocol](events.md)
- [Text pilot support specification](.github/specs/text-pilot-support.md)
- Multi-airport architecture proposal ([HTML with diagrams](.github/specs/multi-airport-support/architecture-proposal.html) · [Markdown source](.github/specs/multi-airport-support/architecture-proposal.md) · [Implementation epic plan](.github/specs/multi-airport-support/epic-plan.md))
- [Grafana dashboards and observability runbook](observability/README.md)
- [GitHub Issues](https://github.com/flightstrips/FlightStrips/issues) · [Commit message conventions](docs/commit-messages.md)

## License

FlightStrips is licensed under the GNU General Public License v3.0. See [LICENSE](LICENSE).
