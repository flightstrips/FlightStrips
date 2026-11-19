# -- coding: utf-8 --
#
# EKCH stand and pushback assignment - GSX airport handler script.
# Arriving: selects the stand the controller assigned. Departing: marks the
# pushback route they gave. Python 3.7 (couatl ships python37.dll).
#
# ---------------------------------------------------------------------------
# FOR PILOTS: nothing to configure. Drop this file next to the .ini profile in
# %APPDATA%\Virtuali\GSX\MSFS\ and fly.
#
# On arrival your assigned stand is selected for you, and it follows you if the
# controller moves you.
#
# On departure:
#   1. Select "Prepare for Pushback and Departure" as usual.
#   2. Wait for the tug. Once it is connected GSX asks which way to push.
#   3. Pick the "ATC Assigned" entry. It names the point, for example
#      "ATC Assigned: Z2 Face E", and goes wherever ATC put you.
# The scenery's own routes are all still there; this is one more, not a
# replacement. The assigned point is also on the stand display (PUSH Z2 FACE E)
# and in a GSX message when the route menu opens.
#
# You need not wait for your clearance before calling the tug. GSX does not ask
# which way to push until the tug is connected, a minute or two later, and
# anything assigned in the meantime still reaches the menu. If the controller
# changes your point after that, "ATC Assigned" follows them - no cancelling.
#
# Not on the network, or nobody controlling? No messages and no stand changes.
# The pushback menu still gains a plain "ATC Assigned" entry when you request
# pushback; with nothing assigned it goes where the scenery's first route goes,
# and all the scenery's own routes are still there.
# ---------------------------------------------------------------------------
#
# FOR WHOEVER PUBLISHES THE PROFILE
#   Set API_BASE and SCENERY below, and ship this file alongside the .ini. The
#   filename must be the .ini's name plus "_handler":
#
#       EKCH-Simnord-Sonnich.ini  ->  EKCH-Simnord-Sonnich_handler.py
#
#   SCENERY must match the scenery key in the server's gsx_sceneries.json, which
#   is how the server knows to answer "Gate A31" and "Z2 Face E" rather than
#   whatever another add-on calls the same concrete.
#
#   Endpoint: GET /api/gsx/stand?callsign=&icao=&scenery=
#     arriving here  -> { "stand": "Gate A31", "pushback": null }
#     leaving here   -> { "stand": null, "pushback": ["Z2 Face E"] }
#     nothing to do  -> { "stand": null, "pushback": null }

API_BASE = "https://api.flightstrips.dk"
SCENERY = "Simnord-Sonnich"

POLL_INTERVAL_MS = 5000    # how often to re-check
POLL_LIMIT = 1440          # stop after this many polls (~2h at 5s)
SHOW_ON_VDGS = True        # also show the assigned push on the stand display

# What the assigned route is called in the menu. The menu is narrowed to it, so
# a bare name is enough - and it stays true if the point is changed later, since
# it never named a place to begin with. When ATC gives two facings of the same
# taxiway both are offered, and then they have to be told apart, so the second
# form adds the scenery's own name. An empty MARK_LABEL leaves the names alone.
MARK_LABEL = "ATC Assigned"
MARK_LABEL_MANY = "ATC: %s"

# Add one extra entry to every stand's pushback menu, called this, going where
# ATC said. The scenery's own routes stay on offer alongside it. This is the
# mode to ship; it supersedes SHOW_ONLY_ASSIGNED, which is ignored while it is
# on. See _standApplyAtcEntry.
ATC_ENTRY = True
ATC_ENTRY_LABEL = "ATC Assigned"
# The entry's name once a point is assigned, so the pilot can see in the menu
# where it goes. Must start with ATC_ENTRY_LABEL: that prefix is how the script
# tells its own entry from the scenery's routes.
ATC_ENTRY_LABEL_POINT = "ATC Assigned: %s"

# Offer only the assigned route, then put the scenery's own routes back once GSX
# has built the menu from it. Narrowing and leaving it narrowed is what breaks
# the push; narrowing and restoring at departure state 5 drives normally. Turn
# off to leave the whole menu on offer with the assigned route merely marked.
SHOW_ONLY_ASSIGNED = True

# When the point changes after the menu is built, move where the route the pilot
# already picked leads, rather than making them cancel and start again. See
# _standRedirect.
REDIRECT_LATE = True

# Tell a pilot who calls the tug that no pushback is assigned yet.
#
# Off, because the feed cannot tell "on a strip, nothing assigned" from "nobody
# is controlling": it answers both with an empty payload, on purpose, so that it
# never discloses whether a callsign is known to FlightStrips. A pilot flying
# offline would otherwise be promised a mark that is never coming.
ANNOUNCE_WAITING = False

# Movement that means the push has started, in degrees, so no trigonometry is
# needed: 0.000027 deg of latitude is about 3 m, and the same figure in
# longitude is under 2 m this far north. Anything in the 1-5 m range does; the
# point is only to separate "parked with a tug attached" from "rolling".
MOVED_DEGREES = 0.000027

# FSDT_GSX_DEPARTURE_STATE at which GSX considers pushback requested, and the
# value by which it has built the route menu. See _standPushStage.
DEPARTURE_REQUESTED_STATE = 4
PUSHBACK_COMMITTED_STATE = 5

# Push stages: whether a change can still reach the menu.
PUSH_OPEN = 0        # menu not built yet
PUSH_UNCERTAIN = 1   # it may be built; only the pilot can see
PUSH_ROLLING = 2     # the aircraft is moving

VDGS_ID = "flightstrips_pushback"


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
    """The callsign the pilot filed with, and whether it came from SimBrief.

    Strips are keyed on the VATSIM callsign, so this has to reproduce what the
    pilot connected with. SimBrief is the reliable source. The second return
    value says whether to keep looking: GSX loads the plan a few seconds after
    the handler activates, and a tail number is a confident, non-empty, wrong
    answer that would otherwise be cached for the session.
    """
    try:
        sb = getSimbrief()
        if sb is not None and not sb.last_error and sb.callsign:
            return _standEscape(sb.callsign.upper()), True
    except Exception as err:
        print("[stands] SimBrief unavailable: %s" % err)

    try:
        ddef = (SimDataDefinition()
                ("ATC ID", None, DataType.STRING32)
                ("ATC FLIGHT NUMBER", None, DataType.STRING32))
        tail, flight = USER.requestData(ddef)
    except Exception as err:
        print("[stands] could not read aircraft identity: %s" % err)
        return "", False

    airline = ""
    try:
        airline = aircraft.icaoAirline or ""
    except Exception:
        pass

    if airline and flight:
        return _standEscape((airline + flight).upper()), False
    return _standEscape(str(tail).upper()), False


