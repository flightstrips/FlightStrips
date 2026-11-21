package arrivalinfo

import (
	"context"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc"
)

const testSession int32 = 7

type sentTelex struct{ from, to, packet string }

type fakeSender struct {
	mu   sync.Mutex
	sent []sentTelex
	fail error
}

func (f *fakeSender) SendTelex(_ context.Context, from, to, packet string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return f.fail
	}
	f.sent = append(f.sent, sentTelex{from, to, packet})
	return nil
}

type fakeSessions struct{ sessions map[int32]*models.Session }

func (f fakeSessions) GetByID(_ context.Context, id int32) (*models.Session, error) {
	return f.sessions[id], nil
}

type fakeStrips struct{ strips map[int32][]*models.Strip }

func (f *fakeStrips) List(_ context.Context, session int32) ([]*models.Strip, error) {
	return f.strips[session], nil
}

type fakeStore struct {
	mu           sync.Mutex
	nextID       int64
	messages     []SentMessage
	afterReserve func()
}

func (f *fakeStore) ListEpisode(_ context.Context, session int32, episode string) ([]SentMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []SentMessage
	for _, message := range f.messages {
		if message.Session == session && message.Episode == episode {
			result = append(result, message)
		}
	}
	return result, nil
}

func (f *fakeStore) Reserve(_ context.Context, message SentMessage) (int64, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if message.Kind == KindArrivalInfo {
		for _, existing := range f.messages {
			if existing.Session == message.Session && existing.Episode == message.Episode && existing.Kind == KindArrivalInfo {
				return 0, false, nil
			}
		}
	}
	f.nextID++
	message.ID = f.nextID
	f.messages = append(f.messages, message)
	if f.afterReserve != nil {
		f.afterReserve()
	}
	return message.ID, true, nil
}

func (f *fakeStore) Release(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for index, message := range f.messages {
		if message.ID == id {
			f.messages = append(f.messages[:index], f.messages[index+1:]...)
			return nil
		}
	}
	return nil
}

type harness struct {
	t                *testing.T
	now              time.Time
	sender           *fakeSender
	sessions         fakeSessions
	strips           *fakeStrips
	store            *fakeStore
	service          *Service
	strip            *models.Strip
	flight           aman.AMANFlight
	enabled          bool
	authorityAllowed bool
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{
		t:                t,
		now:              time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC),
		sender:           &fakeSender{},
		sessions:         fakeSessions{sessions: map[int32]*models.Session{testSession: {ID: testSession, Name: "LIVE", Airport: "EKCH"}}},
		store:            &fakeStore{},
		enabled:          true,
		authorityAllowed: true,
		strip:            &models.Strip{ID: 42, Callsign: "SAS123", Session: testSession, Stand: ptr("C27"), Star: ptr("MONAK5C"), PositionAltitude: ptr(int32(9000))},
	}
	h.strips = &fakeStrips{strips: map[int32][]*models.Strip{testSession: {h.strip}}}
	h.flight = aman.AMANFlight{Callsign: "SAS123", State: aman.StateUnstable}
	h.setTiming(50*time.Minute, 10*time.Minute)
	h.service = h.newService()
	return h
}

func (h *harness) newService() *Service {
	service, err := New(Dependencies{
		Sender: h.sender, Sessions: h.sessions, Strips: h.strips, Store: h.store,
		Terminal:         testTerminal(),
		AirportEnabled:   func(string) bool { return h.enabled },
		AuthorityAllowed: func(context.Context) bool { return h.authorityAllowed },
		Now:              func() time.Time { return h.now },
	})
	if err != nil {
		h.t.Fatal(err)
	}
	return service
}

// setTiming places the operational ETA tetaIn from now with the given delay.
func (h *harness) setTiming(tetaIn, delay time.Duration) {
	teta := h.now.Add(tetaIn)
	h.flight.Prediction = &aman.Prediction{OperationalTETA: teta}
	h.flight.Slot = &aman.Slot{Time: teta.Add(delay)}
}

func (h *harness) advance(d time.Duration) {
	h.now = h.now.Add(d)
}

func (h *harness) tick() {
	h.service.ObserveAMANState(h.snapshot())
	h.service.Tick(context.Background())
}

func (h *harness) snapshot() aman.AirportState {
	return aman.AirportState{SessionID: testSession, Airport: "EKCH", Authoritative: true, Flights: []aman.AMANFlight{h.flight}}
}

