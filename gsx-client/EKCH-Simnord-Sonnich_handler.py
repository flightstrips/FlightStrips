# -- coding: utf-8 --
#
# EKCH stand and pushback assignment - GSX airport handler script.
# Arriving: selects the stand the controller assigned. Departing: narrows the
# pushback menu to the route they gave. Python 3.7 (couatl ships python37.dll).
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
#   Set API_BASE and SCENERY below before distributing, and ship this file
#   alongside the .ini. The filename must be the .ini's name plus "_handler":
#
#       EKCH-Simnord-Sonnich.ini  ->  EKCH-Simnord-Sonnich_handler.py
#
#   SCENERY must match the scenery key in the server's gsx_sceneries.json. That
#   is how the server knows to answer "Gate A31" and "Z2 Face E" rather than
#   whatever another add-on calls the same concrete.
#
#   Endpoint: GET /api/gsx/stand?callsign=&icao=&scenery=
#   Answers at most one of the two, because they belong to opposite ends of a
#   turnaround:
#     arriving here  -> { "stand": "Gate A31", "pushback": null }
#     leaving here   -> { "stand": null, "pushback": "Z2 Face E" }
#     nothing to do  -> { "stand": null, "pushback": null }

API_BASE = "https://api.flightstrips.dk"
SCENERY = "Simnord-Sonnich"

POLL_INTERVAL_MS = 30000   # how often to re-check
POLL_LIMIT = 240           # stop after this many polls (~2h at 30s)


# --------------------------------------------------------------------------
# identity and transport
# --------------------------------------------------------------------------

def _standEscape(value):
    """Escape a query value. Callsigns and scenery names only."""
    out = []
    for ch in str(value).strip():
        if ch.isalnum() or ch in "-_.":
            out.append(ch)
        elif ch == " ":
            out.append("%20")
    return "".join(out)


def _standReadCallsign(self):
    """The callsign the pilot filed with.

    FlightStrips keys strips on the VATSIM callsign, so this has to reproduce
    what the pilot connected with. SimBrief is the reliable source: sb.callsign
    is the ATC callsign from the filed plan, and GSX already falls back to
    icao_airline + flight_number internally when the plan omits it.
    """
    try:
        sb = getSimbrief()
        if sb is not None and not sb.last_error and sb.callsign:
            return _standEscape(sb.callsign.upper())
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
        return _standEscape((airline + flight).upper())
    return _standEscape(str(tail).upper())


def _standFetch(self):
    """The current assignment, or None on error.

    etag=True makes GSX send If-None-Match; an unchanged assignment costs a 304
    with no payload.
    """
    if self._standCallsign is None:
        self._standCallsign = _standReadCallsign(self)
    if not self._standCallsign:
        return None

    airport = getAirport()
    icao = airport.icao if airport else ""
    url = "%s/api/gsx/stand?callsign=%s&icao=%s&scenery=%s" % (
        API_BASE, self._standCallsign, icao, _standEscape(SCENERY))
    return fetchJson(url, timeout=8, etag=True)


def _standSay(self, text):
    """showMessage only lands while the GSX menu is open, so log it too."""
    print("[stands] %s" % text)
    showMessage(text)


def _standSame(a, b):
    return str(a).upper().replace(" ", "") == str(b).upper().replace(" ", "")


# --------------------------------------------------------------------------
# applying an assignment
# --------------------------------------------------------------------------

def _standOnAssignedStand(self, stand):
    """True when GSX currently holds the stand the controller assigned."""
    gate = getGate()
    if gate is None:
        return False
    name = (gate.uiGateName or "").upper().replace(" ", "")
    return name.endswith(str(stand).upper().replace(" ", ""))


def _standApply(self, stand):
    """Select the assigned stand.

    selectGate is deferred - it stores the request and returns immediately - so
    the gate only actually changes on the next GSX cycle.
    """
    result = selectGate(stand)

    if isinstance(result, list) and result:
        # Ambiguous name: prefer the parking whose UI name ends with it.
        chosen = result[0]
        for parking in result:
            if _standSame(parking.uiGateName or "", stand):
                chosen = parking
                break
        result = selectGate(chosen)

    if result is True:
        _standSay(self, "Stand %s assigned" % stand)
        return True
    if result is False:
        print("[stands] '%s' refused (parked, or parking services revoked)" % stand)
    elif result is None:
        print("[stands] selectGate error - no airport loaded")
    else:
        print("[stands] no parking matches '%s' in this scenery" % stand)
    return False


