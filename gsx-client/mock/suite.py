# One suite for the GSX handler, run under python 3.7.
#   python suite.py handler.py
import sys, builtins

SRC = open(sys.argv[1], encoding="utf-8").read()
LEFT, RIGHT = (55.626, 12.641, 123.0), (55.625, 12.643, -56.9)
LOG = []


class Gate(object):
    """A stand with two default routes and two extras, one label double-spaced."""

    uiGateName = "Gate B8"

    def __init__(self):
        self.rebuild()
        self.pushbackLeftPos, self.pushbackRightPos = LEFT, RIGHT

    def rebuild(self):
        self.pushbackLabels = ("Y1  Face E", "Y2  Face W")
        # The full shape a .ini slot carries, not just label and pos.
        self.pushbackAddPos = [
            {"label": "Z6  Face E", "pos": (1, 2, 3), "snap": False,
             "approach": [], "wws": 2, "startEngines": 0.0},
            {"label": "Z7  Face W", "pos": (4, 5, 6), "snap": False,
             "approach": [], "wws": 2, "startEngines": 0.0}]
        self.pushback = 3

    def menu(self):
        out = []
        if self.pushback & 1:
            out.append(self.pushbackLabels[0])
        if self.pushback & 2:
            out.append(self.pushbackLabels[1])
        return out + [s["label"] for s in self.pushbackAddPos]


class CopyingGate(Gate):
    """What GSX actually does, flown at D2: every read of pushbackAddPos is a
    fresh copy, so changing a dict read out of it never reaches GSX. Only
    assigning the whole list lands."""

    @property
    def pushbackAddPos(self):
        return [dict(s) for s in self._addPos]

    @pushbackAddPos.setter
    def pushbackAddPos(self, value):
        if getattr(self, "refuse", 0):
            self.refuse -= 1
            raise Exception("write refused")
        self._addPos = [dict(s) for s in value]


class Env(object):
    def __init__(self, copying=False):
        self.gate = CopyingGate() if copying else Gate()
        self.payload = {"stand": None, "pushback": None}
        self.lat, self.lon, self.state = 55.6267, 12.6432, 1
        self.vdgs = None

    def globals(self, restricted=False):
        env = self

        def vdgs(msg):
            env.vdgs = msg["display"]["wide"]["pages"][0]["lines"][0]

        def calc(code):
            if "LATITUDE" in code:
                return env.lat
            if "LONGITUDE" in code:
                return env.lon
            if "DEPARTURE_STATE" in code:
                return env.state
            raise Exception("unknown " + code)

        g = {"print": lambda *a: LOG.append(" ".join(str(x) for x in a)),
             "showMessage": lambda t: LOG.append("SAY " + t),
             "fetchJson": lambda u, timeout=0, etag=False: env.payload,
             "getGate": lambda: env.gate,
             "getAirport": lambda: type("A", (), {"icao": "EKCH"})(),
             "selectGate": lambda n: True, "runAsync": lambda f: "h",
             "cancelAsync": lambda h: None, "truewait": lambda ms: None,
             "addVdgsMessage": vdgs, "removeVdgsMessage": lambda i: None,
             "executeCalculatorCode": calc,
             "getSimbrief": lambda: type("S", (), {"last_error": None,
                                                   "callsign": "DAT99"})(),
             # What a profile script the block is embedded in may use, for its
             # own operator selection, say.
             "aircraft": type("Aircraft", (), {"icaoAirline": "SAS", "icaoType": "A20N"})(),
             "getGlobalPersistentVariable": lambda name, default=None: "auto",
             "setGlobalPersistentVariable": lambda name, value: None,
             "choiceBox": lambda *a, **k: 0,
             "Distance": type("Distance", (), {"fromMeters": staticmethod(lambda m: m)})}
        if restricted:
            # GSX hands a handler a restricted builtins set - globals() is not
            # in it, which once stopped the whole script loading.
            # Only what the script has been seen to use successfully in GSX.
            # setattr, vars, delattr and globals are deliberately absent: three
            # of those have already broken a real flight.
            allowed = ("isinstance str len abs float int any list tuple dict zip "
                       "enumerate hasattr getattr range bool Exception TypeError "
                       "ValueError").split()
            g["__builtins__"] = {n: getattr(builtins, n) for n in allowed
                                 if hasattr(builtins, n)}
        return g


class Handler(object):
    """Mimics GSX's handlerSandbox: an attribute is readable only if it was
    created through the normal setter. Writing straight into __dict__ made every
    _stand field unreadable and killed the poll mid-departure with
    "'_standMenuRestored' is not accessible from handler scripts"."""

    def __init__(self):
        object.__setattr__(self, "_created", set())

    def __setattr__(self, name, value):
        object.__getattribute__(self, "_created").add(name)
        object.__setattr__(self, name, value)

    def __getattribute__(self, name):
        if name.startswith("_stand"):
            if name not in object.__getattribute__(self, "_created"):
                raise AttributeError(
                    "'%s' is not accessible from handler scripts" % name)
        return object.__getattribute__(self, name)


def load(env, src=SRC, **restricted):
    g = env.globals(**restricted)
    exec(compile(src, "handler.py", "exec"), g)
    return g


def said():
    out = [l[4:] for l in LOG if l.startswith("SAY ")]
    del LOG[:]
    return out


def case(name):
    print("  " + name)


# Marking and narrowing are separate concerns; these first sections isolate
# marking, so the menu they see is the whole of what the scenery offers.
NO_ENTRY = SRC.replace("ATC_ENTRY = True", "ATC_ENTRY = False")
NARROW_ONLY = NO_ENTRY                       # narrow the menu, restore for the push
MARK_ONLY = NO_ENTRY.replace("SHOW_ONLY_ASSIGNED = True", "SHOW_ONLY_ASSIGNED = False")


# -- marking -----------------------------------------------------------------
# Only pushbackLabels may be written: writing pushback or pushbackAddPos leaves
# the tug with no computable path. Four flights at EKCH B8 established that.

env = Env()
g = load(env, src=MARK_ONLY)
self = Handler()
g["_standInit"](self)
frozen = (env.gate.pushback, env.gate.pushbackAddPos,
          env.gate.pushbackLeftPos, env.gate.pushbackRightPos)


def marks(route, expect):
    assert g["_standApplyPushback"](self, route) is True, route
    assert env.gate.menu() == expect, env.gate.menu()
    assert env.gate.pushback == 3, "bitmask must never be written"
    assert env.gate.pushbackAddPos is frozen[1], "the extras list must never be replaced"
    assert (env.gate.pushbackLeftPos, env.gate.pushbackRightPos) == frozen[2:], "no geometry"
    assert g["_standPushbackMarked"](self, route) is True, "must read back as marked"


marks(["Y1 Face E"], ["ATC Assigned", "Y2  Face W", "Z6  Face E", "Z7  Face W"])
marks(["Y2 Face W"], ["Y1  Face E", "ATC Assigned", "Z6  Face E", "Z7  Face W"])
marks(["Z6 Face E"], ["Y1  Face E", "Y2  Face W", "ATC Assigned", "Z7  Face W"])
marks(["Z7 Face W"], ["Y1  Face E", "Y2  Face W", "Z6  Face E", "ATC Assigned"])
marks(["Y1 Face E"], ["ATC Assigned", "Y2  Face W", "Z6  Face E", "Z7  Face W"])
case("marks one route, restores the last, never writes what breaks the push")

