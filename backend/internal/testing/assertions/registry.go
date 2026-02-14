package assertions

import (
	"context"
	"fmt"
	"reflect"

	"FlightStrips/internal/database"
)

// CheckType defines the type of assertion check
type CheckType string

const (
	// Strip assertions
	CheckStripExists       CheckType = "strip_exists"
	CheckStripNotExists    CheckType = "strip_not_exists"
	CheckStripFieldEquals  CheckType = "strip_field_equals"
	CheckStripFieldNotEquals CheckType = "strip_field_not_equals"
	CheckStripFieldContains CheckType = "strip_field_contains"
	
	// Controller assertions
	CheckControllerOnline  CheckType = "controller_online"
	CheckControllerOffline CheckType = "controller_offline"
	
	// Count assertions
	CheckStripCount        CheckType = "strip_count"
	CheckControllerCount   CheckType = "controller_count"
	
	// Session assertions
	CheckSessionExists     CheckType = "session_exists"
	CheckSessionAirport    CheckType = "session_airport"
)

// Checker defines the interface for assertion checkers
type Checker interface {
	// Check executes the assertion check
	Check(ctx context.Context, params map[string]interface{}) (bool, string, error)
	
	// Type returns the check type
	Type() CheckType
	
	// Description returns a human-readable description
	Description(params map[string]interface{}) string
}

// CheckResult represents the result of an assertion check
type CheckResult struct {
	Type        CheckType
	Description string
	Passed      bool
	Message     string
	Error       error
}

// AssertionResult represents the result of a group of checks at a specific point
type AssertionResult struct {
	AfterEventIndex int
	Description     string
	CheckResults    []CheckResult
	AllPassed       bool
}

// Registry holds all available checkers
type Registry struct {
	checkers map[CheckType]Checker
	queries  *database.Queries
}

// NewRegistry creates a new checker registry
func NewRegistry(queries *database.Queries) *Registry {
	registry := &Registry{
		checkers: make(map[CheckType]Checker),
		queries:  queries,
	}
	
	// Register all checkers
	registry.Register(NewStripExistsChecker(queries))
	registry.Register(NewStripNotExistsChecker(queries))
	registry.Register(NewStripFieldEqualsChecker(queries))
	registry.Register(NewStripFieldNotEqualsChecker(queries))
	registry.Register(NewStripFieldContainsChecker(queries))
	// TODO: These require database methods that don't exist yet:
	// registry.Register(NewControllerOnlineChecker(queries))
	// registry.Register(NewControllerOfflineChecker(queries))
	// registry.Register(NewStripCountChecker(queries))
	// registry.Register(NewControllerCountChecker(queries))
	// registry.Register(NewSessionExistsChecker(queries))
	// registry.Register(NewSessionAirportChecker(queries))
	
	return registry
}

// Register adds a checker to the registry
func (r *Registry) Register(checker Checker) {
	r.checkers[checker.Type()] = checker
}

// Get retrieves a checker by type
func (r *Registry) Get(checkType CheckType) (Checker, error) {
	checker, ok := r.checkers[checkType]
	if !ok {
		return nil, fmt.Errorf("unknown check type: %s", checkType)
	}
	return checker, nil
}

// ExecuteCheck runs a single assertion check
func (r *Registry) ExecuteCheck(ctx context.Context, checkType CheckType, params map[string]interface{}) CheckResult {
	checker, err := r.Get(checkType)
	if err != nil {
		return CheckResult{
			Type:        checkType,
			Description: fmt.Sprintf("Check type: %s", checkType),
			Passed:      false,
			Message:     "",
			Error:       err,
		}
	}
	
	passed, message, err := checker.Check(ctx, params)
	
	return CheckResult{
		Type:        checkType,
		Description: checker.Description(params),
		Passed:      passed,
		Message:     message,
		Error:       err,
	}
}

// Helper function to get required string parameter
func getRequiredString(params map[string]interface{}, key string) (string, error) {
	val, ok := params[key]
	if !ok {
		return "", fmt.Errorf("missing required parameter: %s", key)
	}
	str, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("parameter %s must be a string", key)
	}
	return str, nil
}

// Helper function to get required int parameter
func getRequiredInt(params map[string]interface{}, key string) (int, error) {
	val, ok := params[key]
	if !ok {
		return 0, fmt.Errorf("missing required parameter: %s", key)
	}
	
	// Handle both int and float64 (JSON unmarshals numbers as float64)
	switch v := val.(type) {
	case int:
		return v, nil
	case float64:
		return int(v), nil
	case int32:
		return int(v), nil
	default:
		return 0, fmt.Errorf("parameter %s must be a number", key)
	}
}

// Helper function to get required int32 parameter
func getRequiredInt32(params map[string]interface{}, key string) (int32, error) {
	val, err := getRequiredInt(params, key)
	if err != nil {
		return 0, err
	}
	return int32(val), nil
}

// Helper function to compare values
func compareValues(actual, expected interface{}) bool {
	// Use reflect for deep comparison
	return reflect.DeepEqual(actual, expected)
}