def _standApplyPushback(self, wanted):
    """Narrow the pushback menu to the route the controller assigned.

    GSX has no selectPushback(): a script cannot answer the menu on the pilot's
    behalf. What it can do is remove the routes that were not assigned, leaving
    the assigned one as the only routed choice. GSX still offers Straight and
    Pull Straight regardless, so the menu does not disappear.
    """
    gate = getGate()
    if gate is None or not wanted:
        return False

    try:
        # An extra slot defined by the profile: keep only it, and switch both
        # of the default left/right routes off.
        for slot in (gate.pushbackAddPos or []):
            label = slot.get("label") if isinstance(slot, dict) else None
            if label and _standSame(label, wanted):
                gate.pushbackAddPos = [slot]
                gate.pushback = 0
                _standSay(self, "Pushback: %s" % wanted)
                return True

        # Otherwise one of the two defaults. pushbackLabels is left then right,
        # and the direction enum is 1 = left, 2 = right.
        labels = gate.pushbackLabels
        if isinstance(labels, str):
            labels = labels.split("|")
        for index, label in enumerate((labels or [])[:2]):
            if _standSame(label, wanted):
                gate.pushback = 1 if index == 0 else 2
                gate.pushbackAddPos = []
                _standSay(self, "Pushback: %s" % wanted)
                return True
    except Exception as err:
        print("[stands] could not set pushback: %s" % err)
        return False

    print("[stands] '%s' is not a pushback route at this stand" % wanted)
    return False


def _standCheck(self):
    """One fetch-and-apply cycle. Returns False when polling should stop.

    The server sends at most one of the two, because they belong to opposite
    ends of a turnaround: a stand for traffic arriving here, a pushback route
    for traffic leaving. An aircraft that is already parked and loading is never
    told to move.
    """
    if self._standUserOverride:
        return False

    payload = _standFetch(self)
    if payload is None:
        return True                          # transient error - keep polling

    stand = payload.get("stand")
    if stand:
        return _standCheckArrival(self, stand)

    pushback = payload.get("pushback")
    if pushback:
        return _standCheckDeparture(self, pushback)

    return True                              # nothing assigned - keep waiting


def _standCheckArrival(self, stand):
    """Inbound: park on the stand the controller assigned."""
    if _standSame(stand, self._standAssigned or ""):
        return True                          # already applied

    if self._standAssigned is not None:
        _standSay(self, "Stand changed by ATC")
    self._standAssigned = stand
    _standApply(self, stand)
    return True                              # selectGate lands next cycle


def _standCheckDeparture(self, pushback):
    """Outbound: narrow the push menu on the stand we are already parked on."""
    if _standSame(pushback, self._standPushback or ""):
        return True                          # already applied

    if self._standPushback is not None:
        _standSay(self, "Pushback changed by ATC")
    if _standApplyPushback(self, pushback):
        self._standPushback = pushback
    return True


def _standStartPolling(self):
    """Run the check loop in a tasklet so GSX is never blocked.

    truewait uses wall-clock time, so the interval does not stretch with the sim
    rate. Tasklets are killed automatically on airport exit.
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

def _standInit(self):
    if not hasattr(self, "_standUserOverride"):
        self._standUserOverride = False
        self._standAssigned = None
        self._standPushback = None
        self._standPoll = None
        self._standCallsign = None


def onEnterAirport(self):
    """Fires once the airport handler activates: on the ground, at low speed."""
    _standInit(self)

    payload = _standFetch(self)

    # No strip, no assignment, or not on the network. Leave the pilot completely
    # alone: no message, no polling, no change to how GSX behaves.
    if payload is None or (not payload.get("stand") and not payload.get("pushback")):
        return

    stand = payload.get("stand")
    if stand is not None and getGate() is not None and not _standOnAssignedStand(self, stand):
        # Inbound, but already sitting on a stand we did not assign, so the
        # pilot picked it. Leave them alone. This check is for arrivals only:
        # a departing aircraft is always on a stand it chose, and that is
        # exactly where its assigned pushback route applies.
        _standSay(self, "Keeping your stand; ATC updates off")
        self._standUserOverride = True
        return

    _standCheck(self)
    _standStartPolling(self)


def onDepartureRequested(self, *args):
    """Re-apply the pushback route in case it was set after we last polled."""
    _standInit(self)
    if not self._standUserOverride:
        payload = _standFetch(self)
        if payload and payload.get("pushback"):
            _standApplyPushback(self, payload.get("pushback"))
    if hasattr(self, "_super_onDepartureRequested"):
        self._super_onDepartureRequested()


def onGateReset(self, reason):
    """The pilot took control: back off for the rest of this visit.

    Nothing is written back to FlightStrips. The controller's board is the
    source of truth, and a pilot parking elsewhere is a discrepancy they should
    see rather than have the simulator quietly overwrite.
    """
    _standInit(self)
    if reason in ("user_changed", "user_revoked") and (self._standAssigned or self._standPushback):
        self._standUserOverride = True
        cancelAsync(self._standPoll)
        self._standPoll = None
        print("[stands] pilot took over (%s) - ATC updates stopped" % reason)


def onExitAirport(self):
    cancelAsync(self._standPoll)
    self._standPoll = None
    self._standAssigned = None
    self._standPushback = None
    self._standUserOverride = False
    self._standCallsign = None
