package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDistanceAlongTrackIsNotValidatedAsAngle(t *testing.T) {
	require.NoError(t, validateTyped((&pb.AmanRouteProgress{AlongTrackNm: 720}).ProtoReflect()))
	course := 360.0
	err := validateTyped((&pb.NavLeg{CourseTrueDegrees: &course}).ProtoReflect())
	require.Error(t, err)
	require.Contains(t, err.Error(), "course_true_degrees")
}
