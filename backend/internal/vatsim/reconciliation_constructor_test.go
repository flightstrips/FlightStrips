package vatsim

import (
	"FlightStrips/internal/models"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func validReconcilerDependencies() ReconcilerDependencies {
	return ReconcilerDependencies{
		Cache:              newReconciliationTestCache(time.Now()),
		Sessions:           reconciliationTestSessions{items: []*models.Session{}},
		Strips:             &reconciliationTestStrips{},
		Assignments:        reconciliationTestAssignments{},
		DepartureLifecycle: testDepartureLifecycle{},
		ArrivalLifecycle:   testArrivalLifecycle{},
		Notifier:           testReconciliationNotifier{},
	}
}

func TestNewReconcilerRejectsMissingRequiredDependencies(t *testing.T) {
	tests := []struct {
		name   string
		remove func(*ReconcilerDependencies)
		want   string
	}{
		{"cache", func(d *ReconcilerDependencies) { d.Cache = nil }, "VATSIM reconciler requires VATSIM cache"},
		{"sessions", func(d *ReconcilerDependencies) { d.Sessions = nil }, "VATSIM reconciler requires session store"},
		{"strips", func(d *ReconcilerDependencies) { d.Strips = nil }, "VATSIM reconciler requires strip store"},
		{"assignments", func(d *ReconcilerDependencies) { d.Assignments = nil }, "VATSIM reconciler requires stand assignment store"},
		{"departure lifecycle", func(d *ReconcilerDependencies) { d.DepartureLifecycle = nil }, "VATSIM reconciler requires departure lifecycle"},
		{"arrival lifecycle", func(d *ReconcilerDependencies) { d.ArrivalLifecycle = nil }, "VATSIM reconciler requires arrival lifecycle"},
		{"notifier", func(d *ReconcilerDependencies) { d.Notifier = nil }, "VATSIM reconciler requires strip notifier"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deps := validReconcilerDependencies()
			test.remove(&deps)
			_, err := NewReconciler(deps, time.Second)
			require.EqualError(t, err, test.want)
		})
	}
}
