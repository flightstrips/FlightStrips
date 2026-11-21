package arrivalinfo

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/terminal"
	"FlightStrips/internal/dependencies"
	"FlightStrips/internal/models"
)

const (
	// TriggerHorizon sends arrival information once the operational ETA is
	// this close, after roughly five minutes as an unstable flight.
	TriggerHorizon = 40 * time.Minute
	// LateCutoff skips arrival information for a flight first seen this close
	// to its landing slot.
	LateCutoff = 15 * time.Minute
	// StandDebounce waits for the stand allocator to settle before telling
	// the pilot about a new stand.
	StandDebounce = 30 * time.Second
	// StandUpdateMinAltitudeFeet: below this the crew is busy on approach.
	StandUpdateMinAltitudeFeet = 3000

	defaultInterval = 5 * time.Second
)

// TelexSender is the Hoppie client. Telex is used rather than CPDLC because
// arrivals are normally logged on to an ACC or approach station.
type TelexSender interface {
	SendTelex(ctx context.Context, from, to, packet string) error
}

type SessionReader interface {
	GetByID(ctx context.Context, id int32) (*models.Session, error)
}

type StripLister interface {
	List(ctx context.Context, session int32) ([]*models.Strip, error)
}

// SentMessage is one persisted automatic telex.
type SentMessage struct {
	ID       int64
	Session  int32
	Callsign string
	// Episode identifies one arrival. It is the strip id, which survives a
	// go-around but changes for a later flight with the same callsign.
	Episode int64
	Kind    Kind
	Text    string
	Stand   string
	SentAt  time.Time
}

// Store persists sent messages so a restart never sends a duplicate.
type Store interface {
	// ListEpisode returns the episode's messages oldest first.
	ListEpisode(ctx context.Context, session int32, episode int64) ([]SentMessage, error)
	// Reserve records a message before it is sent. For arrival information it
	// returns inserted=false when the episode already has one.
	Reserve(ctx context.Context, message SentMessage) (id int64, inserted bool, err error)
	// Release removes a reservation whose telex was not accepted.
	Release(ctx context.Context, id int64) error
}

type Dependencies struct {
	Sender   TelexSender
	Sessions SessionReader
	Strips   StripLister
	Store    Store
	// Terminal derives the full STAR from a STAR family. Optional.
	Terminal *terminal.Configuration
	// AirportEnabled reports the airport's "pdc" capability.
	AirportEnabled func(airport string) bool
	Now            func() time.Time
	Interval       time.Duration
}

type episodeKey struct {
	session int32
	episode int64
}

type episodeState struct {
	infoSent bool
	// lastStand is the stand the pilot was last told ("" when none).
	lastStand    string
	pendingStand string
	pendingSince time.Time
}

// Service evaluates the latest published AMAN state for each LIVE session.
type Service struct {
	deps Dependencies

	mu       sync.Mutex
	states   map[int32]aman.AirportState
	episodes map[episodeKey]*episodeState
	removed  map[int32]struct{}
}

func New(deps Dependencies) (*Service, error) {
	for _, dependency := range []struct {
		name  string
		value any
	}{{"telex sender", deps.Sender}, {"session reader", deps.Sessions}, {"strip lister", deps.Strips}, {"message store", deps.Store}} {
		if dependencies.IsNil(dependency.value) {
			return nil, errors.New("arrival info requires " + dependency.name)
		}
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.Interval <= 0 {
		deps.Interval = defaultInterval
	}
	return &Service{
		deps:     deps,
		states:   make(map[int32]aman.AirportState),
		episodes: make(map[episodeKey]*episodeState),
		removed:  make(map[int32]struct{}),
	}, nil
}

// ObserveAMANState keeps the latest state of a session. It never blocks on I/O
// so AMAN publication is not delayed by Hoppie.
func (s *Service) ObserveAMANState(state aman.AirportState) {
	if s == nil || state.SessionID <= 0 {
		return
	}
	state.Flights = append([]aman.AMANFlight(nil), state.Flights...)
	s.mu.Lock()
	if _, removed := s.removed[state.SessionID]; !removed {
		s.states[state.SessionID] = state
	}
	s.mu.Unlock()
}

// RemoveSession forgets a session that the server removed.
func (s *Service) RemoveSession(session int32) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.states, session)
	s.removed[session] = struct{}{}
	for key := range s.episodes {
		if key.session == session {
			delete(s.episodes, key)
		}
	}
}