assert g["_standApplyPushback"](self, ["Z9 Face N"]) is False
case("a route the stand cannot reach declines, menu untouched")

# GSX rebuilds the parking: the mark must come back, silently. Go through
# _standCheckDeparture first so the route counts as already applied.
g["_standCheckDeparture"](self, ["Y1 Face E"])
del LOG[:]
env.gate.rebuild()
assert g["_standPushbackMarked"](self, ["Y1 Face E"]) is False
g["_standCheckDeparture"](self, ["Y1 Face E"])
assert env.gate.menu()[0] == "ATC Assigned", env.gate.menu()
assert not said(), "a re-mark must not talk to the pilot"
case("re-marks silently after GSX restores the menu")

# -- the flight that exposed the snapshot bug --------------------------------

env = Env()
g = load(env, src=MARK_ONLY)
self = Handler()
g["onEnterAirport"](self)
del LOG[:]

env.payload = {"stand": None, "pushback": ["Y2 Face W"]}
g["_standCheck"](self)
assert env.gate.menu()[1] == "ATC Assigned", env.gate.menu()

g["onAirportDepartureRequested"](self)          # used to destroy the snapshot
env.state = 4
del LOG[:]
assert env.gate.menu()[1] == "ATC Assigned", env.gate.menu()

env.payload = {"stand": None, "pushback": ["Y1 Face E"]}
env.state = 5
g["_standCheck"](self)
assert env.gate.menu()[0] == "ATC Assigned", env.gate.menu()
assert env.gate.menu()[1] == "Y2  Face W", "the old mark must be given back"
msg = said()
# Marking alone cannot redirect: with four entries on offer there is no knowing
# which one the pilot picked.
assert msg and "cancel and request pushback again" in msg[0], msg
case("a change after the pilot has asked still marks, and is explained")

env.payload = {"stand": None, "pushback": ["Z9 Face N"]}
g["_standCheck"](self)
assert "no route to it" in said()[0]
g["_standCheck"](self)
assert not said(), "an unroutable point must be said once, not every poll"
case("an unroutable point is said once")

env.lat += 0.0002                                # the push starts
env.payload = {"stand": None, "pushback": ["Z7 Face W"]}
g["_standCheck"](self)
assert "too late for this tug" in said()[0]
case("once rolling, the pilot is told to cancel and request again")

# -- environment -------------------------------------------------------------

for base, expect in (("https://x.app", "https://x.app/api/gsx/stand"),
                     ("https://x.app/", "https://x.app/api/gsx/stand"),
                     ("https://x.app///", "https://x.app/api/gsx/stand")):
    seen = []
    e = Env()
    gg = e.globals()
    gg["fetchJson"] = lambda u, timeout=0, etag=False: seen.append(u)
    # FS_API_BASE once embedded in a profile's script by tools/embed.js.
    exec(compile(SRC.replace('API_BASE = "https://api.flightstrips.dk"',
                             'API_BASE = "%s"' % base), "h.py", "exec"), gg)
    h = Handler()
    gg["_standInit"](h)
    gg["_standFetch"](h)
    assert seen[-1].startswith(expect), seen[-1]
case("a trailing slash on API_BASE cannot produce a 404 double slash")

url = []
e = Env()
gg = e.globals()
gg["fetchJson"] = lambda u, timeout=0, etag=False: url.append(u)
exec(compile(SRC, "h.py", "exec"), gg)
h = Handler()
gg["_standInit"](h)
gg["_standFetch"](h)
assert "scenery=" not in url[-1], url[-1]
case("the poll names no GSX profile: every profile gets the same answer")

# -- arrival: stand names a profile spells its own way --------------------------


class Parking(object):
    def __init__(self, name):
        self.uiGateName = name


def arrive(names, stand):
    """Select stand at an airport with these parkings; the name GSX lands on."""
    parkings = [Parking(n) for n in names]
    chosen = []

    def select(wanted):
        # GSX: an exact or a suffix match on the UI name, else False.
        if isinstance(wanted, Parking):
            chosen.append(wanted.uiGateName)
            return True
        hits = [p for p in parkings
                if p.uiGateName.upper() == wanted.upper()
                or p.uiGateName.upper().endswith(" " + wanted.upper())]
        if len(hits) == 1:
            chosen.append(hits[0].uiGateName)
            return True
        return hits or False

    e = Env()
    gg = e.globals(restricted=True)
    exec(compile(SRC, "h.py", "exec"), gg)
    gg["selectGate"] = select
    gg["getAirport"] = lambda: type("A", (), {"icao": "EKCH", "parkings": parkings})()
    gg["_standApply"](Handler(), stand)
    return chosen[-1] if chosen else None


assert arrive(["Gate D2", "Gate D3"], "D2") == "Gate D2"
case("a stand GSX matches by name needs nothing more")

assert arrive(["Parking 91", "Gate A31"], "F91") == "Parking 91"
assert arrive(["None 95"], "F95") == "None 95"
assert arrive(["Parking 110", "E Parking 110"], "E110") == "E Parking 110"
assert arrive(["Parking 007"], "F7") == "Parking 007"
case("a stand the profile names differently is found by its number")

assert arrive(["E Parking 91"], "F91") is None, "an E stand is not F91"
assert arrive(["Gate C91"], "F91") is None, "C91 is not F91"
assert arrive(["Parking 89", "W Parking 89", "Apron 89"], "E89") is None
assert arrive(["Gate A1"], "F92") is None
case("a number that fits another stand, or several, is never guessed")

# The callbacks catch and log their own exceptions, so surviving is not enough:
# each has to have done its job, and nothing may have logged a failure.
env = Env()
g = load(env, restricted=True)
h = Handler()
del LOG[:]
g["_standInit"](h)
g["_standCheck"](h)
env.payload = {"stand": None, "pushback": ["Y2 Face W"]}
g["onAirportDepartureRequested"](h)
assert env.gate.menu()[-1] == "ATC Assigned: Y2 Face W", env.gate.menu()
g["onAirportGateReset"](h, "user_changed")
assert any("pilot took over" in l for l in LOG), LOG
g["onExitAirport"](h)
assert not any("failed" in l for l in LOG), LOG
case("every callback survives GSX's restricted builtins, and does its job")

# MARK_LABEL = "" must leave every name as the scenery wrote it. Narrowing off
# too, so this tests naming alone.
env = Env()
g = load(env, src=MARK_ONLY.replace('MARK_LABEL = "ATC Assigned"', 'MARK_LABEL = ""'))
h = Handler()
g["_standInit"](h)
before = list(env.gate.menu())
g["_standApplyPushback"](h, ["Y1 Face E"])
assert env.gate.menu() == before, env.gate.menu()
case("MARK_LABEL='' leaves every name as the scenery wrote it")

# -- only the assigned route, then the scenery's own back for the push --------
# Narrowing and leaving it narrowed is what stops the tug. The menu is built at
# departure state 5 and the push runs later, so the pair only has to be right at
# the second moment.

