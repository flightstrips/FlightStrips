package vatsim

import (
	"FlightStrips/internal/models"
	"context"
	"time"
)

type testDepartureLifecycle struct{}

func (testDepartureLifecycle) ProcessDeparture(context.Context, int32, *models.Strip, DepartureFlightInfo) error {
	return nil
}

func (testDepartureLifecycle) CancelDeparture(context.Context, int32, string) error {
	return nil
}

type testArrivalLifecycle struct{}

func (testArrivalLifecycle) ProcessArrival(context.Context, int32, *models.Strip, ArrivalFlightInfo) error {
	return nil
}

type testReconciliationNotifier struct{}

func (testReconciliationNotifier) SendStripUpdate(int32, string) {}

func newTestReconciler(
	cache *Cache,
	sessions reconciliationSessionStore,
	strips reconciliationStripStore,
	assignments reconciliationAssignmentStore,
	notifier reconciliationNotifier,
	interval time.Duration,
	options ...ArrivalETAOption,
) *Reconciler {
	if notifier == nil {
		notifier = testReconciliationNotifier{}
	}
	return newReconciler(
		cache,
		sessions,
		strips,
		assignments,
		testDepartureLifecycle{},
		testArrivalLifecycle{},
		notifier,
		interval,
		options...,
	)
}