func (s *Service) Start(ctx context.Context) {
	ticker := time.NewTicker(s.deps.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.Tick(ctx)
		}
	}
}

// Tick evaluates every observed session once.
func (s *Service) Tick(ctx context.Context) {
	s.mu.Lock()
	sessions := make([]int32, 0, len(s.states))
	for session := range s.states {
		sessions = append(sessions, session)
	}
	s.mu.Unlock()
	sort.Slice(sessions, func(i, j int) bool { return sessions[i] < sessions[j] })
	for _, session := range sessions {
		s.mu.Lock()
		state, ok := s.states[session]
		s.mu.Unlock()
		if !ok {
			continue
		}
		if err := s.processSession(ctx, state); err != nil {
			slog.WarnContext(ctx, "Arrival info: session evaluation failed", slog.Int("session", int(session)), slog.Any("error", err))
		}
	}
}

func (s *Service) processSession(ctx context.Context, state aman.AirportState) error {
	session, err := s.deps.Sessions.GetByID(ctx, state.SessionID)
	if err != nil {
		return err
	}
	if session == nil || session.Name != "LIVE" || !strings.EqualFold(strings.TrimSpace(session.Airport), strings.TrimSpace(state.Airport)) {
		return nil
	}
	station := strings.ToUpper(strings.TrimSpace(session.Airport))
	if station == "" || s.deps.AirportEnabled != nil && !s.deps.AirportEnabled(station) {
		return nil
	}
	strips, err := s.deps.Strips.List(ctx, session.ID)
	if err != nil {
		return err
	}
	byCallsign := make(map[string]*models.Strip, len(strips))
	for _, strip := range strips {
		if strip != nil {
			byCallsign[strings.ToUpper(strings.TrimSpace(strip.Callsign))] = strip
		}
	}

	now := s.deps.Now().UTC()
	seen := make(map[episodeKey]struct{}, len(state.Flights))
	var failures []error
	for _, flight := range state.Flights {
		strip := byCallsign[strings.ToUpper(strings.TrimSpace(flight.Callsign))]
		if strip == nil {
			continue
		}
		key := episodeKey{session: session.ID, episode: int64(strip.ID)}
		seen[key] = struct{}{}
		if err := s.processFlight(ctx, station, key, flight, strip, now); err != nil {
			failures = append(failures, err)
		}
	}
	s.mu.Lock()
	for key := range s.episodes {
		if _, ok := seen[key]; key.session == session.ID && !ok {
			delete(s.episodes, key)
		}
	}
	s.mu.Unlock()
	return errors.Join(failures...)
}

func (s *Service) processFlight(ctx context.Context, station string, key episodeKey, flight aman.AMANFlight, strip *models.Strip, now time.Time) error {
	episode, err := s.episode(ctx, key)
	if err != nil {
		return err
	}
	if !episode.infoSent {
		if !ArrivalInfoDue(flight, now) {
			return nil
		}
		delay, _ := Delay(flight)
		stand := ExpectedStand(strip)
		text := ArrivalInfoText(station, stand, ExpectedSTAR(strip, flight, s.deps.Terminal), delay)
		return s.send(ctx, station, key, episode, SentMessage{
			Session: key.session, Callsign: strip.Callsign, Episode: key.episode,
			Kind: KindArrivalInfo, Text: text, Stand: stand, SentAt: now,
		})
	}

	stand, due := episode.standUpdateDue(ExpectedStand(strip), strip.PositionAltitude, now)
	if !due {
		return nil
	}
	return s.send(ctx, station, key, episode, SentMessage{
		Session: key.session, Callsign: strip.Callsign, Episode: key.episode,
		Kind: KindStandUpdate, Text: StandUpdateText(station, stand), Stand: stand, SentAt: now,
	})
}