env = Env()
g = load(env, src=NARROW_ONLY)
self = Handler()
g["_standInit"](self)
original = (env.gate.pushback, list(env.gate.pushbackAddPos))

g["_standApplyPushback"](self, ["Y2 Face W"])
assert env.gate.menu() == ["ATC Assigned"], env.gate.menu()
case("the menu offers the assigned route and nothing else")

env.state = 5
self._standDepartureRequested = True
env.payload = {"stand": None, "pushback": ["Y2 Face W"]}
g["_standCheck"](self)                      # the restore hangs off the state
assert env.gate.pushback == original[0], env.gate.pushback
assert env.gate.pushbackAddPos == original[1], env.gate.pushbackAddPos
assert env.gate.menu()[1] == "ATC Assigned", env.gate.menu()
case("state 5 puts the scenery's routes back, mark intact, for the push to drive")

# Restoring happens once; a later poll must not narrow again behind the push.
g["_standApplyPushback"](self, ["Y2 Face W"])
assert env.gate.pushback == original[0], "must not re-narrow after the restore"
case("no re-narrowing once the menu has been handed back")

# -- changing the point after the menu, without cancelling -------------------

env = Env()
g = load(env, src=NARROW_ONLY)   # the redirect needs to know what was offered
self = Handler()
g["_standInit"](self)
g["onEnterAirport"](self)
env.payload = {"stand": None, "pushback": ["Y2 Face W"]}
g["_standCheck"](self)
g["onAirportDepartureRequested"](self)
env.state = 5
g["_standCheck"](self)                       # crosses into PUSH_UNCERTAIN
del LOG[:]

env.payload = {"stand": None, "pushback": ["Y1 Face E"]}
g["_standCheck"](self)
assert env.gate.pushbackLeftPos == LEFT, env.gate.pushbackLeftPos
assert env.gate.pushbackRightPos == LEFT, "both entries must lead to the new point"
msg = said()
assert msg and "your push now goes there" in msg[0], msg
case("a change after the menu redirects the push, no cancelling")

# An extra slot's coordinates must be reachable as a redirect target too. The
# entry the pilot was offered was Y2, the right default, so that is what moves.
env.payload = {"stand": None, "pushback": ["Z6 Face E"]}
g["_standCheck"](self)
assert env.gate.pushbackRightPos == (1, 2, 3), env.gate.pushbackRightPos
assert env.gate.pushbackLeftPos == LEFT, "only the offered entry may move"
case("an extra slot is a valid redirect target")

# With the experiment off, nothing is moved and the old advice returns.
env = Env()
g = load(env, src=NARROW_ONLY.replace("REDIRECT_LATE = True", "REDIRECT_LATE = False"))
self = Handler()
g["_standInit"](self)
g["onEnterAirport"](self)
env.payload = {"stand": None, "pushback": ["Y2 Face W"]}
g["_standCheck"](self)
g["onAirportDepartureRequested"](self)
env.state = 5
g["_standCheck"](self)
del LOG[:]
env.payload = {"stand": None, "pushback": ["Y1 Face E"]}
g["_standCheck"](self)
assert env.gate.pushbackRightPos == RIGHT, "REDIRECT_LATE off must move nothing"
assert "cancel and request pushback again" in said()[0]
case("REDIRECT_LATE off moves nothing and says so")


# The restore must happen on the departure state alone. It used to hang off
# _standPushStage, which _standCheckDeparture skips whenever the assignment has
# not changed - the normal case, and the one flight where it has to run.

env = Env()
g = load(env, src=NARROW_ONLY)
self = Handler()
g["onEnterAirport"](self)
env.payload = {"stand": None, "pushback": ["Y2 Face W"]}
g["_standCheck"](self)
g["onAirportDepartureRequested"](self)
narrowed = (env.gate.pushback, list(env.gate.pushbackAddPos))
assert env.gate.menu() == ["ATC Assigned"], env.gate.menu()

env.state = 5
del LOG[:]
g["_standCheck"](self)                      # same assignment, nothing changed
assert any("menu restored" in l for l in LOG), LOG
assert env.gate.pushback == 3, env.gate.pushback
assert len(env.gate.pushbackAddPos) == 2, env.gate.pushbackAddPos
assert narrowed[0] == 2, "the menu really was narrowed first"
case("the restore runs at state 5 even when the assignment never changes")


# Only the entry the pilot was actually offered may be moved. Everything else
# keeps the geometry the scenery gave it, so a later re-request still works.
env = Env()
g = load(env, src=NARROW_ONLY)
self = Handler()
g["onEnterAirport"](self)
env.payload = {"stand": None, "pushback": ["Z6 Face E"]}   # an extra slot
g["_standCheck"](self)
g["onAirportDepartureRequested"](self)
assert env.gate.menu() == ["ATC Assigned"], env.gate.menu()
env.state = 5
g["_standCheck"](self)
env.payload = {"stand": None, "pushback": ["Y1 Face E"]}
g["_standCheck"](self)
assert env.gate.pushbackAddPos[0]["pos"] == LEFT, env.gate.pushbackAddPos[0]["pos"]
assert env.gate.pushbackLeftPos == LEFT and env.gate.pushbackRightPos == RIGHT, "defaults untouched"
assert env.gate.pushbackAddPos[1]["pos"] == (4, 5, 6), "the other extra is untouched"
case("an extra slot offered to the pilot is the one redirected, in place")


# Two facings of the same taxiway: both are offered, so a bare "ATC Assigned"
# twice would say nothing. They carry the scenery's names instead.
env = Env()
env.gate.pushbackAddPos = [{"label": "Z1  Face E", "pos": (7, 8, 9)},
                           {"label": "Z1  Face W", "pos": (10, 11, 12)}]
g = load(env, src=NARROW_ONLY)
self = Handler()
g["_standInit"](self)
assert g["_standApplyPushback"](self, ["Z1 Face E", "Z1 Face W"]) is True
assert env.gate.menu() == ["ATC: Z1  Face E", "ATC: Z1  Face W"], env.gate.menu()
assert g["_standPushbackMarked"](self, ["Z1 Face E", "Z1 Face W"]) is True
case("two facings are named, not two identical entries")

# ...and one route on its own is just the bare name.
env = Env()
g = load(env, src=NARROW_ONLY)
self = Handler()
g["_standInit"](self)
g["_standApplyPushback"](self, ["Y2 Face W"])
assert env.gate.menu() == ["ATC Assigned"], env.gate.menu()
case("a single route is called ATC Assigned and nothing more")


# A second departure in the same session must start from nothing. It used to
# inherit the first one's state: the menu counted as already restored so it was
# never narrowed, and the redirect aimed at the route the previous departure
# had offered.
env = Env()
g = load(env, src=NARROW_ONLY)
self = Handler()
g["onEnterAirport"](self)

env.payload = {"stand": None, "pushback": ["Y2 Face W"]}
g["_standCheck"](self)
g["onAirportDepartureRequested"](self)
env.state = 5
g["_standCheck"](self)                       # first departure: narrow, restore
assert self._standMenuRestored and self._standMenuRoute == "Y2 Face W"