func (h *harness) expectSent(packets ...string) {
	h.t.Helper()
	h.sender.mu.Lock()
	defer h.sender.mu.Unlock()
	if len(h.sender.sent) != len(packets) {
		h.t.Fatalf("sent %d telex %+v, want %d %q", len(h.sender.sent), h.sender.sent, len(packets), packets)
	}
	for index, packet := range packets {
		got := h.sender.sent[index]
		if got.packet != packet || got.from != "EKCH" || got.to != "SAS123" {
			h.t.Fatalf("telex %d = %+v, want EKCH -> SAS123 %q", index, got, packet)
		}
	}
}

const infoC27 = "EKCH ARR INFO: EXP STAND C27. MONAK5C. DELAY 5-10MIN."

// sendInfo moves the flight to the 40-minute trigger and sends message 1.
func (h *harness) sendInfo() {
	h.t.Helper()
	h.setTiming(40*time.Minute, 10*time.Minute)
	h.tick()
	h.expectSent(infoC27)
}

func TestArrivalInfoFiresOnceAtFortyMinutes(t *testing.T) {
	h := newHarness(t)
	h.setTiming(40*time.Minute+time.Second, 10*time.Minute)
	h.tick()
	h.expectSent()

	h.setTiming(40*time.Minute, 10*time.Minute)
	h.tick()
	h.expectSent(infoC27)

	h.flight.State = aman.StateStable
	h.advance(time.Minute)
	h.setTiming(30*time.Minute, 25*time.Minute)
	h.tick()
	h.expectSent(infoC27)

	if got := len(h.store.messages); got != 1 || h.store.messages[0].Kind != KindArrivalInfo || h.store.messages[0].Stand != "C27" || h.store.messages[0].Episode != "legacy/SAS123" {
		t.Fatalf("persisted %+v", h.store.messages)
	}
}

func TestArrivalInfoNotResentAfterGoAroundOrRestart(t *testing.T) {
	h := newHarness(t)
	h.sendInfo()

	h.flight.State = aman.StateGoAround
	h.advance(20 * time.Minute)
	h.tick()
	h.flight.State = aman.StateUnstable
	h.setTiming(25*time.Minute, 0)
	h.tick()
	h.expectSent(infoC27)

	h.service = h.newService()
	h.tick()
	h.expectSent(infoC27)
}

func TestArrivalInfoSkippedAfterConfirmedGoAroundWhenNeverSent(t *testing.T) {
	h := newHarness(t)
	h.flight.GoAroundConfirmation = &aman.GoAroundConfirmation{Status: aman.GoAroundConfirmationConfirmed}
	h.setTiming(30*time.Minute, 0)
	h.tick()
	h.expectSent()

	// The detector prompt can be cleared before resequencing, and a manual
	// report can record the controller command without ever creating a prompt.
	h.flight.GoAroundConfirmation = nil
	h.flight.GoAroundDetection = &aman.GoAroundDetectionState{LastControllerCommandID: "report-1"}
	h.tick()
	h.expectSent()
}

func TestArrivalInfoUsesNewEpisodeWhenCallsignAndStripAreReused(t *testing.T) {
	h := newHarness(t)
	h.sendInfo()

	h.advance(time.Hour)
	h.flight.ArrivalEpisodeID = "SAS123/new-arrival"
	h.strip.Stand = ptr("D2")
	h.setTiming(35*time.Minute, 10*time.Minute)
	h.service = h.newService()
	h.tick()
	h.expectSent(infoC27, "EKCH ARR INFO: EXP STAND D2. MONAK5C. DELAY 5-10MIN.")

	h.service = h.newService()
	h.tick()
	h.expectSent(infoC27, "EKCH ARR INFO: EXP STAND D2. MONAK5C. DELAY 5-10MIN.")
}

func TestArrivalInfoLegacyEpisodeSurvivesStripReplacement(t *testing.T) {
	h := newHarness(t)
	h.sendInfo()
	h.strip.ID++
	h.service = h.newService()
	h.tick()
	h.expectSent(infoC27)
}

func TestArrivalInfoWaitsForUnresolvedEpisode(t *testing.T) {
	h := newHarness(t)
	h.flight.ArrivalEpisodeID = "provisional"
	h.setTiming(35*time.Minute, 10*time.Minute)
	state := h.snapshot()
	state.DisconnectedArrivals = map[aman.Callsign]aman.DisconnectedArrival{
		h.flight.Callsign: {EpisodeID: "old-arrival", GoAroundCommandID: "report"},
	}
	h.service.ObserveAMANState(state)
	h.service.Tick(t.Context())
	h.expectSent()
}

