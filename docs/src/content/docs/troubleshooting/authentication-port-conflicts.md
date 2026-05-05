---
title: Authentication
description: Resolving port conflicts that prevent EuroScope plugin authentication.
sidebar:
  order: 1
---

If you see the error **"Authentication redirect listener failed to start on port 27015"** when logging in via the EuroScope plugin, another application is blocking the authentication port.

## What's happening

The EuroScope plugin creates a local HTTP server on port 27015 to handle the OAuth callback after you authenticate. If this port is already in use, the login process fails.

## Resolve port conflicts

### 1. Check what's using port 27015

Open PowerShell and run:

```powershell
netstat -ano | findstr :27015
```

This shows all processes using that port. Note the **PID** (Process ID) number.

### 2. Identify the process

To see what process owns that PID, run:

```powershell
Get-Process -Id <PID> | Select-Object Name, ProcessName, Path
```

Replace `<PID>` with the number from the previous command.

### 3. Common culprits

**iTunes or Apple services:** iTunes and related Apple services frequently bind to this port range. Diable or close them.s

**Another EuroScope instance:** If you have a copy of EuroScope still running in the background, close it completely.

**Other applications:** Some games, media players, or development tools may use this port.

### 4. Fix the conflict

Choose one approach:

#### Option A: Close the conflicting application

If it's iTunes, close the application entirely or disable the "Device Sync" feature in settings.

If it's another EuroScope instance, fully close it and restart.

#### Option B: Restart your computer

A restart releases all port bindings and often resolves transient conflicts.


## Verify the fix

After resolving the port conflict, try logging in again. The authentication should complete without errors.

If you continue to see the error, verify port 27015 is clear by running the netstat command again.

