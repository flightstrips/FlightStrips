# Mock stand feed

A stand-in for `GET /api/gsx/stand` so the handler script can be flown without a
controller on the other end. You edit a JSON file, the handler picks it up on its
next poll, and you play both seats.

```bash
node gsx-client/mock/server.js
```

Or, on Windows, a keyboard-driven version for **Gate D2 at EKCH**. Park
there with either EKCH Simnord profile active before you start: the mock
doesn't know which stand you are on. `1` to `4` assign S1, S2, R1 or S3, each
sent as the real server sends it: the bare point and
its position from `backend/config/ekch/gsx_sceneries.json`. So this tests the
position matching every profile relies on. `R` resets to nothing assigned,
and the screen lists recent polls. Edit `$Points` at the top of the script
to change the options.

```powershell
powershell -ExecutionPolicy Bypass -File gsx-client\mock\api-tui.ps1
```

Then point the handler at it. Edit the **deployed** copy, not the one in this
repo, so an ngrok URL never reaches a commit:

```
%APPDATA%\Virtuali\GSX\MSFS\EKCH-Simnord-Sonnich_handler.py
```

```python
API_BASE = "http://localhost:8787"
```

`F5` in the GSX Handler Editor reloads the script without restarting the sim.

## Do you actually need ngrok?

The handler runs inside couatl on the same machine as the sim, so `localhost`
reaches the server directly and no tunnel is involved. Use ngrok when the sim is
on another machine, or when you want to hand the URL to someone else:

```bash
ngrok http 8787
```

```python
API_BASE = "https://<your-subdomain>.ngrok-free.app"
```

If the handler starts logging JSON parse errors against an ngrok URL, you have
hit the free plan's browser interstitial. It is skipped for non-browser clients,
and couatl fetches over libcurl, so this should not happen — but that is what it
looks like when it does.

## Playing the controller

`assignment.json` is read on every request, so saving it is the whole gesture:
the change reaches the handler within five seconds. `examples.json` holds one
payload per situation worth testing — copy one across.

Gate A32 in `EKCH-Simnord-Sonnich` offers `J1 Face S`, `Z2 Face E` and
`Z3 Face W`, which is enough to exercise every path. Other stands are in
[`backend/config/ekch/gsx_sceneries.json`](../../backend/config/ekch/gsx_sceneries.json)
under `available`.

The sequence that matters, and the one this whole change exists for:

1. Start with `pushback: null` — nothing assigned.
2. Spawn cold on A32 and select **Prepare for Pushback and Departure** before
   assigning anything. The log should say `added the ATC Assigned entry`.
3. While the tug drives out, set `pushback` to `["Z2 Face E"]` and save. The
   stand display should read `PUSH Z2 FACE E`.
4. When GSX asks which way to push, the menu should hold every scenery route
   plus `ATC Assigned: Z2 Face E`, and the message should name the point.
5. Pick it. The push should go to Z2 facing east.
6. Fly it again, this time changing to `["Z3 Face W"]` with the menu up.
   Expect `ATC changed your push to Z3 Face W - pick "ATC Assigned: Z2 Face E",
   it now goes to Z3 Face W`. The menu keeps the name it was built with; that
   is expected, not a failure. Keep the menu open, pick the entry, and check
   the push goes to Z3.
7. Change it once more while the aircraft is moving: the entry must not move,
   and the message should say `too late for this tug`.

Answered at D2 on 2026-11-19: the menu keeps the label it was built with, but
an entry moved after state 5 (by rewriting the whole list) is where the tug
goes. A dict changed in place goes nowhere: reads of `pushbackAddPos` are
copies.

Watch `%APPDATA%\Virtuali\Couatl.log` for the `[stands]` lines, or the Handler
Editor's output panel. `GSX departure state` is logged on every transition.

## Contract

Five fields, and the handler reads the first four. The real server no longer
sends `pushback`; the mock still accepts it, for testing older behaviour. A stand and a pushback are
never both set — a stand is where arriving traffic parks, a pushback point is
how departing traffic leaves the stand it is on.

| Field | Meaning |
| --- | --- |
| `stand` | The parking to select. Arrivals only. |
| `pushback` | Route labels to look for, as a list. Departures only. The handler falls back to them when there is no `target`. |
| `point` | The bare release point, such as `S1`, or a direction, such as `W`. Every departure. |
| `target` | Where the push should end: `lat`, `lon`, `heading` and `radius_m`, plus `avoid` for a direction. The handler picks the route ending there, by position. Without it, the handler falls back to the route names. |
| `revision` | What the real endpoint puts in its `ETag`. The mock sends no `ETag`, so this is only along for the ride. |

The mock answers `503` for malformed JSON, which is what the handler is built to
ride out — a half-saved edit costs one skipped poll rather than a stall.

## Tests

`suite.py` exercises the handler against a stub of EKCH B8 — the real labels
from the `.ini`, double spaces and all. It needs Python 3.7 (couatl's version),
which Docker is the easiest way to get:

```bash
docker run --rm -v "$PWD/gsx-client:/w" -w /w python:3.7-alpine \
  python mock/suite.py EKCH-Simnord-Sonnich_handler.py
```

The case that matters most: **marking must never write `pushback` or
`pushbackAddPos`.** Those two writes leave the tug with no computable path, so
GSX reaches "release parking brakes" and never moves. The suite asserts the
bitmask, the extras list identity and both geometry pairs are untouched after
every mark.
