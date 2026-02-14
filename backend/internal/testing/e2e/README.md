# E2E Testing Guide

This directory contains end-to-end tests for the FlightStrips backend using the record/replay test harness with **automatic PostgreSQL test containers**.

## Prerequisites

1. **Docker** - Tests automatically spin up PostgreSQL containers using [testcontainers-go](https://golang.testcontainers.org/)
2. **No manual database setup required** - Each test run creates a fresh database
3. **Recorded sessions** - Sample sessions are in `backend/recordings/`

## Running Tests

### Run all E2E tests

```bash
cd backend
go test -v ./internal/testing/e2e/...
```

### Run a specific test

```bash
cd backend
go test -v ./internal/testing/e2e/... -run TestBasicSync
```

### Run with verbose output

```bash
cd backend
go test -v ./internal/testing/e2e/... -v
```

## How It Works

### Automatic Test Containers

Tests use **testcontainers-go** to automatically:
1. Pull `postgres:16-alpine` Docker image (if needed)
2. Start a PostgreSQL container
3. Run database migrations
4. Connect test server to container
5. Clean up container after tests

**No manual database setup required!** Each test run gets a fresh, isolated database.

## Test Structure

### TestMain
- Sets up TEST_MODE environment
- Initializes configuration
- Starts PostgreSQL test container with migrations
- Starts test server on port 2994
- Runs all tests
- Tears down server and container after completion

### Available Tests

- **TestBasicSync** - Tests that a sync event creates strips in the database
- **TestFrontendActions** - Tests frontend client actions during replay
- **TestRealRecording** - Replays your real recorded EKCH session (if available)

## Writing New Tests

### Basic Test Template

```go
func TestMyScenario(t *testing.T) {
    // Clean database before test
    if err := testServer.CleanupDatabase(); err != nil {
        t.Fatalf("Failed to cleanup database: %v", err)
    }

    helper := NewTestHelper(t, testServer)

    // Run replay
    helper.FastReplay("recordings/my_scenario.json")

    // Wait for database updates
    helper.WaitForDatabaseUpdate()

    // Add assertions
    helper.AssertStripExists(sessionID, "SAS123")
}
```

### Helper Functions

- `helper.FastReplay(file)` - Replay session in fast mode
- `helper.RunReplay(file, speed)` - Replay at specific speed multiplier
- `helper.WaitForDatabaseUpdate()` - Wait 200ms for async database updates
- `helper.AssertStripExists(sessionID, callsign)` - Verify strip exists (TODO)
- `helper.AssertStripCount(sessionID, count)` - Verify strip count (TODO)
- `helper.AssertControllerOnline(sessionID, callsign)` - Verify controller (TODO)

## Limitations

- Server runs on fixed port 2994
- Database assertion helpers are placeholder (need query methods)
- Tests run sequentially (server is shared)
- Each test run creates a new database container (isolated and clean)

## CI/CD Integration

Add to GitHub Actions workflow:

```yaml
- name: Run E2E Tests
  run: |
    cd backend
    export TEST_MODE=true
    go test -v ./internal/testing/e2e/...
```

**No database setup required in CI!** Test containers work automatically in any environment with Docker.

## Creating Test Scenarios

1. **Record a session**:
   ```bash
   cd backend
   export RECORD_MODE=true
   export TEST_MODE=true
   go run cmd/server/main.go
   ```

2. **Connect EuroScope** and perform the actions you want to test

3. **Find the recording** in `backend/recordings/`

4. **Add frontend actions** (optional) to test two-way interactions

5. **Create a test** using the recording file

## Troubleshooting

### Port already in use
If port 2994 is already in use, stop the development server or change the port in `e2e/server.go`.

### Docker not available
Ensure Docker Desktop is running. Test containers require Docker to be accessible.

### Test timeouts
Increase the timeout in the test context or use faster replay speeds. Container startup typically takes 2-3 seconds.

### Config file errors
Tests change directory to `backend/` root to find config files. Ensure you run tests from the `backend/` directory.

### Container cleanup
Test containers are automatically cleaned up after tests. If you see orphaned containers, they will be cleaned up by the Ryuk reaper container.