def _standFetch(self):
    """The current assignment, or None on error. etag=True makes a 304 free."""
    if not self._standCallsignTrusted:
        callsign, trusted = _standReadCallsign(self)
        if callsign and (trusted or not self._standCallsign):
            if callsign != self._standCallsign:
                print("[stands] callsign %s (from %s)"
                      % (callsign, "SimBrief" if trusted else "the sim"))
                self._standAssigned = None
                if ATC_ENTRY and self._standPushback:
                    # The old callsign's point is on the gate and the VDGS.
                    # Take it back, so the new callsign starts clean; its own
                    # point, if any, arrives through the normal path.
                    _standWithdrawAtcEntry(self)
                self._standPushback = None
                self._standPushPoint = None
            self._standCallsign = callsign
            self._standCallsignTrusted = trusted

    if not self._standCallsign:
        if not self._standWarnedNoCallsign:
            print("[stands] no callsign yet (SimBrief not loaded?) - will retry")
            self._standWarnedNoCallsign = True
        return None
    self._standWarnedNoCallsign = False

    airport = getAirport()
    icao = airport.icao if airport else ""
    # rstrip the base: a trailing slash makes this "https://host//api/gsx/stand",
    # a different path to most servers, answered 404 - and a failed fetch looks
    # exactly like "nothing assigned" from the cockpit.
    url = "%s/api/gsx/stand?callsign=%s&icao=%s&scenery=%s" % (
        API_BASE.rstrip("/"), self._standCallsign, icao, _standEscape(SCENERY))
    return fetchJson(url, timeout=8, etag=True)


def _standSay(self, text):
    """showMessage only lands while the GSX menu is open, so log it too."""
    print("[stands] %s" % text)
    try:
        showMessage(text)
    except Exception as err:
        print("[stands] showMessage failed: %s" % err)


def _standVdgs(self, narrow, wide):
    """One message on the stand's docking display, replacing any previous.

    showMessage renders only while the GSX menu is open, so on its own a pilot
    can miss that ATC assigned anything. The VDGS is always in view at a stand
    that has one - 76 of EKCH's 119 in this scenery.
    """
    if not SHOW_ON_VDGS:
        return
    try:
        addVdgsMessage({"id": VDGS_ID, "display": {
            "narrow": {"pages": [{"lines": narrow, "duration": 8000}]},
            "wide": {"pages": [{"lines": wide, "duration": 8000}]}}})
    except Exception as err:
        print("[stands] VDGS message failed: %s" % err)


def _standShowOnVdgs(self, routes):
    if not routes:
        return
    route = str(routes[0]).upper()
    _standVdgs(self, ["PUSH", route[:12]], ["PUSH " + route[:20]])


def _standClearVdgs(self):
    try:
        removeVdgsMessage(VDGS_ID)
    except Exception:
        pass


def _standSame(a, b):
    return str(a).upper().replace(" ", "") == str(b).upper().replace(" ", "")


def _standRoutes(value):
    """Normalise the pushback field to a list of labels.

    One route may arrive as a bare string and several as a list, and a pilot's
    copy of this script is never guaranteed to match the server's version.
    Iterating a string would yield characters and match nothing.
    """
    if not value:
        return []
    if isinstance(value, str):
        return [value]
    return [str(v) for v in value if v]


def _standWantedTest(wanted):
    def isWanted(label):
        return any(_standSame(label, route) for route in wanted)
    return isWanted


# --------------------------------------------------------------------------
# reading the sim
# --------------------------------------------------------------------------

def _standReadVar(code):
    """One RPN read, or None. Used for both A: and L: variables."""
    try:
        return executeCalculatorCode(code)
    except Exception:
        return None


def _standPosition():
    lat = _standReadVar("(A:PLANE LATITUDE, degrees)")
    lon = _standReadVar("(A:PLANE LONGITUDE, degrees)")
    if lat is None or lon is None:
        return None
    try:
        return float(lat), float(lon)
    except (TypeError, ValueError):
        return None


def _standDepartureState(self):
    """FSDT_GSX_DEPARTURE_STATE, or 0. Logged on change; see _standPushStage."""
    value = _standReadVar("(L:FSDT_GSX_DEPARTURE_STATE, number)")
    try:
        state = int(value)
    except (TypeError, ValueError):
        return 0
    if state != self._standLastDepartureState:
        self._standLastDepartureState = state
        print("[stands] GSX departure state %s" % state)
    return state


def _standAnchor(self):
    position = _standPosition()
    if position is not None:
        self._standAnchorPos = position


def _standPushStarted(self):
    """True once the aircraft has moved since the tug was called.

    Position rather than ground speed, because GSX moves a frozen aircraft by
    repositioning it. Once true it stays true.
    """
    if self._standPushStartedLatch:
        return True

    position = _standPosition()
    if position is None:
        # Without a position a parked aircraft cannot be told from a rolling
        # one, and marking simply carries on. That is the right way to fail,
        # but not one to fail at silently.
        if not self._standWarnedNoPosition:
            self._standWarnedNoPosition = True
            print("[stands] cannot read the aircraft position - "
                  "a push already under way will not be detected")
        return False
    if self._standAnchorPos is None:
        # No position could be read when the tug was called. Anchor on the
        # first one that can be, so a push is still caught once it moves on
        # from here rather than never.
        self._standAnchorPos = position
        return False

    if not self._standDepartureRequested:
        # Parked, no tug called. Keep the anchor current so GSX repositioning
        # the aircraft is never later mistaken for the start of a push.
        self._standAnchorPos = position
        return False

    if (abs(position[0] - self._standAnchorPos[0]) > MOVED_DEGREES
            or abs(position[1] - self._standAnchorPos[1]) > MOVED_DEGREES):
        self._standPushStartedLatch = True
        print("[stands] the aircraft has moved - the push is under way")
        return True
    return False


def _standPushStage(self):
    """How far past changing the menu this push has got.

    There is no signal for the moment GSX builds the route menu, and that is
    the moment that matters. Two flights at B8, timed from the request:

        towbarless   state 5 at 1:25, menu at 1:45
        towbar       state 5 at 2:03, menu at 2:03

    So state 5 is at or before the menu, never after. Between it and the menu
    the honest answer is that we do not know whether a change landed, hence
    three stages rather than a boolean: the pilot can see the menu, this cannot.
    """
    if _standPushStarted(self):
        return PUSH_ROLLING
    if self._standPushCommittedLatch:
        return PUSH_UNCERTAIN
    if (self._standDepartureRequested
            and _standDepartureState(self) >= PUSHBACK_COMMITTED_STATE):
        self._standPushCommittedLatch = True
        return PUSH_UNCERTAIN
    return PUSH_OPEN


def _standNoteDeparture(self, source, restart=False):
    """Record that the pushback sequence has begun, and anchor movement here.

    `restart` is the pilot asking again, which is how they recover a push we
    were too late to mark: GSX rebuilds the parking, so the window re-opens and
    a commit latched during the previous attempt has to go.

    The gate snapshot deliberately survives - _standSnapshot notices a rebuild
    on its own, and clearing it here left it unrecoverable while our own label
    was on the gate.
    """
    if self._standDepartureRequested and not restart:
        return
    self._standDepartureRequested = True
    _standResetDeparture(self)
    _standAnchor(self)
    print("[stands] pushback requested (%s)" % source)