env.gate.rebuild()                           # the pilot cancels; GSX rebuilds
env.state = 1
g["_standCheck"](self)
env.payload = {"stand": None, "pushback": ["Y1 Face E"]}
g["_standCheck"](self)
g["onAirportDepartureRequested"](self)       # second departure
assert env.gate.menu() == ["ATC Assigned"], env.gate.menu()
assert env.gate.pushback == 1, "the second departure must narrow too"

env.state = 5
del LOG[:]
g["_standCheck"](self)
assert any("menu restored" in l for l in LOG), LOG
assert self._standMenuRoute == "Y1 Face E", self._standMenuRoute
case("a second departure narrows, restores and redirects on its own terms")

# ...and its redirect must move the route this departure offered, not the last.
env.payload = {"stand": None, "pushback": ["Z6 Face E"]}
g["_standCheck"](self)
assert env.gate.pushbackLeftPos == (1, 2, 3), env.gate.pushbackLeftPos
assert env.gate.pushbackRightPos == RIGHT, "the previous departure's slot must not move"
case("the redirect follows this departure's offered route")


# The pilot calls the tug with nothing assigned. Assigned before GSX builds the
# menu, the narrowing still lands; assigned after, nothing is written at all -
# narrowing then shows nobody anything and would stop the tug until the next
# poll undid it.
env = Env()
g = load(env, src=NARROW_ONLY)
self = Handler()
g["onEnterAirport"](self)
g["onAirportDepartureRequested"](self)          # nothing assigned yet
env.state = 4
g["_standCheck"](self)
assert env.gate.menu() == ["Y1  Face E", "Y2  Face W", "Z6  Face E", "Z7  Face W"]

env.payload = {"stand": None, "pushback": ["Y2 Face W"]}
g["_standCheck"](self)                          # assigned before state 5
assert env.gate.menu() == ["ATC Assigned"], env.gate.menu()
case("assigned after the tug was called but before the menu: still narrows")

env = Env()
g = load(env, src=NARROW_ONLY)
self = Handler()
g["onEnterAirport"](self)
g["onAirportDepartureRequested"](self)
env.state = 5
g["_standCheck"](self)                          # menu built with everything
untouched = (env.gate.pushback, list(env.gate.pushbackAddPos))
env.payload = {"stand": None, "pushback": ["Y2 Face W"]}
del LOG[:]
g["_standCheck"](self)                          # assigned too late
assert env.gate.pushback == untouched[0], "must not narrow behind a built menu"
assert env.gate.pushbackAddPos == untouched[1], env.gate.pushbackAddPos
assert "cancel and request pushback again" in said()[0]
case("assigned after the menu was built: nothing is written, and it says so")


# The normal order: ATC assigns before the pilot calls the tug. The restore used
# to key off a flag that the pushback request cleared, so a menu narrowed before
# the request was never handed back and the tug never moved.
env = Env()
g = load(env, src=NARROW_ONLY)
self = Handler()
g["onEnterAirport"](self)
env.payload = {"stand": None, "pushback": ["Y2 Face W"]}
g["_standCheck"](self)
assert env.gate.menu() == ["ATC Assigned"], env.gate.menu()

env.payload = None                       # and the fetch fails as they request
g["onAirportDepartureRequested"](self)
env.state = 5
g["_standCheck"](self)
assert env.gate.pushback == 3, "the menu must be handed back before the push"
assert len(env.gate.pushbackAddPos) == 2, env.gate.pushbackAddPos
case("assigned before the tug was called: still restored for the push")


# -- the ATC Assigned entry --------------------------------------------------
# The scenery's routes stay exactly as they are and one more is added, cloned
# from the assigned route so GSX drives geometry it already has. It is named for
# the point while the menu can still show a new name.
#
# Every case runs against CopyingGate: GSX hands out copies, so the only write
# that counts is assigning the whole list. Editing a dict in place passed every
# test here and did nothing in the sim.

def atc(env):
    return env.gate.pushbackAddPos[-1]


env = Env(copying=True)
g = load(env)
self = Handler()
g["onEnterAirport"](self)
scenery = list(env.gate.menu())

# Assigned while parked: announced, but the gate is left alone until the pilot
# asks for pushback.
env.payload = {"stand": None, "pushback": ["Y2 Face W"]}
del LOG[:]
g["_standCheck"](self)
assert env.gate.menu() == scenery, env.gate.menu()
assert said() == ['ATC pushback: Y2 Face W - pick "ATC Assigned: Y2 Face W" '
                  'when GSX asks which way to push'], LOG
assert env.vdgs == "PUSH Y2 FACE W", env.vdgs
g["_standCheck"](self)
assert said() == [], "said once, not once a poll"
case("assigned before the request: announced, gate untouched")

g["onAirportDepartureRequested"](self)
assert env.gate.menu() == scenery + ["ATC Assigned: Y2 Face W"], env.gate.menu()
assert env.gate.pushback == 3, "the defaults must stay on offer"
assert env.gate.pushbackLabels == ("Y1  Face E", "Y2  Face W"), "no renaming"
assert atc(env)["pos"] == RIGHT, atc(env)["pos"]
assert atc(env)["snap"] is False and atc(env)["wws"] == 2, "the profile's own slot shape"
assert said() == ['ATC pushback: Y2 Face W - pick "ATC Assigned: Y2 Face W" '
                  'when GSX asks which way to push'], "told again, now it is on the menu"
case("the request adds the entry, named for the point and leading there")

env.state = 4
del LOG[:]
g["_standCheck"](self)
g["_standCheck"](self)
assert len(env.gate.pushbackAddPos) == 3, "added once"
assert not any("re-applied" in l for l in LOG), "a write that landed reads back as landed"
assert said() == [], "nothing to say while nothing changes"
case("the entry reads back as written - no re-applying every poll")

# The menu opens at state 5: name the point once, while it can be seen.
env.state = 5
g["_standCheck"](self)
assert said() == ['ATC pushback: Y2 Face W - pick "ATC Assigned: Y2 Face W"'], LOG
g["_standCheck"](self)
assert said() == [], "reminded once per departure"
case("the point is named again as the route menu opens")

# A change with the menu up: the whole list is rewritten with the entry moved,
# same length, same order - and the name left as the menu shows it.
env.payload = {"stand": None, "pushback": ["Y1 Face E"]}
g["_standCheck"](self)
assert env.gate.menu() == scenery + ["ATC Assigned: Y2 Face W"], env.gate.menu()
assert atc(env)["pos"] == LEFT, atc(env)["pos"]
assert [s["pos"] for s in env.gate.pushbackAddPos[:2]] == [(1, 2, 3), (4, 5, 6)]
assert env.gate.pushbackLeftPos == LEFT and env.gate.pushbackRightPos == RIGHT
assert said() == ['ATC changed your push to Y1 Face E - pick "ATC Assigned: Y2 Face W", '
                  'it now goes to Y1 Face E'], LOG
del LOG[:]
g["_standCheck"](self)
assert not any("re-applied" in l for l in LOG), LOG
case("a change with the menu up moves the entry, keeps the name it is shown by")

