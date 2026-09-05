package euroscope

import (
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events"
	eventseuroscope "FlightStrips/pkg/events/euroscope"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	gorilla "github.com/gorilla/websocket"
)

type Client struct {
	conn        *gorilla.Conn
	session     int32
	sessionName string
	send        chan events.OutgoingMessage
	closeOnce   sync.Once
	closed      chan struct{}
	hub         *Hub
	user        shared.AuthenticatedUser

	position string
	callsign string
	airport  string
	version  string
	observer bool
	localIP  string

	flightPlanCacheMu     sync.Mutex
	positionProcessLocks  map[string]*sync.Mutex
	stripUpdateCache      map[string]eventseuroscope.Strip
	assignedSquawkCache   map[string]string
	positionUpdateCache   map[string]cachedAircraftPosition
	pendingPositions      map[string]*pendingPositionUpdate
	positionCoalesceDelay time.Duration
}

const defaultPositionCoalesceDelay = time.Second

type cachedAircraftPosition struct {
	lat      float64
	lon      float64
	altitude int32
}

type pendingPositionUpdate struct {
	callsign string
	position cachedAircraftPosition
	timer    *time.Timer
}

func (c *Client) GetSendChannel() chan events.OutgoingMessage {
	return c.send
}

func (c *Client) Enqueue(message events.OutgoingMessage) bool {
	select {
	case <-c.closed:
		return false
	case c.send <- message:
		return true
	default:
		c.disconnectSlowConsumer()
		return false
	}
}

func (c *Client) disconnectSlowConsumer() {
	shouldUnregister := false
	c.closeOnce.Do(func() {
		close(c.closed)
		shouldUnregister = true
	})
	if !shouldUnregister {
		return
	}

	slog.Warn("Disconnecting slow websocket client",
		slog.String("source", c.GetSource()),
		slog.String("cid", c.GetCid()),
		slog.Int("session", int(c.session)),
	)

	if c.hub != nil {
		go c.hub.Unregister(c)
		return
	}

	_ = c.Close()
}

func (c *Client) Close() error {
	if c.closed != nil {
		c.closeOnce.Do(func() {
			close(c.closed)
		})
	}
	c.stopPendingPositionUpdates()
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *Client) GetCid() string {
	return c.user.GetCid()
}

func (c *Client) GetCallsign() string {
	return c.callsign
}

func (c *Client) GetAirport() string {
	return c.airport
}

func (c *Client) GetPosition() string {
	return c.position
}

func (c *Client) GetSession() int32 {
	return c.session
}

func (c *Client) GetSessionName() string {
	return c.sessionName
}

func (c *Client) GetSource() string {
	return "euroscope"
}

func (c *Client) GetVersion() string {
	return c.version
}

func (c *Client) GetConnection() *gorilla.Conn {
	return c.conn
}

func (c *Client) IsAuthenticated() bool {
	return c.user.IsValid()
}

func (c *Client) SetUser(user shared.AuthenticatedUser) {
	c.user = user
}

func (c *Client) CanHandleMessage(messageType string) error {
	if !c.observer || messageType == "token" || messageType == "login" || messageType == "runway" {
		return nil
	}

	return errors.New("observer Euroscope clients cannot publish operational data")
}

// HandlePong handles pong messages from the client
func (c *Client) HandlePong() error {
	// Update the last seen timestamp in the database
	controllerRepo := c.hub.server.GetControllerRepository()
	now := time.Now().UTC()
	count, err := controllerRepo.SetEuroscopeSeen(context.Background(), c.GetCid(), c.session, &now)

	if count != 1 {
		return errors.New("failed to update last seen timestamp")
	}
	return err
}

// RecordMessage records an incoming message if recording is enabled
func (c *Client) RecordMessage(rawMessage []byte) {
	c.hub.recordMessage(c.session, rawMessage)
}

func cachedOperationalStrip(strip eventseuroscope.Strip) eventseuroscope.Strip {
	strip.Position.Lat = 0
	strip.Position.Lon = 0
	strip.Position.Altitude = 0
	return strip
}

func flightPlanCacheKey(callsign string) string {
	return strings.ToUpper(strings.TrimSpace(callsign))
}

func (c *Client) positionProcessLock(callsign string) *sync.Mutex {
	key := flightPlanCacheKey(callsign)
	c.flightPlanCacheMu.Lock()
	defer c.flightPlanCacheMu.Unlock()

	if c.positionProcessLocks == nil {
		c.positionProcessLocks = make(map[string]*sync.Mutex)
	}
	if lock := c.positionProcessLocks[key]; lock != nil {
		return lock
	}
	lock := &sync.Mutex{}
	c.positionProcessLocks[key] = lock
	return lock
}

func (c *Client) hasCachedOperationalStrip(strip eventseuroscope.Strip) bool {
	c.flightPlanCacheMu.Lock()
	defer c.flightPlanCacheMu.Unlock()

	cached, ok := c.stripUpdateCache[flightPlanCacheKey(strip.Callsign)]
	return ok && cached == cachedOperationalStrip(strip)
}

func (c *Client) rememberOperationalStrip(strip eventseuroscope.Strip) {
	c.flightPlanCacheMu.Lock()
	defer c.flightPlanCacheMu.Unlock()

	if c.stripUpdateCache == nil {
		c.stripUpdateCache = make(map[string]eventseuroscope.Strip)
	}
	if c.assignedSquawkCache == nil {
		c.assignedSquawkCache = make(map[string]string)
	}
	if c.positionUpdateCache == nil {
		c.positionUpdateCache = make(map[string]cachedAircraftPosition)
	}
	key := flightPlanCacheKey(strip.Callsign)
	if pending := c.pendingPositions[key]; pending != nil {
		pending.timer.Stop()
		delete(c.pendingPositions, key)
	}
	c.stripUpdateCache[key] = cachedOperationalStrip(strip)
	c.assignedSquawkCache[key] = strip.AssignedSquawk
	c.positionUpdateCache[key] = cachedAircraftPosition{
		lat: strip.Position.Lat, lon: strip.Position.Lon, altitude: strip.Position.Altitude,
	}
}