def _standResetDeparture(self):
    """Forget the last pushback, so this one starts from nothing.

    Plain assignments, one per field. GSX proxies attribute access on the
    handler object and an attribute is only readable if it was created through
    the normal setter, so writing these straight into __dict__ made every one of
    them unreadable - "'_standMenuRestored' is not accessible from handler
    scripts", thrown from inside the poll, mid-departure.

    This being the only place they are set is what keeps a re-request from
    inheriting the last pushback's state.
    """
    self._standPushStartedLatch = False      # the aircraft has moved
    self._standPushCommittedLatch = False    # GSX has built the route menu
    self._standMenuRestored = False          # ...and handed them back for the push
    self._standMenuRoute = None              # what the menu was built from
    self._standMenuReminded = False          # the point was named as the menu opened
    self._standPresentNote = None            # last read-back mismatch logged


# --------------------------------------------------------------------------
# the pushback menu
#
# Only pushbackLabels may be written. Writing `pushback` or `pushbackAddPos`
# leaves the tug with no computable path - GSX reaches "release parking brakes"
# and never moves, logging "[workZoneGate] PushBack: NOT GATED - pose not
# computable". Measured over four flights at EKCH B8, one per combination of the
# two writes; only the pair left untouched pushes. GSX builds the tug's path
# from them when it prepares the parking, so changing them afterwards leaves the
# menu offering routes whose geometry no longer exists.
#
# So the menu cannot be narrowed to the assigned route, only marked. Renaming
# has no such effect and is safe at any time, including after the menu is built.
# --------------------------------------------------------------------------

def _standLabels(gate):
    """The parking's two default labels, left slot then right slot."""
    labels = gate.pushbackLabels
    if isinstance(labels, str):
        labels = labels.split("|")
    return [str(label).strip() for label in (labels or [])]


def _standMarked(original, alone=True):
    """What a marked route is called. Empty MARK_LABEL leaves names alone."""
    if not MARK_LABEL:
        return original
    if alone:
        return MARK_LABEL % original if "%s" in MARK_LABEL else MARK_LABEL
    return MARK_LABEL_MANY % original if "%s" in MARK_LABEL_MANY else MARK_LABEL_MANY


def _standSnapshot(self, gate):
    """The parking's route names as the scenery defines them.

    Marking renames in place, so the originals have to be remembered to restore
    from. Re-taken when GSX hands back more extra slots than we remember, which
    is how a rebuild after a re-request shows up - but never while our own label
    is on the gate, or we would record the mark as a scenery name.
    """
    name = gate.uiGateName or ""
    # Never our own entry: it is not a scenery route, and once it is named for
    # the assigned point nothing else would tell it from one.
    current = [slot for slot in (gate.pushbackAddPos or [])
               if not _standIsAtcEntry(slot)]
    labels = _standLabels(gate)
    remembered = self._standGateSnapshot

    ours = bool(MARK_LABEL) and (
        any(_standMarked_is(label) for label in labels)
        or any(isinstance(s, dict) and _standMarked_is(s.get("label"))
               for s in current))

    if (not ours and (remembered is None or remembered["gate"] != name
                      or len(current) > len(remembered["addPos"]))):
        def read(attr):
            try:
                return getattr(gate, attr)
            except Exception:
                return None

        remembered = {
            "gate": name,
            "labels": labels,
            "addPos": current,
            "addPosLabels": [s.get("label") if isinstance(s, dict) else None
                             for s in current],
            # Kept so the menu can be put back exactly as the scenery built it,
            # and so a route's own coordinates can be found again after another
            # route has been written over a slot.
            "pushback": read("pushback"),
            "left": (read("pushbackLeftPos"), read("pushbackLeftApproachPos")),
            "right": (read("pushbackRightPos"), read("pushbackRightApproachPos")),
        }
        self._standGateSnapshot = remembered

    if remembered is None:
        # Our mark is on the gate but the snapshot behind it is gone - an F5
        # reload mid-departure. The scenery's names are not recoverable, so
        # answer with an empty table: marking then declines, which is the right
        # way to lose. Re-requesting pushback rebuilds the parking.
        remembered = {"gate": name, "labels": labels,
                      "addPos": [], "addPosLabels": [], "pushback": None,
                      "left": (None, None), "right": (None, None)}
    return remembered


def _standRouteGeometry(self, gate, wanted):
    """(pos, approach) for the first assigned route, from the original table."""
    pos, approach, _ = _standRouteMatch(self, gate, wanted)
    return pos, approach


def _standRouteMatch(self, gate, wanted):
    """(pos, approach, name) for the first assigned route this stand has.

    In the server's order, so when ATC's point has two facings the one taken is
    the one the server listed first - and the name returned is that route's, so
    a label built from it describes where the entry actually goes.
    """
    snapshot = _standSnapshot(self, gate)
    labels = snapshot["labels"]
    for route in wanted:
        isWanted = _standWantedTest([route])
        if len(labels) > 0 and isWanted(labels[0]) and snapshot["left"][0]:
            return snapshot["left"][0], snapshot["left"][1], labels[0]
        if len(labels) > 1 and isWanted(labels[1]) and snapshot["right"][0]:
            return snapshot["right"][0], snapshot["right"][1], labels[1]
        for slot, was in zip(snapshot["addPos"], snapshot["addPosLabels"]):
            if was and isWanted(was) and isinstance(slot, dict):
                return slot.get("pos"), slot.get("approach"), was
    return None, None, None


def _standIsAtcEntry(slot):
    """True for the extra menu slot this script adds, whatever it is named.

    Off unless ATC_ENTRY: marking renames scenery slots to the same label.
    """
    if not ATC_ENTRY or not isinstance(slot, dict):
        return False
    label = str(slot.get("label") or "")
    return label == ATC_ENTRY_LABEL or label.startswith(ATC_ENTRY_LABEL + ":")


def _standPointName(route):
    """A scenery route label as the pilot should read it: "Z2  Face E" -> "Z2 Face E"."""
    return " ".join(str(route).split())


def _standAtcEntryLabel(wanted):
    """What the entry is called: the bare label until ATC names a point."""
    if not wanted or not ATC_ENTRY_LABEL_POINT:
        return ATC_ENTRY_LABEL
    point = _standPointName(wanted[0])
    if "%s" in ATC_ENTRY_LABEL_POINT:
        return ATC_ENTRY_LABEL_POINT % point
    return ATC_ENTRY_LABEL_POINT


