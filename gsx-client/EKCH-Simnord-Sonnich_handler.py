# -- coding: utf-8 --
#
# EKCH stand assignment - GSX airport handler script.
# Reads the stand FlightStrips has assigned and selects it in the sim.
# Python 3.7 (couatl ships python37.dll).
#
# ---------------------------------------------------------------------------
# FOR PILOTS: there is nothing to configure. Drop this file next to the .ini
# profile in %APPDATA%\Virtuali\GSX\MSFS\ and fly. If a controller has you on a
# stand it is selected automatically, and it follows you if they move you. If
# you are not on the network, or nobody is controlling, GSX behaves exactly as
# it always did.
# ---------------------------------------------------------------------------
#
# FOR WHOEVER PUBLISHES THE PROFILE
#   Set API_BASE below before distributing, and ship this file alongside the
#   .ini. The filename must be the .ini's name plus "_handler":
#
#       EKCH-Simnord-Sonnich.ini  ->  EKCH-Simnord-Sonnich_handler.py
#
#   That binding is what makes the script airport- and scenery-specific: it
#   loads only for the scenery whose profile is active, so the stand names it
#   resolves are the ones that scenery actually has.
#
#   The endpoint is GET /api/gsx/stand?callsign=&icao= and answers
#   { "stand": "A12", "revision": "A12" } or { "stand": null }.

API_BASE = "https://flightstrips.example.org"

POLL_INTERVAL_MS = 30000   # how often to re-check for a new stand
POLL_LIMIT = 240           # stop after this many polls (~2h at 30s)


# --------------------------------------------------------------------------
# identity
# --------------------------------------------------------------------------

def _standCallsign(self):
    """The callsign this aircraft is flying under.

    FlightStrips keys strips on the VATSIM callsign and holds no registration
    used for matching, so this has to reproduce what the pilot connected with.

    SimBrief is the best source by far: sb.callsign is the ATC callsign from the
    filed plan, which is the same string the pilot gives vPilot. GSX already
    falls back to icao_airline + flight_number internally when the plan omits
    it, so there is nothing to compose here.

    The sim is only the fallback, for pilots flying without a SimBrief plan.
    """
    try:
        sb = getSimbrief()
        if sb is not None and not sb.last_error and sb.callsign:
            return _standClean(sb.callsign)
    except Exception as err:
        print("[stands] SimBrief unavailable: %s" % err)

    try:
        ddef = (SimDataDefinition()
                ("ATC ID", None, DataType.STRING32)
                ("ATC FLIGHT NUMBER", None, DataType.STRING32))
        tail, flight = USER.requestData(ddef)
    except Exception as err:
        print("[stands] could not read aircraft identity: %s" % err)
        return ""

    airline = ""
    try:
        airline = aircraft.icaoAirline or ""
    except Exception:
        pass

    if airline and flight:
        return _standClean(airline + flight)
    return _standClean(tail)


def _standClean(value):
    """Callsigns are alphanumeric; drop anything else rather than escaping it."""
    out = []
    for ch in str(value).strip().upper():
        if ch.isalnum():
            out.append(ch)
    return "".join(out)[:12]


# --------------------------------------------------------------------------
# helpers
# --------------------------------------------------------------------------

def _standInit(self):
    if not hasattr(self, "_standUserOverride"):
        self._standUserOverride = False
        self._standCurrent = None
        self._standPoll = None
        self._standCallsignCache = None


def _standFetch(self):
    """The stand FlightStrips currently holds for us, or None on error.

    etag=True makes GSX send If-None-Match. The server's ETag is the stand
    itself, so a poll that changes nothing costs a 304 with no payload.
    """
    if self._standCallsignCache is None:
        self._standCallsignCache = _standCallsign(self)
    if not self._standCallsignCache:
        return None

    airport = getAirport()
    icao = airport.icao if airport else ""
    url = "%s/api/gsx/stand?callsign=%s&icao=%s" % (API_BASE, self._standCallsignCache, icao)
    return fetchJson(url, timeout=8, etag=True)


def _standSay(self, text):
    """showMessage only lands while the GSX menu is open, so log it too."""
    print("[stands] %s" % text)
    showMessage(text)