# Nothing was narrowed, so nothing may be restored - that would delete it.
assert not any("menu restored" in l for l in LOG), LOG
assert len(env.gate.pushbackAddPos) == 3, env.gate.pushbackAddPos
case("the restore stands down, so the entry survives to the push")

# Rolling: the tug is driving the entry's geometry. It must not move.
env.lat += 0.001
env.payload = {"stand": None, "pushback": ["Z6 Face E"]}
g["_standCheck"](self)
assert atc(env)["pos"] == LEFT, "never moved under a moving tug"
assert any("too late for this tug" in l for l in said())
case("once rolling, the entry is left alone and the pilot told to re-request")


# Most pilots call for pushback only once GSX shows the route menu, so the
# point arrives after state 5. The menu says a bare "ATC Assigned"; where it
# goes still changes, because GSX reads the route again when the tug drives.
env = Env(copying=True)
g = load(env)
self = Handler()
g["onEnterAirport"](self)
scenery = list(env.gate.menu())
g["onAirportDepartureRequested"](self)
env.state = 4
g["_standCheck"](self)
env.state = 5
g["_standCheck"](self)                          # menu built, nothing assigned
assert env.gate.menu() == scenery + ["ATC Assigned"], env.gate.menu()
del LOG[:]
env.payload = {"stand": None, "pushback": ["Z7 Face W"]}
g["_standCheck"](self)
assert atc(env)["pos"] == (4, 5, 6), atc(env)["pos"]
assert env.gate.menu() == scenery + ["ATC Assigned"], "the menu's name stays put"
assert said() == ['ATC pushback: Z7 Face W - pick "ATC Assigned"'], "said once, no double reminder"
g["_standCheck"](self)
assert said() == [] and not any("re-applied" in l for l in LOG), LOG
case("assigned while the menu is up: the bare entry is pointed there")


# A point this stand cannot reach adds nothing rather than a broken entry.
env = Env(copying=True)
g = load(env)
h = Handler()
g["onEnterAirport"](h)
env.payload = {"stand": None, "pushback": ["Z9 Face N"]}
g["onAirportDepartureRequested"](h)
assert env.gate.menu() == scenery + ["ATC Assigned"], env.gate.menu()
assert atc(env)["pos"] == LEFT, "still the scenery default"
assert any("no route to it from this stand" in l for l in said())
case("an unreachable point leaves the entry at the default, and says so")


# The entry goes on at the pushback request, pointing at the scenery's own
# first route until ATC speaks.
env = Env(copying=True)
g = load(env)
self = Handler()
g["onEnterAirport"](self)
scenery = list(env.gate.menu())
assert env.gate.menu() == scenery, "nothing added before the pilot asks"

g["onAirportDepartureRequested"](self)          # still nothing assigned
assert env.gate.menu() == scenery + ["ATC Assigned"], env.gate.menu()
assert atc(env)["pos"] == LEFT, "unassigned, it goes where GSX would have gone"
case("ATC Assigned is on the menu from the pushback request, assigned or not")

env.state = 4
g["_standCheck"](self)                          # polls with nothing assigned
assert len(env.gate.pushbackAddPos) == 3, "and is added once, not once a poll"

# The scenery's label has a double space; the menu shows it single-spaced.
env.payload = {"stand": None, "pushback": ["Z6 Face E"]}
g["_standCheck"](self)
assert atc(env)["pos"] == (1, 2, 3), atc(env)["pos"]
assert env.gate.menu() == scenery + ["ATC Assigned: Z6 Face E"], env.gate.menu()
case("a clearance arriving before the menu moves and names it")

# Our own entry, renamed, must never be mistaken for a scenery route.
snap = g["_standSnapshot"](self, env.gate)
assert len(snap["addPos"]) == 2, snap["addPosLabels"]
assert all(not str(l).startswith("ATC") for l in snap["addPosLabels"]), snap
case("the named entry never enters the scenery snapshot")

# Withdrawn: back to the default, and no longer naming a point.
env.payload = {"stand": None, "pushback": None}
g["_standCheck"](self)
assert atc(env)["pos"] == LEFT, atc(env)["pos"]
assert env.gate.menu() == scenery + ["ATC Assigned"], env.gate.menu()
assert len(env.gate.pushbackAddPos) == 3
case("a withdrawn point puts the entry back to the scenery default")

# Two facings: the first the server lists is the one taken, and named.
env.payload = {"stand": None, "pushback": ["Z7 Face W", "Z6 Face E"]}
g["_standCheck"](self)
assert atc(env)["pos"] == (4, 5, 6), atc(env)["pos"]
assert env.gate.menu()[-1] == "ATC Assigned: Z7 Face W", env.gate.menu()
case("of two facings the server's first is taken, and the label matches it")

# The bypass pin is the last fixed point before the menu: check right then.
env = Env(copying=True)
g = load(env)
self = Handler()
g["onEnterAirport"](self)
g["onAirportDepartureRequested"](self)
env.state = 4
env.payload = {"stand": None, "pushback": ["Y2 Face W"]}
g["onAirportBypassPinConnected"](self)
assert env.gate.menu()[-1] == "ATC Assigned: Y2 Face W", env.gate.menu()
case("the bypass pin applies a fresh clearance without waiting for the poll")

# Every new callback under GSX's restricted builtins.
env = Env(copying=True)
g = load(env, restricted=True)
self = Handler()
g["onEnterAirport"](self)
env.payload = {"stand": None, "pushback": ["Y2 Face W"]}
g["_standCheck"](self)
g["onAirportDepartureRequested"](self)
g["onAirportBypassPinConnected"](self)
env.state = 5
g["_standCheck"](self)
assert not any("failed" in l for l in LOG), LOG
case("the ATC entry path survives GSX's restricted builtins")

# -- review fixes ------------------------------------------------------------

def departing(**gate):
    env = Env(copying=True)
    for k, v in gate.items():
        setattr(env.gate, k, v)
    g = load(env)
    h = Handler()
    g["onEnterAirport"](h)
    g["onAirportDepartureRequested"](h)
    env.state = 4
    return env, g, h


# A route without an approach must not keep the last one's waypoints.
env, g, h = departing()
slots = env.gate.pushbackAddPos
slots[0]["approach"] = [(9, 9, 9)]
env.gate.pushbackAddPos = slots
h._standGateSnapshot = None
env.payload = {"stand": None, "pushback": ["Z6 Face E"]}
g["_standCheck"](h)
assert atc(env)["approach"] == [(9, 9, 9)], atc(env)
env.payload = {"stand": None, "pushback": ["Z7 Face W"]}
g["_standCheck"](h)
assert atc(env)["approach"] == [], atc(env)
case("a route without an approach clears the previous one's")

# The entry takes the matched route's own slot settings.
env, g, h = departing()
slots = env.gate.pushbackAddPos
slots[1]["snap"] = True
slots[1]["wws"] = 0
env.gate.pushbackAddPos = slots
h._standGateSnapshot = None
env.payload = {"stand": None, "pushback": ["Z7 Face W"]}
g["_standCheck"](h)
assert atc(env)["snap"] is True and atc(env)["wws"] == 0, atc(env)
env.payload = {"stand": None, "pushback": ["Y1 Face E"]}
g["_standCheck"](h)
assert atc(env)["snap"] is False and atc(env)["wws"] == 2, atc(env)
case("the entry carries the matched route's snap and wing walkers")