def _standApplyAtcEntry(self, wanted):
    """Add one extra menu entry that goes where ATC said, leaving the rest alone.

    The scenery's own routes stay exactly as they are and the pilot gets one
    more choice. The entry is a clone of the assigned route - same coordinates,
    same approach path, and the same extra fields as a slot the profile already
    defines - so GSX is driving geometry it already knows, under another name.

    The entry is appended once, before GSX builds the menu - a route GSX has
    not seen by then is not on the menu. A reassignment rewrites the list with
    the entry replaced, same length and same order; see _standAtcEntryTo.
    """
    gate = getGate()
    if gate is None or not wanted:
        return False

    pos, approach, name = _standRouteMatch(self, gate, wanted)
    if not pos:
        print("[stands] no route to %s at this stand" % ", ".join(wanted))
        return False
    return _standAtcEntryTo(self, gate, pos, approach, _standAtcEntryLabel([name]),
                            _standRouteShape(self, gate, wanted))


def _standRouteShape(self, gate, wanted):
    """The matched route's own slot settings (snap, wws, startEngines...).

    An extra slot carries them, so the entry takes that route's. A default
    route has none in this form, so it gets _standDefaultShape.
    """
    snapshot = _standSnapshot(self, gate)
    labels = snapshot["labels"]
    for route in wanted:
        isWanted = _standWantedTest([route])
        if any(i < len(labels) and isWanted(labels[i]) for i in (0, 1)):
            break
        for slot, was in zip(snapshot["addPos"], snapshot["addPosLabels"]):
            if was and isWanted(was) and isinstance(slot, dict):
                return dict((k, v) for k, v in slot.items()
                            if k not in ("label", "pos", "approach"))
    return _standDefaultShape(self, gate)


def _standDefaultShape(self, gate):
    """Slot settings for a route without its own: a real slot's, else GSX's."""
    for slot in _standSnapshot(self, gate)["addPos"]:
        if isinstance(slot, dict):
            return dict((k, v) for k, v in slot.items()
                        if k not in ("label", "pos", "approach"))
    return {"snap": False, "wws": 2, "startEngines": 0.0}


def _standDefaultGeometry(self, gate):
    """Where the entry points before ATC has said anything.

    The scenery's own first route: if the pilot picks "ATC Assigned" with no
    clearance, they go where GSX would have taken them anyway.
    """
    snapshot = _standSnapshot(self, gate)
    for pos, approach in (snapshot["left"], snapshot["right"]):
        if pos:
            return pos, approach
    for slot in snapshot["addPos"]:
        if isinstance(slot, dict) and slot.get("pos"):
            return slot.get("pos"), slot.get("approach")
    return None, None


def _standEnsureAtcEntry(self, gate):
    """Put the entry on the menu before GSX builds it, assigned or not.

    GSX builds the route menu about ninety seconds after the tug is called,
    which is usually before the clearance arrives - and an entry that is not on
    the menu by then is not on the menu at all. So it goes on as soon as the
    pilot asks for pushback, and ATC moves it later.

    Only ever creates. An entry already on the menu may be leading to ATC's
    point, and pointing it back at the default would undo the assignment.
    """
    if not ATC_ENTRY or gate is None:
        return False
    if _standLiveAtcEntry(self, gate) is not None:
        return True
    pos, approach = _standDefaultGeometry(self, gate)
    if not pos:
        return False
    return _standAtcEntryTo(self, gate, pos, approach, ATC_ENTRY_LABEL)


def _standLiveAtcEntry(self, gate):
    """Our entry as GSX holds it now, or None.

    Found on the gate rather than trusted from the last write, so that if GSX
    keeps a copy of the dict rather than ours, it is GSX's copy that is moved.
    """
    try:
        slots = list(gate.pushbackAddPos or [])
    except Exception:
        return None
    for slot in slots:
        if _standIsAtcEntry(slot):
            self._standAtcEntry = slot
            return slot
    return None


def _standAtcEntryTo(self, gate, pos, approach, label, shape=None):
    """Create the entry, or move where it leads and what it is called.

    GSX hands the script copies: a dict read out of pushbackAddPos can be
    changed all day without GSX seeing it (flown at D2 - the menu kept the old
    name and the push went to the old route). The only write that lands is the
    whole list, so a change rewrites it with our slot replaced, same length and
    same order, every other slot exactly as read.

    Once GSX has built the menu its names are fixed, so the label is left as it
    was and only where the entry leads changes. GSX reads the routes again when
    the tug drives - which is why restoring the menu at state 5 used to work -
    so a point assigned while the pilot is looking at the menu still lands.
    """
    entry = _standLiveAtcEntry(self, gate)
    if shape is None:
        shape = _standDefaultShape(self, gate)

    if entry is not None:
        moved = dict(entry)
        moved.update(shape)
        moved["pos"] = pos
        # Always written, empty included: a route without an approach must not
        # inherit the last one's waypoints.
        moved["approach"] = approach or []
        if not self._standPushCommittedLatch:
            moved["label"] = label
        try:
            gate.pushbackAddPos = [moved if _standIsAtcEntry(slot) else slot
                                   for slot in list(gate.pushbackAddPos or [])]
            self._standAtcEntry = moved
            return True
        except Exception as err:
            print("[stands] could not move the ATC entry: %s" % err)
            return False

    if self._standPushCommittedLatch:
        print("[stands] too late to add the ATC entry - GSX has built the menu")
        return False

    # The route's own settings, so wws, snap and startEngines are whatever this
    # profile uses for it, rather than something invented here.
    template = dict(shape)
    template["label"] = label
    template["pos"] = pos
    template["approach"] = approach or []

    try:
        gate.pushbackAddPos = list(gate.pushbackAddPos or []) + [template]
        self._standAtcEntry = template
        print("[stands] added the %s entry" % label)
        return True
    except Exception as err:
        print("[stands] could not add the ATC entry: %s" % err)
        return False


def _standAtcEntryPresent(self, wanted):
    """True when our entry is on the menu, leads where ATC says and says so."""
    gate = getGate()
    if gate is None:
        return False
    entry = _standLiveAtcEntry(self, gate)
    if entry is None:
        return False                         # never added, or GSX rebuilt the parking
    try:
        pos, approach, name = _standRouteMatch(self, gate, wanted)
        label = _standAtcEntryLabel([name])
        # The label is frozen once the menu is built; only the route counts.
        ok = (bool(pos) and _standSamePos(entry.get("pos"), pos)
              and _standSameApproach(entry.get("approach"), approach)
              and (self._standPushCommittedLatch or entry.get("label") == label))
        if not ok:
            # Say once per departure what was read back, so a flight shows
            # whether GSX dropped the write or this comparison is wrong.
            note = "%r|%r|%r|%r" % (entry.get("label"), entry.get("pos"), label, pos)
            if note != self._standPresentNote:
                self._standPresentNote = note
                print("[stands] entry reads back label=%r pos=%r (%s), want label=%r "
                      "pos=%r (%s)" % (entry.get("label"), entry.get("pos"),
                                       type(entry.get("pos")).__name__, label, pos,
                                       type(pos).__name__))
        return ok
    except Exception as err:
        print("[stands] could not read the entry back: %s" % err)
        return False