func TestArrivalInfoRequiresUnstableOrStableWithSlot(t *testing.T) {
	h := newHarness(t)
	h.setTiming(35*time.Minute, 0)
	for _, state := range []aman.FlightState{aman.StatePlanned, aman.StateAirborne, aman.StateGoAround, aman.StateLanded, aman.StateRemoved} {
		h.flight.State = state
		h.tick()
	}
	h.expectSent()

	h.flight.State = aman.StateStable
	h.flight.Slot = nil
	h.tick()
	h.expectSent()

	h.setTiming(35*time.Minute, 0)
	h.tick()
	h.expectSent("EKCH ARR INFO: EXP STAND C27. MONAK5C. NO DELAY EXPECTED.")
}

func TestLateAppearingFlight(t *testing.T) {
	t.Run("sent as soon as it has a slot", func(t *testing.T) {
		h := newHarness(t)
		h.setTiming(25*time.Minute, 0)
		h.flight.Slot = nil
		h.tick()
		h.expectSent()
		h.setTiming(25*time.Minute, 3*time.Minute)
		h.tick()
		h.expectSent("EKCH ARR INFO: EXP STAND C27. MONAK5C. NO DELAY EXPECTED.")
	})
	t.Run("just outside the cutoff", func(t *testing.T) {
		h := newHarness(t)
		h.setTiming(15*time.Minute, time.Second)
		h.tick()
		h.expectSent("EKCH ARR INFO: EXP STAND C27. MONAK5C. NO DELAY EXPECTED.")
	})
	t.Run("within fifteen minutes of landing", func(t *testing.T) {
		h := newHarness(t)
		h.setTiming(15*time.Minute, 0)
		h.tick()
		h.setTiming(10*time.Minute, 0)
		h.tick()
		h.expectSent()
	})
	t.Run("delayed flight uses the landing slot for the cutoff", func(t *testing.T) {
		h := newHarness(t)
		h.setTiming(10*time.Minute, 25*time.Minute)
		h.tick()
		h.expectSent("EKCH ARR INFO: EXP STAND C27. MONAK5C. DELAY MORE THAN 20MIN.")
	})
}

func TestArrivalInfoMissingStandAndSTAR(t *testing.T) {
	h := newHarness(t)
	h.strip.Stand = nil
	h.strip.Star = nil
	h.setTiming(35*time.Minute, 8*time.Minute)
	h.tick()
	h.expectSent("EKCH ARR INFO: DELAY 5-10MIN.")
}

func TestArrivalInfoDerivesSTARFromFamily(t *testing.T) {
	h := newHarness(t)
	h.strip.Star = nil
	group := aman.RunwayGroupID("ARRIVAL-22L")
	h.flight.SelectedSTARFamily = ptr("MONAK")
	h.flight.SelectedRunwayGroup = &group
	h.setTiming(35*time.Minute, 13*time.Minute)
	h.tick()
	h.expectSent("EKCH ARR INFO: EXP STAND C27. MONAK5C. DELAY 10-20MIN.")
}

func TestArrivalInfoGates(t *testing.T) {
	t.Run("not LIVE", func(t *testing.T) {
		h := newHarness(t)
		h.sessions.sessions[testSession].Name = "PLAYBACK"
		h.setTiming(35*time.Minute, 0)
		h.tick()
		h.expectSent()
	})
	t.Run("pdc capability disabled", func(t *testing.T) {
		h := newHarness(t)
		h.enabled = false
		h.setTiming(35*time.Minute, 0)
		h.tick()
		h.expectSent()
	})
	t.Run("removed session", func(t *testing.T) {
		h := newHarness(t)
		h.service.RemoveSession(testSession)
		h.setTiming(35*time.Minute, 0)
		h.tick()
		h.expectSent()
	})
	t.Run("uses the session airport as station", func(t *testing.T) {
		h := newHarness(t)
		h.sessions.sessions[testSession].Airport = "ESSA"
		h.setTiming(35*time.Minute, 0)
		state := h.snapshot()
		state.Airport = "ESSA"
		h.service.ObserveAMANState(state)
		h.service.Tick(context.Background())
		if len(h.sender.sent) != 1 || h.sender.sent[0].from != "ESSA" || h.sender.sent[0].packet != "ESSA ARR INFO: EXP STAND C27. MONAK5C. NO DELAY EXPECTED." {
			t.Fatalf("sent %+v", h.sender.sent)
		}
	})
}

func TestArrivalInfoDefinitelyUnacceptedTelexReleasesReservation(t *testing.T) {
	for _, failure := range []error{pdc.ErrHoppieRejected, pdc.ErrHoppieNotSent} {
		t.Run(failure.Error(), func(t *testing.T) {
			h := newHarness(t)
			h.sender.fail = failure
			h.setTiming(35*time.Minute, 10*time.Minute)
			h.tick()
			if len(h.store.messages) != 0 {
				t.Fatalf("failed send left %+v", h.store.messages)
			}
			h.sender.fail = nil
			h.tick()
			h.expectSent(infoC27)
		})
	}
}