# A new point the stand cannot reach takes the old one off the entry.
env, g, h = departing()
env.payload = {"stand": None, "pushback": ["Y2 Face W"]}
g["_standCheck"](h)
env.payload = {"stand": None, "pushback": ["Z9 Face N"]}
del LOG[:]
g["_standCheck"](h)
assert atc(env)["pos"] == LEFT and atc(env)["label"] == "ATC Assigned", atc(env)
env.state = 5
g["_standCheck"](h)
assert not any("Y2" in l for l in said()), "no reminder of the withdrawn point"
case("an unreachable new point withdraws the old one, and no reminder names it")

# A new callsign takes the old callsign's point back.
env, g, h = departing()
env.payload = {"stand": None, "pushback": ["Y2 Face W"]}
g["_standCheck"](h)
h._standCallsign, h._standCallsignTrusted = "OLD1", False
env.payload = {"stand": None, "pushback": None}
g["_standCheck"](h)
assert h._standCallsign == "DAT99"
assert atc(env)["pos"] == LEFT and atc(env)["label"] == "ATC Assigned", atc(env)
case("a callsign change withdraws the previous callsign's point")

# Past state 5 a missing entry is not appended: it would not be on the menu.
env, g, h = departing()
env.gate.rebuild()                              # GSX dropped our slot
env.state = 5
g["_standCheck"](h)
assert len(env.gate.pushbackAddPos) == 2, env.gate.menu()
case("no entry is appended to a menu already built")

# The entry was on, then GSX rebuilt the parking after the menu was built:
# the same assignment now has no entry, so the pilot is told the scenery
# route instead - once, not every poll.
env, g, h = departing()
env.payload = {"stand": None, "pushback": ["Y2 Face W"]}
g["_standCheck"](h)
env.state = 5
g["_standCheck"](h)
env.gate.rebuild()                              # GSX dropped our slot
del LOG[:]
g["_standCheck"](h)
assert said() == ['ATC pushback: Y2 Face W - pick "Y2  Face W" in the pushback menu'], LOG
g["_standCheck"](h)
assert said() == [], "told once"
case("an entry lost after the menu is built names the scenery route, once")

# A refused withdrawal is retried, not forgotten.
env, g, h = departing()
env.payload = {"stand": None, "pushback": ["Y2 Face W"]}
g["_standCheck"](h)
env.gate.refuse = 1
env.payload = {"stand": None, "pushback": None}
g["_standCheck"](h)
assert atc(env)["pos"] == RIGHT, "the refused write left it"
g["_standCheck"](h)
assert atc(env)["pos"] == LEFT, "and the next poll put it back"
case("a refused withdrawal is retried on the next poll")

# No position when the tug was called: anchor on the first one that can be read.
env = Env(copying=True)
g = load(env)
h = Handler()
g["onEnterAirport"](h)
lat = env.lat
env.lat = None
g["onAirportDepartureRequested"](h)
env.lat = lat
env.state = 4
g["_standCheck"](h)                             # anchors here
env.lat += 0.001
env.payload = {"stand": None, "pushback": ["Z7 Face W"]}
g["_standCheck"](h)
assert atc(env)["pos"] == LEFT, "the push was caught, and the entry left alone"
case("a push is still caught when no position was readable at the request")


# -- any profile ---------------------------------------------------------------
# An unmapped scenery gets only the bare point; the script finds its own routes
# to it by name, whatever naming the profile uses.

env, g, h = departing()
env.payload = {"stand": None, "pushback": None, "point": "Z7"}
g["_standCheck"](h)
assert atc(env)["pos"] == (4, 5, 6), atc(env)
assert env.gate.menu()[-1] == "ATC Assigned: Z7 Face W", env.gate.menu()
case("the bare point finds this stand's route named after it")

env, g, h = departing()
env.gate.pushbackLabels = ("Face E on Y1", "Release point Y2 (W)")
h._standGateSnapshot = None
env.payload = {"stand": None, "pushback": None, "point": "Y2"}
g["_standCheck"](h)
assert atc(env)["pos"] == RIGHT, atc(env)
assert env.gate.menu()[-1] == "ATC Assigned: Release point Y2 (W)", env.gate.menu()
case("other profiles' naming styles are found too")

# Labels mapped for another profile, absent at this stand: fall back to the point.
env, g, h = departing()
env.payload = {"stand": None, "pushback": ["Z7 EAST"], "point": "Z7"}
g["_standCheck"](h)
assert atc(env)["pos"] == (4, 5, 6), atc(env)
case("labels this stand lacks fall back to the point")

# A point with no route here: nothing moves, and the pilot hears its name.
env, g, h = departing()
env.payload = {"stand": None, "pushback": None, "point": "Z1"}
del LOG[:]
g["_standCheck"](h)
assert atc(env)["pos"] == LEFT and atc(env)["label"] == "ATC Assigned", atc(env)
assert said() == ["ATC assigned Z1 - no route to it from this stand"], LOG
case("a point this stand has no route to says so, by its own name")


# -- embedded in another profile's script ------------------------------------------
# The block may sit at the end of a profile's own script, which defines the same
# callbacks. The profile's runs first; ours still runs; a reload never loops.

PROFILE = ('def onAirportBeforeVehicleSelect(self):\n'
           '    print("profile gate setup")\n'
           'def onExitAirport(self):\n'
           '    print("profile exit")\n')
# Already embedded in a profile script: its own callbacks are the ones to see.
# The docs snippet is the block alone, so it gets the stand-in profile above.
EMBEDDED = "def " in SRC.split(">>> FlightStrips")[0] if ">>> FlightStrips" in SRC else False
env = Env()
g = load(env, src=SRC if EMBEDDED else PROFILE + SRC)
h = Handler()
del LOG[:]
g["onAirportBeforeVehicleSelect"](h)
if EMBEDDED:
    assert any(not l.startswith("[stands]") for l in LOG), LOG
else:
    assert "profile gate setup" in LOG, LOG
assert h._standPoll == "h", "and FlightStrips started its watch"
assert not any("failed" in l for l in LOG), LOG
g["onExitAirport"](h)
if not EMBEDDED:
    assert "profile exit" in LOG, LOG
case("a profile's own callbacks still run, before FlightStrips'")

exec(compile(SRC, "handler.py", "exec"), g)     # F5: the same namespace again
del LOG[:]
g["onAirportBeforeVehicleSelect"](h)
assert LOG.count("profile gate setup") <= 1, LOG
assert not any("failed" in l for l in LOG), LOG
case("reloading into the same namespace never chains FlightStrips to itself")


# -- by position ---------------------------------------------------------------
# Made-up stands on FlightStrips' own release point positions. Profiles name
# routes in different styles and sometimes after the wrong point; the server
# sends where the point is, and the route ending there is the answer.