def _standSameApproach(a, b):
    """Approach waypoint lists compared by value; None and empty are the same."""
    a, b = list(a or []), list(b or [])
    return len(a) == len(b) and all(_standSamePos(x, y) for x, y in zip(a, b))


def _standSamePos(a, b):
    """Positions compared by value, whatever sequence or object GSX hands back."""
    if a is None or b is None:
        return False
    if a is b or a == b:
        return True
    try:
        return [round(float(x), 7) for x in a] == [round(float(y), 7) for y in b]
    except Exception:
        return str(a) == str(b)


def _standNarrowMenu(self, gate, wanted):
    """Offer only the assigned route, until GSX has built the menu from it.

    Writing `pushback` and `pushbackAddPos` and leaving them written is what
    breaks the push - GSX reaches "release parking brakes" and never moves. But
    every test that established this narrowed and never put anything back. The
    menu is built at departure state 5 and the push runs later, so the theory
    here is that the pair only has to be right at the second moment, not the
    first: narrow for the menu, restore for the push.

    Untested until a flight says otherwise, which is why SHOW_ONLY_ASSIGNED can
    be turned off in one line.
    """
    if ATC_ENTRY or not SHOW_ONLY_ASSIGNED or self._standMenuRestored:
        return

    if self._standPushCommittedLatch:
        # GSX has already built the menu, so narrowing now shows the pilot
        # nothing - and would leave the properties in the state that stops the
        # tug until the next poll handed them back. This is the case where ATC
        # assigns nothing until after the tug is connected: there is no menu
        # left to narrow, and _standRedirect cannot help either, because the
        # menu was built from every route and which one was picked is unknown.
        return

    snapshot = _standSnapshot(self, gate)
    isWanted = _standWantedTest(wanted)
    labels = snapshot["labels"]

    direction = 0
    if len(labels) > 0 and isWanted(labels[0]):
        direction += 1
    if len(labels) > 1 and isWanted(labels[1]):
        direction += 2
    # Match on the scenery's own names: the assigned slot has been renamed.
    keep = [slot for slot, was in zip(snapshot["addPos"], snapshot["addPosLabels"])
            if was and isWanted(was)]

    if direction == 0 and not keep:
        return
    try:
        gate.pushbackAddPos = keep
        gate.pushback = direction
    except Exception as err:
        print("[stands] could not narrow the menu: %s" % err)


def _standRestoreMenu(self, gate):
    """Give the scenery's own routes back, before GSX has to drive one.

    Called the moment the menu has been built. What the pilot is looking at is
    already rendered; what GSX needs from here is a route list that still
    matches the geometry it prepared.
    """
    if ATC_ENTRY or self._standMenuRestored or gate is None:
        return   # nothing was narrowed, and our entry must not be removed

    snapshot = _standSnapshot(self, gate)
    if snapshot["pushback"] is None:
        return

    # Decide from the gate, not from a flag saying we narrowed it. The flag is
    # cleared when the pilot requests pushback, so a menu narrowed before the
    # request - the normal order - was never handed back, and the tug never
    # moved. What matters is whether the menu differs from the scenery's own,
    # whoever made it so.
    try:
        if (gate.pushback == snapshot["pushback"]
                and len(gate.pushbackAddPos or []) == len(snapshot["addPos"])):
            return
    except Exception:
        return

    # The one route the menu was built from, and therefore the entry the pilot
    # is about to pick. A later change redirects that entry rather than guessing.
    # Kept from the first attempt, so a retried restore cannot replace it.
    if self._standMenuRoute is None:
        self._standMenuRoute = self._standPushback

    try:
        gate.pushbackAddPos = snapshot["addPos"]
        gate.pushback = snapshot["pushback"]
        # Only now: a half-done restore must be retried on the next poll.
        self._standMenuRestored = True
        print("[stands] menu restored for the push")
    except Exception as err:
        print("[stands] could not restore the menu: %s" % err)


def _standRedirect(self, wanted):
    """Send the route the pilot was offered to a newly assigned point.

    The only way to change the push without cancelling it. Once GSX has built
    the menu the labels on it are fixed, but where each entry leads is a
    different property, and the menu was narrowed to exactly one route - so the
    entry the pilot picked is known, and only that one has to be moved.

    The entry keeps its old name. This changes where the aircraft goes, not what
    it is called.
    """
    if not REDIRECT_LATE or not self._standMenuRoute:
        return False

    gate = getGate()
    if gate is None:
        return False

    pos, approach = _standRouteGeometry(self, gate, wanted)
    if not pos:
        print("[stands] no geometry for %s - cannot redirect" % " or ".join(wanted))
        return False

    snapshot = _standSnapshot(self, gate)
    labels = snapshot["labels"]
    wasOffered = _standWantedTest(self._standMenuRoute.split("|"))

    try:
        if len(labels) > 0 and wasOffered(labels[0]):
            gate.pushbackLeftPos = pos
            if approach:
                gate.pushbackLeftApproachPos = approach
        elif len(labels) > 1 and wasOffered(labels[1]):
            gate.pushbackRightPos = pos
            if approach:
                gate.pushbackRightApproachPos = approach
        else:
            slot = None
            for candidate, was in zip(snapshot["addPos"], snapshot["addPosLabels"]):
                if was and wasOffered(was) and isinstance(candidate, dict):
                    slot = candidate
                    break
            if slot is None:
                return False
            # In place, as with the label: replacing the pushbackAddPos list is
            # the write that breaks the push, editing a dict inside it is not.
            slot["pos"] = pos
            if approach:
                slot["approach"] = approach

        print("[stands] redirected the push to %s at %s" % (" or ".join(wanted), pos))
        return True
    except Exception as err:
        print("[stands] could not redirect the push: %s" % err)
        return False


def _standMarked_is(label):
    """True when a label already carries our mark, in either form."""
    if not label or not MARK_LABEL:
        return False
    for form in (MARK_LABEL, MARK_LABEL_MANY):
        if "%s" not in form:
            if str(label) == form:
                return True
        else:
            prefix = form.split("%s")[0]
            if prefix and str(label).startswith(prefix):
                return True
    return False


