# E2E Testing Infrastructure

This directory contains a comprehensive end-to-end testing system for FlightStrips, including recording/replay capabilities, frontend simulation, and message validation.

## Overview

The E2E testing system provides:
1. **Recording** - Capture real EuroScope sessions to JSON files with precise timing
2. **Replay** - Execute recorded sessions against the backend with configurable speed
3. **Frontend Simulation** - Simulate frontend client actions (strip updates, clearances)
4. **Message Validation** - Verify message flows between EuroScope and frontend clients
5. **E2E Test Suite** - Automated integration tests with database verification

## Quick Start - Running E2E Tests

```bash
cd backend
go test ./internal/testing/e2e/... -v
```

All tests use testcontainers to spin up isolated PostgreSQL databases, so no manual setup is required.

## Features

### 1. Recording & Replay

1. **Ensure you have a recorded session:**
   ```bash
   cd backend
   go run cmd/replay/main.go -list
   ```

2. **Start the backend server in TEST_MODE:**
   ```bash
   # Set TEST_MODE=true to bypass authentication
   $env:TEST_MODE="true"
   $env:ENV="development"
   go run cmd/server/main.go
   ```

3. **Replay the session:**
   ```bash
   # Real-time replay (1x speed)
   go run cmd/replay/main.go -session recordings/EKCH_LIVE_20260214_123456.json
   
   # Fast replay (10x speed)
   go run cmd/replay/main.go -session recordings/EKCH_LIVE_20260214_123456.json -speed 10.0
   
   # Instant replay (minimal delays)
   go run cmd/replay/main.go -session recordings/EKCH_LIVE_20260214_123456.json -mode fast
   ```

**Recording a Session:**

```bash
# Start server with recording enabled
TEST_MODE=true RECORD_MODE=true go run cmd/server/main.go

# Connect with EuroScope
# Sessions are auto-recorded to recordings/ directory
# Format: EKCH_LIVE_20260214_123456.json
```

**Replaying a Session:**

```bash
# Real-time replay (1x speed)
go run cmd/replay/main.go -session recordings/EKCH_LIVE_20260214.json

# Fast replay (10x speed)
go run cmd/replay/main.go -session recordings/EKCH_LIVE_20260214.json -speed 10.0

# Instant replay (minimal delays)
go run cmd/replay/main.go -session recordings/EKCH_LIVE_20260214.json -mode fast
```

**CLI Options:**

```
-session string          Path to recorded session JSON file (required)
-mode string            Replay mode: 'time' or 'fast' (default: "time")
-speed float            Speed multiplier for time-based mode (default: 1.0)
-server string          WebSocket server URL (default: "ws://localhost:8080/ws/euroscope")
-min-delay int          Minimum delay between events in fast mode (ms) (default: 10)
-stop-on-error          Stop replay on first error (default: true)
-verbose                Enable verbose logging
-list                   List available recorded sessions
-recordings-path string Path to recordings directory (default: "recordings")
```

**Replay Modes:**

**Time-Based Mode (`-mode time`):**
- Respects original event timing
- Configurable speed multiplier (1x to 100x or more)
- Example: 30-minute session at 10x = 3 minutes

**Fast Mode (`-mode fast`):**
- Sends events as quickly as possible
- Minimal delay between events (default 10ms)
- Useful for quick smoke tests

### 2. Frontend Simulation

Recordings can include frontend actions that simulate user interactions:

```json
{
  "frontend_actions": [
    {
      "after_event_index": 0,
      "delay_ms": 1000,
      "action": "update_strip",
      "callsign": "SAS123",
      "updates": {
        "sid": "VENER2A",
        "runway": "22L"
      }
    },
    {
      "after_event_index": 0,
      "delay_ms": 2000,
      "action": "update_field",
      "callsign": "SAS123",
      "params": {
        "field": "cleared_altitude",
        "value": 5000
      }
    }
  ]
}
```

The replayer automatically:
- Connects a frontend WebSocket client with the same CID as the EuroScope client
- Executes frontend actions at specified times
- Validates two-way communication (frontend updates → backend → EuroScope events)

### 3. Message Validation

Tests can capture and verify messages received by clients:

```go
// In E2E tests
h := e2e.NewTestHelper(t)
replayer := h.FastReplay("test_recording.json")

// Verify EuroScope received specific events
h.AssertEuroscopeReceivedMessage(replayer, "sid")
h.AssertEuroscopeReceivedMessage(replayer, "cleared_altitude")

// Verify frontend received updates
h.AssertFrontendReceivedMessage(replayer, "initial")
h.AssertFrontendReceivedMessage(replayer, "strip_update")

// Dump all messages for debugging
h.DumpEuroscopeMessages(replayer)
h.DumpFrontendMessages(replayer)
```

### 4. E2E Test Suite

Automated tests in `internal/testing/e2e/`:

**TestBasicSync** - Verifies basic sync and session creation
**TestFrontendActions** - Tests frontend→backend→EuroScope communication  
**TestComprehensiveFieldUpdates** - Validates all field types (SID, route, altitude, heading, stand)
**TestRealRecording** - Full integration test with real recorded data (1618 events)

All tests:
- Use testcontainers for isolated PostgreSQL instances
- Verify database state after replay
- Check message flows between clients
- Test with realistic timing and data

## Recording Format

Recorded sessions are saved as JSON:

```json
{
  "version": "1.0",
  "metadata": {
    "airport": "EKCH",
    "connection": "LIVE",
    "recorded_at": "2026-02-14T11:00:00Z",
    "duration_seconds": 1800,
    "description": "Auto-recorded session"
  },
  "events": [
    {
      "index": 0,
      "timestamp_ms": 0,
      "type": "token",
      "payload": {
        "type": "token",
        "token": "__TEST_TOKEN__"
      }
    },
    {
      "index": 1,
      "timestamp_ms": 150,
      "type": "login",
      "payload": { ... }
    }
  ],
  "assertions": [],
  "frontend_actions": []
}
```

## Key Technical Details

**Session Association:**
- Frontend clients start with session=-1 (waiting for EuroScope)
- When EuroScope logs in, `CidOnline()` associates frontend clients with the session
- Both clients must use the same CID (via `__TEST_TOKEN__`)

**Timing Issues:**
- Replayer waits 2 seconds after all events complete to ensure frontend actions finish
- This prevents EuroScope from disconnecting before field updates are delivered

**Route Computation:**
- Sessions must have active runways set for route computation to work
- Recordings should include a runway event before the sync event
- Example: `{"type": "runway", "runways": [{"name": "04L", "departure": true, "arrival": true}]}`

**Event Name Mapping (Frontend → EuroScope):**
- `altitude` → `cleared_altitude`
- `sid` → `sid`
- `route` → `route`
- `heading` → `heading`
- `stand` → `stand`

## Environment Variables

- `TEST_MODE=true` - Bypass authentication (required for recording/replay)
- `RECORD_MODE=true` - Enable automatic recording of EuroScope sessions
- `RECORDING_PATH=recordings` - Directory for recorded sessions

⚠️ **Security:** The backend panics if `TEST_MODE=true` and `ENV=production`

## Architecture

```
┌──────────────────┐
│ Recording File   │  Contains: events, frontend_actions
│   (.json)        │
└────────┬─────────┘
         │
         ▼
  ┌─────────────────┐
  │    Replayer     │  Orchestrates replay
  │                 │  - Sends login event
  │                 │  - Replays recorded events
  │                 │  - Executes frontend actions
  └──┬──────────┬───┘
     │          │
     │          └──────────┐
     ▼                     ▼
┌──────────┐        ┌─────────────┐
│ ES Client│        │ FE Client   │  Both clients connect
│ (replay) │        │ (simulate)  │  to backend WebSocket
└────┬─────┘        └──────┬──────┘
     │                     │
     └──────────┬──────────┘
                ▼
        ┌───────────────┐
        │    Backend    │  Real server instance
        │   + Database  │  (testcontainers)
        └───────────────┘
```

## Directory Structure

```
internal/testing/
├── README.md                    # This file
├── recorder/                    # Recording infrastructure
│   ├── recorder.go             # Session recorder
│   └── types.go                # Recording data structures
├── replay/                      # Replay engine
│   ├── replayer.go             # Main replayer orchestration
│   ├── client.go               # EuroScope WebSocket client
│   ├── loader.go               # Session file loader/validator
│   └── errors.go               # Replay-specific errors
├── frontend/                    # Frontend simulation
│   └── client.go               # Frontend WebSocket client
└── assertions/                  # Assertion engine (unused)
    ├── e2e_test.go             # Integration tests
    ├── helpers.go              # Test helpers and assertions
    └── setup.go                # Test infrastructure setup
```

## Contributing

When adding new event types:
1. Ensure they're captured during recording
2. Add test coverage in E2E tests
3. Verify message validation works for both directions

## Troubleshooting

**Tests failing with "Could not compute route"?**
- Check that recordings include runway events
- Verify runway event comes before sync event
- Runways must match those configured in config/ekch.yaml

**"Event sequence broken" errors?**
- Event indices must be sequential (0, 1, 2, ...)
- Timestamps must be monotonically increasing
- Reindex after manually editing recordings

**Frontend actions not executing?**
- Ensure frontend client has same CID as EuroScope client
- Check that replayer waits long enough after replay (2s default)
- Verify session association with debug logging

**Message validation failing?**
- Check event name mapping (altitude → cleared_altitude)
- Verify both clients are receiving messages
- Use DumpMessages() helpers to see actual messages received