func (c *Client) hasCachedAssignedSquawk(callsign, squawk string) bool {
	c.flightPlanCacheMu.Lock()
	defer c.flightPlanCacheMu.Unlock()

	cached, ok := c.assignedSquawkCache[flightPlanCacheKey(callsign)]
	return ok && cached == squawk
}

func (c *Client) rememberAssignedSquawk(callsign, squawk string) {
	c.flightPlanCacheMu.Lock()
	defer c.flightPlanCacheMu.Unlock()

	if c.assignedSquawkCache == nil {
		c.assignedSquawkCache = make(map[string]string)
	}
	c.assignedSquawkCache[flightPlanCacheKey(callsign)] = squawk
}

func (c *Client) forgetFlightPlanCache(callsign string) {
	processLock := c.positionProcessLock(callsign)
	processLock.Lock()
	defer processLock.Unlock()

	c.flightPlanCacheMu.Lock()
	defer c.flightPlanCacheMu.Unlock()

	key := flightPlanCacheKey(callsign)
	if pending := c.pendingPositions[key]; pending != nil {
		pending.timer.Stop()
		delete(c.pendingPositions, key)
	}
	delete(c.stripUpdateCache, key)
	delete(c.assignedSquawkCache, key)
	delete(c.positionUpdateCache, key)
}

func (c *Client) rememberAircraftPosition(callsign string, position cachedAircraftPosition) {
	c.flightPlanCacheMu.Lock()
	defer c.flightPlanCacheMu.Unlock()

	if c.positionUpdateCache == nil {
		c.positionUpdateCache = make(map[string]cachedAircraftPosition)
	}
	c.positionUpdateCache[flightPlanCacheKey(callsign)] = position
}

func (c *Client) processAircraftPosition(ctx context.Context, callsign string, position cachedAircraftPosition) error {
	processLock := c.positionProcessLock(callsign)
	processLock.Lock()
	defer processLock.Unlock()

	key := flightPlanCacheKey(callsign)
	c.flightPlanCacheMu.Lock()
	if pending := c.pendingPositions[key]; pending != nil {
		pending.timer.Stop()
		delete(c.pendingPositions, key)
	}
	c.flightPlanCacheMu.Unlock()

	if err := c.hub.stripService.UpdateAircraftPosition(
		ctx, c.session, callsign, position.lat, position.lon, position.altitude, c.airport,
	); err != nil {
		return err
	}
	c.rememberAircraftPosition(callsign, position)
	return nil
}

func (c *Client) queuePositionOnlyUpdate(strip eventseuroscope.Strip) {
	key := flightPlanCacheKey(strip.Callsign)
	position := cachedAircraftPosition{
		lat: strip.Position.Lat, lon: strip.Position.Lon, altitude: strip.Position.Altitude,
	}

	c.flightPlanCacheMu.Lock()
	if cached, ok := c.positionUpdateCache[key]; ok && cached == position {
		c.flightPlanCacheMu.Unlock()
		return
	}
	if pending := c.pendingPositions[key]; pending != nil {
		pending.position = position
		c.flightPlanCacheMu.Unlock()
		return
	}
	if c.pendingPositions == nil {
		c.pendingPositions = make(map[string]*pendingPositionUpdate)
	}
	delay := c.positionCoalesceDelay
	if delay <= 0 {
		delay = defaultPositionCoalesceDelay
	}
	pending := &pendingPositionUpdate{callsign: strip.Callsign, position: position}
	pending.timer = time.AfterFunc(delay, func() { c.flushPositionOnlyUpdate(key) })
	c.pendingPositions[key] = pending
	c.flightPlanCacheMu.Unlock()
}

func (c *Client) flushPositionOnlyUpdate(key string) {
	processLock := c.positionProcessLock(key)
	processLock.Lock()
	defer processLock.Unlock()

	c.flightPlanCacheMu.Lock()
	pending := c.pendingPositions[key]
	if pending == nil {
		c.flightPlanCacheMu.Unlock()
		return
	}
	delete(c.pendingPositions, key)
	if cached, ok := c.positionUpdateCache[key]; ok && cached == pending.position {
		c.flightPlanCacheMu.Unlock()
		return
	}
	c.flightPlanCacheMu.Unlock()

	if c.hub == nil || c.hub.stripService == nil || c.isClosed() {
		return
	}
	ctx := context.Background()
	if err := c.hub.stripService.UpdateAircraftPosition(
		ctx, c.session, pending.callsign,
		pending.position.lat, pending.position.lon, pending.position.altitude, c.airport,
	); err != nil {
		slog.WarnContext(ctx, "Failed to process coalesced full-strip position update",
			slog.String("callsign", pending.callsign), slog.Any("error", err))
		return
	}
	c.rememberAircraftPosition(pending.callsign, pending.position)
	c.hub.markEuroscopeSeen(ctx, c.session, pending.callsign)
}

func (c *Client) stopPendingPositionUpdates() {
	c.flightPlanCacheMu.Lock()
	defer c.flightPlanCacheMu.Unlock()
	for key, pending := range c.pendingPositions {
		pending.timer.Stop()
		delete(c.pendingPositions, key)
	}
}

func (c *Client) isClosed() bool {
	if c.closed == nil {
		return false
	}
	select {
	case <-c.closed:
		return true
	default:
		return false
	}
}