def _standApplyPushback(self, wanted):
    """Mark the assigned route in the menu, unmarking whatever was marked before.

    Silent: the caller decides what to tell the pilot, because this is also
    called to re-assert a mark that has not changed.
    """
    if ATC_ENTRY:
        return _standApplyAtcEntry(self, wanted)

    gate = getGate()
    if gate is None or not wanted:
        return False

    snapshot = _standSnapshot(self, gate)
    labels = list(snapshot["labels"])
    isWanted = _standWantedTest(wanted)

    # Every assigned route, not just the first: the menu keeps all of them, and
    # marking one of two left the other sitting there under its scenery name.
    defaults = [i for i in (0, 1) if i < len(labels) and isWanted(labels[i])]
    extras = [slot for slot, was in zip(snapshot["addPos"], snapshot["addPosLabels"])
              if was and isWanted(was) and isinstance(slot, dict)]

    if not defaults and not extras:
        print("[stands] no route to %s at this stand" % ", ".join(wanted))
        return False

    # One route on offer can be named for what it is. Two - the same taxiway in
    # both facings - have to be told apart, so they carry the scenery's name.
    alone = len(defaults) + len(extras) == 1

    try:
        for index in defaults:
            labels[index] = _standMarked(labels[index], alone)
        gate.pushbackLabels = tuple(labels)

        _standUnmarkExtras(self, gate, keep=extras)
        for slot, was in zip(snapshot["addPos"], snapshot["addPosLabels"]):
            if slot in extras:
                # Mutate the slot in place. Assigning a new pushbackAddPos list
                # is the write that breaks the push; changing a dict inside the
                # list GSX already holds is not that write. Name it from the
                # scenery's own label so re-marking cannot compound.
                slot["label"] = _standMarked(was, alone)

        _standNarrowMenu(self, gate, wanted)
        return True
    except Exception as err:
        print("[stands] could not mark the assigned route: %s" % err)
        return False


def _standUnmarkExtras(self, gate, keep=()):
    """Give every extra slot its scenery name back, in place."""
    snapshot = _standSnapshot(self, gate)
    for slot, original in zip(snapshot["addPos"], snapshot["addPosLabels"]):
        if slot not in keep and isinstance(slot, dict) and slot.get("label") != original:
            slot["label"] = original


def _standPushbackMarked(self, wanted):
    """True when the assigned route, and only it, currently carries the mark.

    Read back rather than trusting that a write once made stays made: GSX
    rebuilds parking state as the departure runs, and a restored menu would
    otherwise leave the pilot with nothing marked and nothing said.
    """
    if ATC_ENTRY:
        return _standAtcEntryPresent(self, wanted)

    gate = getGate()
    if gate is None:
        return False

    snapshot = _standSnapshot(self, gate)
    isWanted = _standWantedTest(wanted)

    # The same count the marking used, so both agree on which form of the name
    # the assigned route should be carrying.
    alone = (len([n for n in snapshot["labels"] if isWanted(n)])
             + len([w for w in snapshot["addPosLabels"] if w and isWanted(w)])) == 1

    try:
        live = _standLabels(gate)
        extras = [s.get("label") if isinstance(s, dict) else None
                  for s in (gate.pushbackAddPos or [])]

        for index, name in enumerate(snapshot["labels"]):
            want = _standMarked(name, alone) if isWanted(name) else name
            if index >= len(live) or live[index] != want:
                return False

        for name, was in zip(extras, snapshot["addPosLabels"]):
            if was is None:
                continue
            want = _standMarked(was, alone) if isWanted(was) else was
            if name != want:
                return False
        return True
    except Exception:
        return False


# --------------------------------------------------------------------------
# one poll
# --------------------------------------------------------------------------

def _standCheck(self):
    """One fetch-and-apply cycle. Returns False when polling should stop.

    The server sends at most one of the two, because they belong to opposite
    ends of a turnaround: a stand for traffic arriving here, a pushback route
    for traffic leaving. A parked, loading aircraft is never told to move.
    """
    if self._standUserOverride:
        return False

    # Notice a departure that began before this script loaded - after an F5,
    # say - but only once the state has been seen idle, or one left over from an
    # earlier flight would read as a departure already committed and every mark
    # for the rest of the session would be refused.
    state = _standDepartureState(self)
    if state < DEPARTURE_REQUESTED_STATE:
        self._standSeenIdleDeparture = True
    elif self._standSeenIdleDeparture:
        _standNoteDeparture(self, "GSX departure state")

    # Hand the menu back here, on the state itself, and not from anything
    # further down. Everything below returns early when the assignment has not
    # changed - which is the normal case, and exactly the flight where the
    # restore has to happen. Putting it behind that early return meant it never
    # ran at all, and cost a test flight that proved nothing.
    if state >= PUSHBACK_COMMITTED_STATE:
        _standRestoreMenu(self, getGate())

    payload = _standFetch(self)
    if payload is None:
        return True                          # transient error - keep polling

    stand = payload.get("stand")
    if stand:
        return _standCheckArrival(self, stand)

    pushback = _standRoutes(payload.get("pushback"))
    if pushback:
        result = _standCheckDeparture(self, pushback)
        _standRemindAtMenu(self, state)
        return result

    if ATC_ENTRY and self._standPushback:
        _standWithdrawAtcEntry(self)

    # Nothing assigned. The entry still has to be on the menu before GSX builds
    # it, or a clearance arriving later has nowhere to land. Past state 5 the
    # menu is built, and a slot appended now would not be on it.
    if self._standDepartureRequested:
        if state >= PUSHBACK_COMMITTED_STATE:
            self._standPushCommittedLatch = True
        _standEnsureAtcEntry(self, getGate())
    return True


def _standRemindAtMenu(self, state):
    """Name the assigned point once, as GSX opens the route menu.

    showMessage only shows while the GSX menu is open, and the route menu is the
    one moment in a departure that GSX opens it unasked - at state 5, within one
    poll of this. Anything said earlier may never have been seen.
    """
    if (not ATC_ENTRY or self._standMenuReminded or not self._standPushPoint
            or not self._standDepartureRequested
            or state < PUSHBACK_COMMITTED_STATE):
        return
    self._standMenuReminded = True
    if _standPushStarted(self) or _standLiveAtcEntry(self, getGate()) is None:
        return                               # nothing on the menu to point at
    _standSay(self, 'ATC pushback: %s - pick "%s"'
              % (self._standPushPoint, _standMenuLabel(self, getGate())))


def _standMenuLabel(self, gate):
    """The entry's name as GSX holds it - what the pilot sees in the menu."""
    entry = _standLiveAtcEntry(self, gate) if gate is not None else None
    if entry is None:
        return ATC_ENTRY_LABEL
    return str(entry.get("label") or ATC_ENTRY_LABEL)


def _standWithdrawAtcEntry(self):
    """ATC took the point back. Point the entry at the scenery default again.

    Left alone, it would go on naming, and leading to, a point nobody assigned.
    Not once the aircraft is moving: the tug is already driving that geometry.
    False when the gate refused the write: the point is then kept, so the next
    poll tries again rather than forgetting there is anything to undo.
    """
    gate = getGate()
    if (gate is not None and not _standPushStarted(self)
            and _standLiveAtcEntry(self, gate) is not None):
        pos, approach = _standDefaultGeometry(self, gate)
        if pos and not _standAtcEntryTo(self, gate, pos, approach, ATC_ENTRY_LABEL):
            return False                     # keep the point; retried next poll
    print("[stands] pushback point withdrawn (was %s)" % self._standPushback)
    self._standPushback = None
    self._standPushPoint = None
    _standClearVdgs(self)
    return True


