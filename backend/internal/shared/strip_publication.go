package shared

import (
	"FlightStrips/internal/models"
	"context"
)

type stripPublicationKey struct{}

func WithStripPublication(ctx context.Context, snapshot *models.StripPublicationSnapshot) context.Context {
	return context.WithValue(ctx, stripPublicationKey{}, snapshot)
}

func StripPublication(ctx context.Context, session int32) *models.StripPublicationSnapshot {
	snapshot, _ := ctx.Value(stripPublicationKey{}).(*models.StripPublicationSnapshot)
	if snapshot == nil || snapshot.Session == nil || snapshot.Session.ID != session {
		return nil
	}
	return snapshot
}
