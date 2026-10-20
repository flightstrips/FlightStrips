package shared

import "context"

// PositionMotion travels with one position report through dispatch and batching.
// ReceiptTime remains the socket receipt time, independent of processing delay.
type PositionMotion struct {
	GroundspeedKnots *float64
	PositionAgeMS    *int64
}

type positionMotionKey struct{}

func WithPositionMotion(ctx context.Context, motion PositionMotion) context.Context {
	return context.WithValue(ctx, positionMotionKey{}, motion)
}

func PositionMotionFromContext(ctx context.Context) PositionMotion {
	motion, _ := ctx.Value(positionMotionKey{}).(PositionMotion)
	return motion
}