func TestArrivalTelexKeepsReservationAfterAmbiguousSend(t *testing.T) {
	t.Run("arrival info", func(t *testing.T) {
		h := newHarness(t)
		h.sender.fail = context.DeadlineExceeded
		h.setTiming(35*time.Minute, 10*time.Minute)
		h.tick()
		h.sender.fail = nil
		h.tick()
		h.service = h.newService()
		h.tick()
		h.expectSent()
		if len(h.store.messages) != 1 {
			t.Fatalf("persisted %+v", h.store.messages)
		}
	})
	t.Run("stand update", func(t *testing.T) {
		h := newHarness(t)
		h.sendInfo()
		h.strip.Stand = ptr("D2")
		h.tick()
		h.advance(StandDebounce)
		h.sender.fail = context.DeadlineExceeded
		h.tick()
		h.sender.fail = nil
		h.tick()
		h.service = h.newService()
		h.tick()
		h.advance(StandDebounce)
		h.tick()
		h.expectSent(infoC27)
		if len(h.store.messages) != 2 || h.store.messages[1].Stand != "D2" {
			t.Fatalf("persisted %+v", h.store.messages)
		}
	})
}

func TestArrivalInfoUsesLatestRevisionAndEqualRevisionAuthority(t *testing.T) {
	h := newHarness(t)
	h.setTiming(35*time.Minute, 10*time.Minute)
	older := h.snapshot()
	older.Revision = 1
	latest := h.snapshot()
	latest.Revision, latest.Authoritative = 2, false
	h.service.ObserveAMANState(latest)
	h.service.ObserveAMANState(older)
	h.service.Tick(t.Context())
	h.expectSent()

	latest.Authoritative = true
	h.service.ObserveAMANState(latest)
	h.service.Tick(t.Context())
	h.expectSent(infoC27)
}

func TestArrivalInfoDiscardsAnOlderAuthoritativeSnapshot(t *testing.T) {
	h := newHarness(t)
	h.setTiming(35*time.Minute, 10*time.Minute)
	older := h.snapshot()
	older.Revision = 1
	latest := h.snapshot()
	latest.Revision = 2
	latest.Flights[0].Slot = nil
	h.service.ObserveAMANState(latest)
	h.service.ObserveAMANState(older)
	h.service.Tick(t.Context())
	h.expectSent()
}

func TestArrivalInfoCancelsSendWhenAuthorityOrRevisionChangesDuringReserve(t *testing.T) {
	for _, change := range []string{"health loss", "authority publication", "newer revision"} {
		t.Run(change, func(t *testing.T) {
			h := newHarness(t)
			h.setTiming(35*time.Minute, 10*time.Minute)
			h.store.afterReserve = func() {
				state := h.snapshot()
				switch change {
				case "health loss":
					h.authorityAllowed = false
				case "authority publication":
					state.Authoritative = false
					h.service.ObserveAMANState(state)
				case "newer revision":
					state.Revision++
					h.service.ObserveAMANState(state)
				}
			}
			h.tick()
			h.expectSent()
			if len(h.store.messages) != 0 {
				t.Fatalf("unsent telex left %+v", h.store.messages)
			}
		})
	}
}

func TestStandUpdateAfterDebounceAboveThreeThousandFeet(t *testing.T) {
	h := newHarness(t)
	h.sendInfo()

	h.strip.Stand = ptr("D2")
	h.tick()
	h.advance(29 * time.Second)
	h.tick()
	h.expectSent(infoC27)

	h.advance(time.Second)
	h.tick()
	h.expectSent(infoC27, "EKCH ARR INFO: NEW STAND D2.")

	h.advance(time.Minute)
	h.tick()
	h.expectSent(infoC27, "EKCH ARR INFO: NEW STAND D2.")
	last := h.store.messages[len(h.store.messages)-1]
	if last.Kind != KindStandUpdate || last.Stand != "D2" {
		t.Fatalf("persisted %+v", last)
	}
}