def _standCheckArrival(self, stand):
    """Inbound: park on the stand the controller assigned."""
    if _standSame(stand, self._standAssigned or ""):
        return True

    if self._standAssigned is not None:
        _standSay(self, "Stand changed by ATC")
    self._standAssigned = stand
    _standApply(self, stand)
    return True                              # selectGate lands next cycle


def _standApply(self, stand):
    """Select the assigned stand. selectGate is deferred by one GSX cycle."""
    result = selectGate(stand)

    if isinstance(result, list) and result:
        chosen = result[0]                   # ambiguous name
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


def _standCheckDeparture(self, pushback):
    """Outbound: mark the assigned route on the stand we are parked on.

    Runs whether or not the tug has been called, and keeps running while it
    drives out. That is the point: a pilot can request pushback the moment they
    are ready and collect their clearance while it is on its way.
    """
    if ATC_ENTRY:
        return _standCheckDepartureEntry(self, pushback)

    key = "|".join(pushback)
    changed = self._standPushback is not None and key != self._standPushback

    if key == (self._standPushback or "") and _standPushbackMarked(self, pushback):
        return True                          # marked, and still in force

    stage = _standPushStage(self)
    reasserting = key == (self._standPushback or "")

    if not _standApplyPushback(self, pushback):
        # Nothing at this stand reaches the assigned point. Say so once: ATC has
        # given the pilot something the scenery cannot do.
        if key != (self._standUnroutable or ""):
            self._standUnroutable = key
            _standSay(self, "ATC assigned %s - no route to it from this stand"
                      % " or ".join(pushback))
        return True

    self._standUnroutable = None
    self._standPushback = key

    if reasserting:
        print("[stands] re-marked %s - GSX had restored the menu"
              % " or ".join(pushback))
        return True

    what = ("ATC changed your push to %s" if changed else "ATC pushback: %s") \
        % " or ".join(pushback)

    # Past the menu the label is fixed, so move where the chosen route leads
    # instead. This is the only way to change the push without cancelling it.
    redirected = stage == PUSH_UNCERTAIN and _standRedirect(self, pushback)

    if stage == PUSH_ROLLING:
        _standSay(self, "%s - too late for this tug, cancel the pushback and "
                        "request it again" % what)
    elif stage == PUSH_UNCERTAIN:
        if redirected:
            _standSay(self, "%s - your push now goes there, whichever entry you "
                            "pick" % what)
        else:
            _standSay(self, "%s - it is marked in the menu; if the mark is not "
                            "there, cancel and request pushback again" % what)
    elif self._standDepartureRequested:
        _standSay(self, "%s - re-open the pushback menu for the mark" % what)
    else:
        _standSay(self, what)

    _standShowOnVdgs(self, pushback)
    return True


def _standCheckDepartureEntry(self, pushback):
    """Outbound, ATC_ENTRY mode: point the "ATC Assigned" entry at the point.

    The gate is not touched until the pilot asks for pushback. Before that the
    point is only announced, so a parked, boarding aircraft's menu is the
    scenery's own; the entry goes on at the request, already leading here.
    """
    key = "|".join(pushback)
    changed = self._standPushback is not None and key != self._standPushback

    gate = getGate()
    if gate is None:
        return True

    pos, approach, name = _standRouteMatch(self, gate, pushback)
    if not pos:
        # Nothing at this stand reaches the assigned point. Say so once: ATC has
        # given the pilot something the scenery cannot do.
        if key != (self._standUnroutable or ""):
            self._standUnroutable = key
            _standSay(self, "ATC assigned %s - no route to it from this stand"
                      % " or ".join(_standPointName(r) for r in pushback))
        if self._standPushback:
            # The previous point no longer stands. Take it off the entry and
            # the VDGS, which also stops the reminder naming it.
            _standWithdrawAtcEntry(self)
        return True
    self._standUnroutable = None

    point = _standPointName(name)
    label = _standAtcEntryLabel([name])
    what = ("ATC changed your push to %s" if changed else "ATC pushback: %s") % point

    if not self._standDepartureRequested:
        if key != self._standPushback:
            self._standPushback = key
            self._standPushPoint = point
            _standSay(self, '%s - pick "%s" when GSX asks which way to push'
                      % (what, label))
            _standShowOnVdgs(self, [point])
        return True

    if key == self._standPushback and _standAtcEntryPresent(self, pushback):
        return True                          # on the menu, and still in force

    reasserting = key == self._standPushback
    stage = _standPushStage(self)

    if stage == PUSH_ROLLING:
        # Never move the entry now: the tug is driving whatever it leads to.
        if not reasserting:
            self._standPushback = key
            self._standPushPoint = point
            _standSay(self, "%s - too late for this tug, cancel the pushback and "
                            "request it again" % what)
            _standShowOnVdgs(self, [point])
        return True

    applied = _standAtcEntryTo(self, gate, pos, approach, label,
                               _standRouteShape(self, gate, pushback))
    self._standPushback = key
    self._standPushPoint = point

    if not applied:
        # The menu was built without our entry - a reload mid-departure, say.
        # The scenery's own route still goes there, so name that instead.
        self._standMenuReminded = True       # no entry to remind them of
        if not reasserting:
            _standSay(self, '%s - pick "%s" in the pushback menu' % (what, name))
            _standShowOnVdgs(self, [point])
        return True

    if reasserting:
        print("[stands] re-applied the %s entry" % label)
        return True

    if stage == PUSH_UNCERTAIN:
        # The menu is up and its names are fixed, but GSX reads the route again
        # when it drives. Quote the entry by the name the pilot can see.
        shown = _standMenuLabel(self, gate)
        self._standMenuReminded = True       # this is the reminder
        if shown == ATC_ENTRY_LABEL:
            _standSay(self, '%s - pick "%s"' % (what, shown))
        else:
            _standSay(self, '%s - pick "%s", it now goes to %s' % (what, shown, point))
    else:
        _standSay(self, '%s - pick "%s" when GSX asks which way to push'
                  % (what, label))
    _standShowOnVdgs(self, [point])
    return True


# --------------------------------------------------------------------------
# the watch
# --------------------------------------------------------------------------

def _standCancelPoll(self):
    """Stop the watch, tolerating a handle GSX will not take back.

    Some builds reject the handle runAsync gave them. Cancelling is only
    housekeeping - GSX kills every tasklet on reload and on airport exit - so a
    failure must never abort the useful work in the callback that called us.
    """
    handle = self._standPoll
    self._standPoll = None
    if handle is None:
        return
    try:
        cancelAsync(handle)
    except Exception as err:
        print("[stands] cancelAsync declined the handle (%s) - harmless" % err)


