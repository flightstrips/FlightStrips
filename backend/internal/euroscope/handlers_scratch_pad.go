package euroscope

import (
	events "FlightStrips/pkg/events/euroscope"
	"context"
	"errors"
	"strings"
	"unicode/utf8"
)

// Every operational controller may edit this field, including a non-tracking slave.
// ES strip sync intentionally never writes it back to the database.
func handleFsScratchPad(ctx context.Context, client *Client, message Message) error {
	if client.identitySnapshot().observer {
		return errors.New("observers cannot edit the FS scratch pad")
	}
	var event events.FsScratchPadEvent
	if err := message.ProtoUnmarshal(&event); err != nil {
		return err
	}
	if strings.TrimSpace(event.Callsign) == "" || !utf8.ValidString(event.Text) || len(event.Text) > 15 || strings.ContainsAny(event.Text, "\x00\r\n") {
		return errors.New("FS scratch pad must contain at most 15 bytes of single-line text")
	}
	client.hub.scratchPadMu.Lock()
	defer client.hub.scratchPadMu.Unlock()
	count, err := client.hub.server.GetStripRepository().UpdateFsScratchPad(ctx, client.session, event.Callsign, event.Text)
	if err != nil {
		return err
	}
	if count == 1 {
		client.hub.Broadcast(client.session, event)
	}
	return nil
}