# From backend/config/ekch/gsx_sceneries.json.
POINTS = {
    "S1": (55.626033, 12.653217), "S2": (55.625207, 12.653250),
    "Y0": (55.626680, 12.640100), "Y1": (55.626254, 12.641250),
    "Z1": (55.627813, 12.631736), "Z2": (55.627587, 12.632347),
    "W2": (55.624398, 12.654864),
}
RADIUS = 40
COMPASS = {"N": 0, "NE": 45, "E": 90, "SE": 135, "S": 180, "SW": 225, "W": 270, "NW": 315}


def at(point, heading, north=0.0):
    """A route end at a point (optionally nudged north, in degrees)."""
    lat, lon = POINTS[point]
    return (lat + north, lon, heading)


class PlannedGate(CopyingGate):
    """A stand from a list of (label, (lat, lon, heading)): two defaults, extras."""

    def __init__(self, routes):
        self.routes = routes
        self.uiGateName = "Gate X1"
        self.rebuild()

    def rebuild(self):
        defaults, extras = self.routes[:2], self.routes[2:]
        self.pushbackLabels = tuple(label for label, _ in defaults)
        self.pushback = 3
        self.pushbackLeftPos = defaults[0][1]
        self.pushbackRightPos = defaults[1][1]
        self.pushbackAddPos = [{"label": label, "pos": pos, "snap": False, "approach": [],
                                "wws": 2, "startEngines": 0.0} for label, pos in extras]


def planned(routes, src=SRC):
    """A departure under way at a made-up stand."""
    env = Env(copying=True)
    env.gate = PlannedGate(routes)
    g = load(env, src=src)
    h = Handler()
    g["onEnterAirport"](h)
    g["onAirportDepartureRequested"](h)
    env.state = 4
    return env, g, h


def target(point, facing=None):
    """What the server sends for a point, as it would."""
    if point in COMPASS:
        return {"lat": None, "lon": None, "heading": COMPASS[point], "radius_m": RADIUS,
                "avoid": [{"lat": lat, "lon": lon} for lat, lon in POINTS.values()]}
    lat, lon = POINTS[point]
    return {"lat": lat, "lon": lon, "radius_m": RADIUS,
            "heading": COMPASS[facing] if facing else None,
            "avoid": [{"lat": a, "lon": b} for name, (a, b) in POINTS.items() if name != point]}


def assign(env, g, h, point, facing=None):
    env.payload = {"stand": None, "pushback": None, "point": point,
                   "target": target(point, facing)}
    del LOG[:]
    g["_standCheck"](h)
    return env.gate.menu()[-1]


SHORT = [("R1 Face N", at("Y0", 0)), ("S1 Face N", at("S1", 0)),
         ("S2 Face S", at("S2", 180))]
LONG = [("Release Point S2 (SOUTH)", at("S2", 180)),
        ("Release Point S1 (NORTH)", at("S1", 0))]
for routes, want in ((SHORT, "S1 Face N"), (LONG, "Release Point S1 (NORTH)")):
    env, g, h = planned(routes)
    assert assign(env, g, h, "S1") == "ATC Assigned: " + want, env.gate.menu()
case("the route ending at the point, in each naming style")

env = Env(copying=True)
env.gate = PlannedGate([("S1 Left", at("S1", 270)), ("S1 Right", at("S1", 90))])
env.gate.pushback = 2
g = load(env)
h = Handler()
g["onEnterAirport"](h)
g["onAirportDepartureRequested"](h)
assert atc(env)["pos"] == at("S1", 90), atc(env)
assert assign(env, g, h, "S1", facing="W") == "ATC Assigned: S1 Right"
assert atc(env)["pos"] == at("S1", 90), atc(env)
case("the placeholder and geometry matching honor the profile's default route mask")

env = Env(copying=True)
env.gate = PlannedGate([("S1 Left", at("S1", 270)), ("S2 Right", at("S2", 90)),
                        ("S2 Extra", at("S2", 270))])
g = load(env)
h = Handler()
g["onEnterAirport"](h)
assign(env, g, h, "S1")  # Poll once before the host configures this departure.


def profile_departure_requested(_):
    env.gate.pushbackLeftPos = at("S2", 270)
    slots = env.gate.pushbackAddPos
    slots[0].update(label="S1 Profile Extra", pos=at("S1", 0), wws=1)
    env.gate.pushbackAddPos = slots


g["_standProfileDepartureRequested"] = profile_departure_requested
g["onAirportDepartureRequested"](h)
assert atc(env)["label"] == "ATC Assigned: S1 Profile Extra", env.gate.menu()
assert atc(env)["pos"] == at("S1", 0) and atc(env)["wws"] == 1, atc(env)
case("embedded departure callbacks can replace routes without changing the slot count")

env, g, h = planned([("Z main L Face E", at("S2", 90)), ("Z/L Face E", at("S1", 90))])
env.payload = {"stand": None, "pushback": None, "point": "Z/L"}
g["_standCheck"](h)
assert atc(env)["label"] == "ATC Assigned: Z/L Face E" and atc(env)["pos"] == at("S1", 90), atc(env)
case("name fallback matches consecutive point tokens separated by punctuation")

# Named after S1, ends at S2: by position it is S2's route, and the pilot is
# told S2 - the name would have said S1.
MISLABEL = [("S1 Face N", at("S1", 0)), ("S1 Face S", at("S2", 180))]
env, g, h = planned(MISLABEL)
assert assign(env, g, h, "S2") == "ATC Assigned: S2", env.gate.menu()
assert said() == ['ATC pushback: S2 - pick "ATC Assigned: S2" when GSX asks which way to push'], LOG
env, g, h = planned(MISLABEL)
assert assign(env, g, h, "S1") == "ATC Assigned: S1 Face N", env.gate.menu()
case("a mislabelled route is used for where it goes, not what it is called")

# Two routes end at Y1, one of them named after Y0. Y0 is only the one that
# ends there; the facing rule picks between the two at Y1.
TWO = [("Y0 Face E", at("Y0", 123)), ("Y1 Face E", at("Y1", 123)),
       ("Y0 Face W", at("Y1", -57))]
env, g, h = planned(TWO)
assert assign(env, g, h, "Y0") == "ATC Assigned: Y0 Face E", env.gate.menu()
env, g, h = planned(TWO)
assert assign(env, g, h, "Y1", facing="E") == "ATC Assigned: Y1 Face E", env.gate.menu()
env, g, h = planned(TWO)
assert assign(env, g, h, "Y1", facing="W") == "ATC Assigned: Y1", env.gate.menu()
case("two routes ending at one point: the facing rule picks, by heading")

# The rule wants east; this profile only offers the point facing west.
# Position still wins - the push goes to the point - and the facing yields.
WEST_ONLY = [("Z1 Face W", at("Z1", -57)), ("S1 Face N", at("S1", 0))]
env, g, h = planned(WEST_ONLY)
assert assign(env, g, h, "Z1", facing="E") == "ATC Assigned: Z1 Face W", env.gate.menu()
case("a facing the profile cannot offer falls back to the route at the point")

# A bare direction is the taxiway: a route ending away from every point beats
# one ending at W2, where both face that way.
BARE = [("Face West", at("Z1", 270, north=0.004)), ("W2 Face W", at("W2", 270)),
        ("S1 Face N", at("S1", 0))]