def _standMatches(self, gate, stand):
    """True if the stand GSX currently holds is the one FlightStrips assigned."""
    if gate is None or not stand:
        return False
    name = (gate.uiGateName or "").upper().replace(" ", "")
    return name.endswith(stand.upper().replace(" ", ""))


def _standApply(self, stand):
    """Resolve the assigned stand to a parking and select it.

    selectGate is deferred - it stores the request and returns immediately, so
    this is safe to call from a callback or a background tasklet.
    """
    if not stand:
        return False

    result = selectGate(stand)

    if isinstance(result, list):
        # The identifier matched several parkings. Prefer one whose UI name
        # ends with the stand, otherwise take the first.
        wanted = stand.upper().replace(" ", "")
        chosen = result[0]
        for parking in result:
            if (parking.uiGateName or "").upper().replace(" ", "").endswith(wanted):
                chosen = parking
                break
        result = selectGate(chosen)

    if result is True:
        self._standCurrent = stand
        _standSay(self, "Stand %s assigned" % stand)
        return True

    if result is False:
        # Either parked with services running, or the pilot revoked parking.
        print("[stands] '%s' refused (parked, or parking services revoked)" % stand)
    elif result is None:
        print("[stands] selectGate error - no airport loaded")
    else:
        print("[stands] no parking matches '%s' in this scenery" % stand)
    return False


def _standCheck(self):
    """One fetch-and-apply cycle. Returns False when polling should stop."""
    if self._standUserOverride:
        return False

    payload = _standFetch(self)
    if payload is None:
        return True                              # transient error - keep polling

    revision = payload.get("revision")
    if revision == self._standCurrent:
        return True                              # nothing changed

    stand = payload.get("stand")
    if not stand:
        self._standCurrent = revision
        return True                              # no stand held - keep waiting

    if self._standCurrent is not None:
        _standSay(self, "Stand changed by ATC")
    _standApply(self, stand)
    return True


def _standStartPolling(self):
    """Run the check loop in a tasklet so GSX is never blocked.

    truewait uses wall-clock time, so the interval does not stretch with the
    sim rate. Tasklets are killed automatically on airport exit.
    """
    cancelAsync(self._standPoll)

    def loop():
        for _ in range(POLL_LIMIT):
            truewait(POLL_INTERVAL_MS)
            if not _standCheck(self):
                return

    self._standPoll = runAsync(loop)


# --------------------------------------------------------------------------
# GSX lifecycle callbacks
# --------------------------------------------------------------------------

def onEnterAirport(self):
    """Fires once the airport handler activates: on the ground, at low speed."""
    _standInit(self)

    payload = _standFetch(self)

    # No strip, no stand, or offline. Leave the pilot completely alone: no
    # message, no polling, no change to how GSX behaves.
    if payload is None or not payload.get("stand"):
        return

    stand = payload.get("stand")
    gate = getGate()

    if gate is not None:
        if _standMatches(self, gate, stand):
            # Already on the assigned stand. Adopt it and watch for changes.
            self._standCurrent = payload.get("revision")
            _standSay(self, "On assigned stand %s" % stand)
        else:
            # A stand we did not assign. The pilot chose it - leave them alone.
            _standSay(self, "Keeping your stand; ATC updates off")
            self._standUserOverride = True
            return
    else:
        _standApply(self, stand)

    _standStartPolling(self)


def onGateReset(self, reason):
    """The stand assignment is being lost or changed.

    'user_changed' and 'user_revoked' both mean the pilot took control. Back off
    for the rest of this visit. Nothing is written back to FlightStrips: the
    controller's board is the source of truth and a pilot parking elsewhere is
    something they should see and resolve, not something the sim overwrites.
    """
    _standInit(self)
    if reason in ("user_changed", "user_revoked") and self._standCurrent is not None:
        self._standUserOverride = True
        cancelAsync(self._standPoll)
        self._standPoll = None
        print("[stands] pilot took over (%s) - ATC updates stopped" % reason)


def onExitAirport(self):
    cancelAsync(self._standPoll)
    self._standPoll = None
    self._standCurrent = None
    self._standUserOverride = False
    self._standCallsignCache = None