// ArrivalInfoDue reports whether arrival information should be sent now to a
// flight that has not had it. It does not consider persistence.
func ArrivalInfoDue(flight aman.AMANFlight, now time.Time) bool {
	if flight.State != aman.StateUnstable && flight.State != aman.StateStable {
		return false
	}
	if _, ok := Delay(flight); !ok {
		return false
	}
	// A flight that went around before it was ever told is not told now.
	if flight.GoAroundConfirmation != nil && flight.GoAroundConfirmation.Status == aman.GoAroundConfirmationConfirmed {
		return false
	}
	if flight.Prediction.OperationalTETA.Sub(now) > TriggerHorizon {
		return false
	}
	return flight.Slot.Time.Sub(now) > LateCutoff
}

// standUpdateDue applies the debounce. A stage change keeps the stand text
// unchanged, so it never starts one; a removed stand is never announced.
func (e *episodeState) standUpdateDue(current string, altitude *int32, now time.Time) (string, bool) {
	if current == "" || current == e.lastStand {
		e.pendingStand, e.pendingSince = "", time.Time{}
		return "", false
	}
	if current != e.pendingStand {
		e.pendingStand, e.pendingSince = current, now
	}
	if now.Sub(e.pendingSince) < StandDebounce {
		return "", false
	}
	if altitude == nil || *altitude <= StandUpdateMinAltitudeFeet {
		return "", false
	}
	return current, true
}

func (s *Service) episode(ctx context.Context, key episodeKey) (*episodeState, error) {
	s.mu.Lock()
	episode, ok := s.episodes[key]
	s.mu.Unlock()
	if ok {
		return episode, nil
	}
	messages, err := s.deps.Store.ListEpisode(ctx, key.session, key.episode)
	if err != nil {
		return nil, err
	}
	episode = episodeFromMessages(messages)
	s.mu.Lock()
	if _, removed := s.removed[key.session]; !removed {
		s.episodes[key] = episode
	}
	s.mu.Unlock()
	return episode, nil
}

func episodeFromMessages(messages []SentMessage) *episodeState {
	episode := &episodeState{}
	for _, message := range messages {
		if message.Kind == KindArrivalInfo {
			episode.infoSent = true
		}
		episode.lastStand = cleanToken(message.Stand)
	}
	return episode
}

func (s *Service) send(ctx context.Context, station string, key episodeKey, episode *episodeState, message SentMessage) error {
	id, inserted, err := s.deps.Store.Reserve(ctx, message)
	if err != nil {
		return err
	}
	if !inserted {
		// Another writer already sent this arrival's information.
		messages, listErr := s.deps.Store.ListEpisode(ctx, key.session, key.episode)
		if listErr != nil {
			return listErr
		}
		*episode = *episodeFromMessages(messages)
		return nil
	}
	if err := s.deps.Sender.SendTelex(ctx, station, strings.ToUpper(strings.TrimSpace(message.Callsign)), message.Text); err != nil {
		if releaseErr := s.deps.Store.Release(context.WithoutCancel(ctx), id); releaseErr != nil {
			err = errors.Join(err, releaseErr)
		}
		return err
	}
	slog.InfoContext(ctx, "Arrival info: telex sent",
		slog.Int("session", int(key.session)), slog.String("callsign", message.Callsign),
		slog.String("kind", string(message.Kind)), slog.String("message", message.Text))
	if message.Kind == KindArrivalInfo {
		episode.infoSent = true
	}
	episode.lastStand = message.Stand
	episode.pendingStand, episode.pendingSince = "", time.Time{}
	return nil
}