def _standStartPolling(self):
    """Run the check loop in a tasklet so GSX is never blocked.

    Each loop carries the generation it started in and stops when that is no
    longer current. cancelAsync cannot be relied on, and three callbacks start a
    watch, so without this a single flight polled three times per interval and
    every message arrived in triplicate.
    """
    _standCancelPoll(self)
    self._standPollGeneration += 1
    generation = self._standPollGeneration

    def loop():
        for _ in range(POLL_LIMIT):
            truewait(POLL_INTERVAL_MS)       # wall clock, not sim rate
            if generation != self._standPollGeneration:
                return                       # superseded
            if not _standCheck(self):
                return

    try:
        self._standPoll = runAsync(loop)
    except Exception as err:
        print("[stands] could not start the watch: %s" % err)
        self._standPoll = None


def _standEnsurePolling(self):
    """Restart the watch if it is not running.

    F9 in the Handler Editor kills every tasklet and re-executes the script, but
    onEnterAirport does not fire again - the airport handler is already active.
    """
    _standInit(self)
    if not self._standUserOverride:
        _standStartPolling(self)


# --------------------------------------------------------------------------
# GSX lifecycle callbacks
# --------------------------------------------------------------------------

def _standResetAll(self):
    """Forget this whole visit. A new airport, or the first time through."""
    self._standUserOverride = False
    self._standAssigned = None
    self._standPushback = None
    self._standPushPoint = None              # the assigned point, as the pilot reads it
    self._standUnroutable = None
    self._standPoll = None
    self._standPollGeneration = 0
    self._standCallsign = None
    self._standCallsignTrusted = False
    self._standWarnedNoCallsign = False
    self._standWarnedNoPosition = False
    self._standDepartureRequested = False
    self._standSeenIdleDeparture = False
    self._standAnchorPos = None
    self._standGateSnapshot = None
    self._standLastDepartureState = None
    # Our added menu slot belongs to the visit, not to one pushback: clearing
    # it per-departure orphaned it, and the next request appended a second.
    self._standAtcEntry = None
    _standResetDeparture(self)


def _standInit(self):
    if not hasattr(self, "_standUserOverride"):
        _standResetAll(self)


def onEnterAirport(self):
    """Fires once the airport handler activates: on the ground, at low speed."""
    _standInit(self)
    _standAnchor(self)

    try:
        payload = _standFetch(self)
    except Exception as err:
        print("[stands] onEnterAirport fetch failed: %s" % err)
        _standStartPolling(self)
        return

    # Poll even with nothing yet. A departure is the normal case here: the pilot
    # spawns cold and the controller assigns minutes later, so an empty first
    # answer is expected, not a reason to give up. Pilots with no strip keep
    # getting an empty 304 - nothing they can perceive.
    if payload is None:
        _standStartPolling(self)
        return

    stand = payload.get("stand")
    if stand is not None and getGate() is not None and not _standOnAssignedStand(self, stand):
        # Inbound but already on a stand we did not assign, so the pilot picked
        # it. Arrivals only: a departing aircraft is always on a stand it chose,
        # and that is exactly where its assigned route applies.
        _standSay(self, "Keeping your stand; ATC updates off")
        self._standUserOverride = True
        return

    _standCheck(self)
    _standStartPolling(self)


def _standOnAssignedStand(self, stand):
    gate = getGate()
    if gate is None:
        return False
    name = (gate.uiGateName or "").upper().replace(" ", "")
    return name.endswith(str(stand).upper().replace(" ", ""))


def onAirportBeforeVehicleSelect(self, *args):
    """Fires whenever the gate is set, including right after onEnterAirport."""
    try:
        _standEnsurePolling(self)
    except Exception as err:
        print("[stands] onAirportBeforeVehicleSelect failed: %s" % err)
    if hasattr(self, "_super_onAirportBeforeVehicleSelect"):
        self._super_onAirportBeforeVehicleSelect(*args)


def onAirportDepartureRequested(self, *args):
    """The pilot asked for pushback - possibly before ATC has assigned one.

    That is the expected order. GSX does not ask which way to push until the tug
    is connected, over a minute later, so this only marks whatever is assigned
    now, says so when nothing is, and records where the aircraft was standing.

    Note the onAirport prefix: airport handlers use it for every service
    callback, and a plain onDepartureRequested would never be called.
    """
    try:
        _standInit(self)
        if not self._standUserOverride:
            _standNoteDeparture(self, "pilot", restart=True)
            _standEnsurePolling(self)
            _standEnsureAtcEntry(self, getGate())
            payload = _standFetch(self)
            routes = _standRoutes(payload.get("pushback")) if payload else []
            if routes and ATC_ENTRY:
                # Announce afresh even if this point was told before the
                # request: then there was nothing on the menu to pick.
                self._standPushback = None
                _standCheckDepartureEntry(self, routes)
            elif routes:
                if _standApplyPushback(self, routes):
                    self._standPushback = "|".join(routes)
                    _standSay(self, "ATC pushback: %s" % " or ".join(routes))
                    _standShowOnVdgs(self, routes)
            elif ANNOUNCE_WAITING and payload is not None:
                _standSay(self, "No ATC pushback yet - it is marked when it arrives")
                _standVdgs(self, ["PUSH", "WAIT ATC"], ["PUSH - AWAIT ATC"])
    except Exception as err:
        print("[stands] onAirportDepartureRequested failed: %s" % err)
    if hasattr(self, "_super_onAirportDepartureRequested"):
        self._super_onAirportDepartureRequested(*args)


def onAirportBypassPinConnected(self, *args):
    """The ground crew has fitted the pin: the route menu is about to be built.

    The last fixed point before GSX reads the menu, so check now rather than
    leave a clearance that has just arrived to the next poll, up to five
    seconds away.
    """
    try:
        _standInit(self)
        if not self._standUserOverride and self._standDepartureRequested:
            _standCheck(self)
    except Exception as err:
        print("[stands] onAirportBypassPinConnected failed: %s" % err)
    if hasattr(self, "_super_onAirportBypassPinConnected"):
        self._super_onAirportBypassPinConnected(*args)


def onAirportGateReset(self, reason):
    """The pilot took control: back off for the rest of this visit.

    Nothing is written back to FlightStrips. The controller's board is the
    source of truth, and a pilot parking elsewhere is a discrepancy they should
    see rather than have the simulator quietly overwrite.
    """
    _standInit(self)
    if reason in ("user_changed", "user_revoked") and (self._standAssigned or self._standPushback):
        self._standUserOverride = True
        _standCancelPoll(self)
        _standClearVdgs(self)
        print("[stands] pilot took over (%s) - ATC updates stopped" % reason)


def onExitAirport(self):
    _standInit(self)
    _standCancelPoll(self)
    _standClearVdgs(self)
    # Drop the whole namespace and let _standInit rebuild it, rather than
    # clearing field by field: the two lists drift apart the moment one grows.
    _standResetAll(self)
