---
title: Authentication
description: Resolving port conflicts that prevent EuroScope plugin authentication.
sidebar:
  order: 1
---

The EuroScope plugin tries local ports **27015**, **32015**, **37015**, **42015**, and **47015**, in that order, when logging in. They are deliberately spread apart so a reserved port range is less likely to block every fallback.

Register these callback URLs in Auth0:

- `http://127.0.0.1:27015/callback-auth0`
- `http://127.0.0.1:32015/callback-auth0`
- `http://127.0.0.1:37015/callback-auth0`
- `http://127.0.0.1:42015/callback-auth0`
- `http://127.0.0.1:47015/callback-auth0`

## What's happening

The EuroScope plugin creates a local HTTP server to handle the OAuth callback after you authenticate. Login fails only when all configured callback ports are in use.

## Resolve port conflicts

### 1. Check what's using the callback ports

Open PowerShell and run:

```powershell
netstat -ano | findstr /C:":27015" /C:":32015" /C:":37015" /C:":42015" /C:":47015"
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

If you continue to see the error, verify that at least one callback port is clear by running the netstat command again.