func TestStandUpdateAltitudeGate(t *testing.T) {
	for name, altitude := range map[string]*int32{
		"below 3000 ft": ptr(int32(2500)),
		"at 3000 ft":    ptr(int32(3000)),
		"unknown":       nil,
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			h.sendInfo()
			h.strip.PositionAltitude = altitude
			h.strip.Stand = ptr("D2")
			h.tick()
			h.advance(time.Minute)
			h.tick()
			h.expectSent(infoC27)
		})
	}
	t.Run("just above 3000 ft", func(t *testing.T) {
		h := newHarness(t)
		h.sendInfo()
		h.strip.PositionAltitude = ptr(int32(3001))
		h.strip.Stand = ptr("D2")
		h.tick()
		h.advance(30 * time.Second)
		h.tick()
		h.expectSent(infoC27, "EKCH ARR INFO: NEW STAND D2.")
	})
}

func TestStandUpdateDoesNotSendAfterLandingOrRemoval(t *testing.T) {
	for _, state := range []aman.FlightState{aman.StateLanded, aman.StateRemoved} {
		t.Run(string(state), func(t *testing.T) {
			h := newHarness(t)
			h.sendInfo()
			h.strip.Stand = ptr("D2")
			h.tick()
			h.advance(StandDebounce)
			h.flight.State = state
			h.tick()
			h.expectSent(infoC27)

			h.flight.State = aman.StateUnstable
			h.tick()
			h.expectSent(infoC27)
			h.advance(StandDebounce)
			h.tick()
			h.expectSent(infoC27, "EKCH ARR INFO: NEW STAND D2.")
		})
	}
}

func TestStandUpdateDebounceSendsOnlyTheFinalStand(t *testing.T) {
	h := newHarness(t)
	h.sendInfo()

	h.strip.Stand = ptr("D2")
	h.tick()
	h.advance(20 * time.Second)
	h.strip.Stand = ptr("D3")
	h.tick()
	h.advance(20 * time.Second)
	h.tick()
	h.expectSent(infoC27)

	h.advance(10 * time.Second)
	h.tick()
	h.expectSent(infoC27, "EKCH ARR INFO: NEW STAND D3.")
}

func TestStandUpdateRevertToLastToldStandSendsNothing(t *testing.T) {
	h := newHarness(t)
	h.sendInfo()

	h.strip.Stand = ptr("D2")
	h.tick()
	h.advance(10 * time.Second)
	h.strip.Stand = ptr("C27")
	h.tick()
	h.advance(time.Minute)
	h.tick()
	h.expectSent(infoC27)
}

func TestStandUpdateIgnoresStageChangeAndRemoval(t *testing.T) {
	h := newHarness(t)
	h.sendInfo()

	// ESTIMATED -> ASSIGNED -> CONFIRMED keeps the same stand designator.
	h.strip.Stand = ptr("c27")
	h.tick()
	h.advance(time.Minute)
	h.tick()

	h.strip.Stand = nil
	h.tick()
	h.advance(time.Minute)
	h.tick()
	h.expectSent(infoC27)
}

func TestStandUpdateNeverSameStandTwiceButEveryRealChange(t *testing.T) {
	h := newHarness(t)
	h.sendInfo()

	for _, stand := range []string{"D2", "C27", "D2"} {
		h.strip.Stand = ptr(stand)
		h.tick()
		h.advance(30 * time.Second)
		h.tick()
		h.advance(time.Minute)
		h.tick()
	}
	h.expectSent(infoC27, "EKCH ARR INFO: NEW STAND D2.", "EKCH ARR INFO: NEW STAND C27.", "EKCH ARR INFO: NEW STAND D2.")
}

func TestStandUpdateNeverBeforeArrivalInfo(t *testing.T) {
	h := newHarness(t)
	h.setTiming(60*time.Minute, 0)
	h.tick()
	h.strip.Stand = ptr("D2")
	h.tick()
	h.advance(time.Minute)
	h.tick()
	h.expectSent()
}

func TestStandUpdateAfterNoStandInArrivalInfo(t *testing.T) {
	h := newHarness(t)
	h.strip.Stand = nil
	h.setTiming(40*time.Minute, 10*time.Minute)
	h.tick()
	h.strip.Stand = ptr("B5")
	h.tick()
	h.advance(30 * time.Second)
	h.tick()
	h.expectSent("EKCH ARR INFO: MONAK5C. DELAY 5-10MIN.", "EKCH ARR INFO: NEW STAND B5.")
}

func TestStandUpdateLastToldStandSurvivesRestart(t *testing.T) {
	h := newHarness(t)
	h.sendInfo()
	h.strip.Stand = ptr("D2")
	h.tick()
	h.advance(30 * time.Second)
	h.tick()

	h.service = h.newService()
	h.advance(time.Minute)
	h.tick()
	h.advance(time.Minute)
	h.tick()
	h.expectSent(infoC27, "EKCH ARR INFO: NEW STAND D2.")
}