env, g, h = planned(BARE)
assert assign(env, g, h, "W") == "ATC Assigned: Face West", env.gate.menu()
env, g, h = planned(BARE[1:])
assert assign(env, g, h, "W") == "ATC Assigned: W2 Face W", env.gate.menu()
case("a direction takes the bare-taxiway route, or the only route that way")

# No route at the point: a route merely named after it is never taken.
NAMED_ONLY = [("Z1 Face E", at("S1", 0)), ("S2 Face S", at("S2", 180))]
env, g, h = planned(NAMED_ONLY)
assign(env, g, h, "Z1")
assert env.gate.menu()[-1] == "ATC Assigned", env.gate.menu()
assert said() == ["ATC assigned Z1 - no route to it from this stand"], LOG
case("no route at the point: nothing is guessed, whatever the names say")

# Exactly named after the point, but ending elsewhere: position rejected it,
# so its name must not bring it back.
EXACT = [("S1", at("S2", 180)), ("S2 Face S", at("S2", 180))]
env, g, h = planned(EXACT)
assign(env, g, h, "S1")
assert env.gate.menu()[-1] == "ATC Assigned", env.gate.menu()
assert said() == ["ATC assigned S1 - no route to it from this stand"], LOG
case("a route called exactly the point, ending elsewhere, is still not taken")

# Two routes share a name; position picks the second, and the entry goes there
# rather than to the first route with that name.
TWINS = [("Push", at("S2", 180)), ("S2 Face S", at("S2", 180)),
         ("Push", at("S1", 0))]
env, g, h = planned(TWINS)
assert assign(env, g, h, "S1") == "ATC Assigned: S1", env.gate.menu()
entry = env.gate._addPos[-1]
assert tuple(entry["pos"][:2]) == POINTS["S1"], entry
case("two routes with one name: the entry goes where position chose")

# The entry is lost after the menu is built, then ATC moves the point to
# another route with the same name: still a change, so the pilot is told.
TWIN_PUSH = [("S2 Face S", at("S2", 180)), ("Y0 Face E", at("Y0", 123)),
             ("Push", at("S1", 0)), ("Push", at("Z1", 90))]
env, g, h = planned(TWIN_PUSH)
assign(env, g, h, "S1")
env.state = 5
g["_standCheck"](h)
env.gate.rebuild()                              # GSX dropped our slot
assign(env, g, h, "S1")
assert said() == ['ATC pushback: S1 - pick "Push" in the pushback menu'], LOG
assign(env, g, h, "S1")
assert said() == [], "told once"
assign(env, g, h, "Z1")
assert said() == ['ATC changed your push to Z1 - pick "Push" in the pushback menu'], LOG
case("a lost entry is re-announced when the point moves to a same-named route")

COINCIDENT = [("Push", at("S1", 90)), ("S2 Face S", at("S2", 180)),
              ("Push", at("S1", 270))]
for point, facing in (("S1", "W"), ("W", None)):
    env, g, h = planned(COINCIDENT)
    extra = env.gate.pushbackAddPos[0]
    extra["approach"] = [(55.626, 12.653, 270)]
    extra["wws"] = 1
    env.gate.pushbackAddPos = [extra] + env.gate.pushbackAddPos[1:]
    assign(env, g, h, point, facing)
    entry = env.gate.pushbackAddPos[-1]
    assert entry["pos"] == at("S1", 270), entry
    assert entry["approach"] == extra["approach"] and entry["wws"] == 1, entry
case("duplicate names and endpoints preserve the slot, heading, approach and settings")

OVERLAP = [("Z1", (55.62762638, 12.63224052, 270)), ("S2 Face S", at("S2", 180))]
env, g, h = planned(OVERLAP)
assert assign(env, g, h, "Z1") == "ATC Assigned", env.gate.menu()
assert said() == ["ATC assigned Z1 - no route to it from this stand"]
assert assign(env, g, h, "Z2") == "ATC Assigned: Z2", env.gate.menu()
case("a route in two matching radii belongs only to the nearer point")

for src in (MARK_ONLY, NARROW_ONLY):
    env, g, h = planned(EXACT, src=src)
    original = env.gate.menu()
    assign(env, g, h, "S2")
    assert env.gate.menu() != original, env.gate.menu()
    assign(env, g, h, "S1")
    assert env.gate.menu() == original, env.gate.menu()
    assert said() == ["ATC assigned S1 - no route to it from this stand"]
case("geometry rejection clears the previous mark and never marks an exact-name match")

for already_restored in (False, True):
    env, g, h = planned([("S2 Left", at("S2", 180)), ("S2 Right", at("S2", 0)),
                         ("S1 Extra", at("S1", 90))], src=NARROW_ONLY)
    original = env.gate.menu()
    assign(env, g, h, "S1")
    assert env.gate.menu() == ["ATC Assigned"], env.gate.menu()
    env.state = 5
    if already_restored:
        g["_standCheck"](h)
    assign(env, g, h, "Z1")
    if not already_restored:
        assert env.gate.menu() == original, env.gate.menu()
    assert env.gate.pushback == 3 and env.gate.pushbackAddPos[0]["pos"] == at("S1", 90)
    assert said() == ["ATC assigned Z1 - no route to it from this stand - "
                      "disregard the old mark, cancel and request pushback again"]
    g["_standCheck"](h)
    assert not said(), "report a rejected reassignment only once"
case("commitment restores rejected extras cleanly and warns when the visible menu may be stale")

env, g, h = planned([("S2 Left", at("S2", 180)), ("S2 Right", at("S2", 0)),
                     ("S1 Extra", at("S1", 90))], src=NARROW_ONLY)
assign(env, g, h, "S1")
assert env.gate.pushback == 0
env.state = 5


def fail_fetch(*args, **kwargs):
    raise RuntimeError("timed out")


g["fetchJson"] = fail_fetch
try:
    g["_standCheck"](h)
except RuntimeError:
    pass
assert env.gate.pushback == 3 and h._standMenuRestored
case("a failed poll still restores the narrowed menu at commitment")

env, g, h = planned([("Push", at("S1", 90)), ("Push", at("S1", 270))], src=MARK_ONLY)
assign(env, g, h, "S1", facing="W")
assert env.gate.pushbackLabels == ("Push", "ATC Assigned"), env.gate.pushbackLabels
assert g["_standPushbackMarked"](h, ["Push"])
env, g, h = planned(COINCIDENT, src=NARROW_ONLY)
assign(env, g, h, "S1", facing="W")
assert env.gate.pushback == 0 and len(env.gate.pushbackAddPos) == 1, env.gate.menu()
assert env.gate.pushbackAddPos[0]["pos"] == at("S1", 270)
case("marking and narrowing use the selected slot for coincident routes")

env, g, h = planned([("Push", at("S1", 90)), ("Push", at("S1", 270))], src=NARROW_ONLY)
assign(env, g, h, "S1", facing="W")
assert env.gate.pushback == 2, env.gate.menu()
env.state = 5
g["_standCheck"](h)
assign(env, g, h, "S1", facing="E")
assert env.gate.pushbackRightPos == at("S1", 90)
assert any("your push now goes there" in message for message in said())
case("after narrowing, a same-label reassignment redirects the slot offered")

print("\nall passed")
