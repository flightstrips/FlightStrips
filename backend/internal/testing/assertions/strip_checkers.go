package assertions

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"FlightStrips/internal/database"

	"github.com/jackc/pgx/v5"
)

// StripExistsChecker checks if a strip exists in the database
type StripExistsChecker struct {
	queries *database.Queries
}

func NewStripExistsChecker(queries *database.Queries) *StripExistsChecker {
	return &StripExistsChecker{queries: queries}
}

func (c *StripExistsChecker) Type() CheckType {
	return CheckStripExists
}

func (c *StripExistsChecker) Description(params map[string]interface{}) string {
	callsign, _ := getRequiredString(params, "callsign")
	return fmt.Sprintf("Strip '%s' should exist", callsign)
}

func (c *StripExistsChecker) Check(ctx context.Context, params map[string]interface{}) (bool, string, error) {
	callsign, err := getRequiredString(params, "callsign")
	if err != nil {
		return false, "", err
	}
	
	sessionID, err := getRequiredInt32(params, "session_id")
	if err != nil {
		return false, "", err
	}
	
	_, err = c.queries.GetStrip(ctx, database.GetStripParams{
		Session:  sessionID,
		Callsign: callsign,
	})
	
	if errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Sprintf("Strip '%s' does not exist", callsign), nil
	}
	if err != nil {
		return false, "", fmt.Errorf("database error: %w", err)
	}
	
	return true, fmt.Sprintf("Strip '%s' exists", callsign), nil
}

// StripNotExistsChecker checks if a strip does not exist
type StripNotExistsChecker struct {
	queries *database.Queries
}

func NewStripNotExistsChecker(queries *database.Queries) *StripNotExistsChecker {
	return &StripNotExistsChecker{queries: queries}
}

func (c *StripNotExistsChecker) Type() CheckType {
	return CheckStripNotExists
}

func (c *StripNotExistsChecker) Description(params map[string]interface{}) string {
	callsign, _ := getRequiredString(params, "callsign")
	return fmt.Sprintf("Strip '%s' should not exist", callsign)
}

func (c *StripNotExistsChecker) Check(ctx context.Context, params map[string]interface{}) (bool, string, error) {
	callsign, err := getRequiredString(params, "callsign")
	if err != nil {
		return false, "", err
	}
	
	sessionID, err := getRequiredInt32(params, "session_id")
	if err != nil {
		return false, "", err
	}
	
	_, err = c.queries.GetStrip(ctx, database.GetStripParams{
		Session:  sessionID,
		Callsign: callsign,
	})
	
	if errors.Is(err, pgx.ErrNoRows) {
		return true, fmt.Sprintf("Strip '%s' does not exist", callsign), nil
	}
	if err != nil {
		return false, "", fmt.Errorf("database error: %w", err)
	}
	
	return false, fmt.Sprintf("Strip '%s' exists but should not", callsign), nil
}

// StripFieldEqualsChecker checks if a strip field equals expected value
type StripFieldEqualsChecker struct {
	queries *database.Queries
}

func NewStripFieldEqualsChecker(queries *database.Queries) *StripFieldEqualsChecker {
	return &StripFieldEqualsChecker{queries: queries}
}

func (c *StripFieldEqualsChecker) Type() CheckType {
	return CheckStripFieldEquals
}

func (c *StripFieldEqualsChecker) Description(params map[string]interface{}) string {
	callsign, _ := getRequiredString(params, "callsign")
	field, _ := getRequiredString(params, "field")
	expected := params["expected"]
	return fmt.Sprintf("Strip '%s' field '%s' should equal '%v'", callsign, field, expected)
}

func (c *StripFieldEqualsChecker) Check(ctx context.Context, params map[string]interface{}) (bool, string, error) {
	callsign, err := getRequiredString(params, "callsign")
	if err != nil {
		return false, "", err
	}
	
	field, err := getRequiredString(params, "field")
	if err != nil {
		return false, "", err
	}
	
	expected, ok := params["expected"]
	if !ok {
		return false, "", fmt.Errorf("missing required parameter: expected")
	}
	
	sessionID, err := getRequiredInt32(params, "session_id")
	if err != nil {
		return false, "", err
	}
	
	strip, err := c.queries.GetStrip(ctx, database.GetStripParams{
		Session:  sessionID,
		Callsign: callsign,
	})
	
	if errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Sprintf("Strip '%s' does not exist", callsign), nil
	}
	if err != nil {
		return false, "", fmt.Errorf("database error: %w", err)
	}
	
	// Get field value using reflection
	actual, err := getFieldValue(strip, field)
	if err != nil {
		return false, "", err
	}
	
	// Compare values
	if compareValues(actual, expected) {
		return true, fmt.Sprintf("Field '%s' equals '%v'", field, expected), nil
	}
	
	return false, fmt.Sprintf("Field '%s' is '%v' but expected '%v'", field, actual, expected), nil
}

// StripFieldNotEqualsChecker checks if a strip field does not equal a value
type StripFieldNotEqualsChecker struct {
	queries *database.Queries
}

func NewStripFieldNotEqualsChecker(queries *database.Queries) *StripFieldNotEqualsChecker {
	return &StripFieldNotEqualsChecker{queries: queries}
}

func (c *StripFieldNotEqualsChecker) Type() CheckType {
	return CheckStripFieldNotEquals
}

func (c *StripFieldNotEqualsChecker) Description(params map[string]interface{}) string {
	callsign, _ := getRequiredString(params, "callsign")
	field, _ := getRequiredString(params, "field")
	value := params["value"]
	return fmt.Sprintf("Strip '%s' field '%s' should not equal '%v'", callsign, field, value)
}

func (c *StripFieldNotEqualsChecker) Check(ctx context.Context, params map[string]interface{}) (bool, string, error) {
	callsign, err := getRequiredString(params, "callsign")
	if err != nil {
		return false, "", err
	}
	
	field, err := getRequiredString(params, "field")
	if err != nil {
		return false, "", err
	}
	
	value, ok := params["value"]
	if !ok {
		return false, "", fmt.Errorf("missing required parameter: value")
	}
	
	sessionID, err := getRequiredInt32(params, "session_id")
	if err != nil {
		return false, "", err
	}
	
	strip, err := c.queries.GetStrip(ctx, database.GetStripParams{
		Session:  sessionID,
		Callsign: callsign,
	})
	
	if errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Sprintf("Strip '%s' does not exist", callsign), nil
	}
	if err != nil {
		return false, "", fmt.Errorf("database error: %w", err)
	}
	
	actual, err := getFieldValue(strip, field)
	if err != nil {
		return false, "", err
	}
	
	if !compareValues(actual, value) {
		return true, fmt.Sprintf("Field '%s' is '%v' (not '%v')", field, actual, value), nil
	}
	
	return false, fmt.Sprintf("Field '%s' is '%v' but should not equal '%v'", field, actual, value), nil
}

// StripFieldContainsChecker checks if a string field contains a substring
type StripFieldContainsChecker struct {
	queries *database.Queries
}

func NewStripFieldContainsChecker(queries *database.Queries) *StripFieldContainsChecker {
	return &StripFieldContainsChecker{queries: queries}
}

func (c *StripFieldContainsChecker) Type() CheckType {
	return CheckStripFieldContains
}

func (c *StripFieldContainsChecker) Description(params map[string]interface{}) string {
	callsign, _ := getRequiredString(params, "callsign")
	field, _ := getRequiredString(params, "field")
	substring, _ := getRequiredString(params, "substring")
	return fmt.Sprintf("Strip '%s' field '%s' should contain '%s'", callsign, field, substring)
}

func (c *StripFieldContainsChecker) Check(ctx context.Context, params map[string]interface{}) (bool, string, error) {
	callsign, err := getRequiredString(params, "callsign")
	if err != nil {
		return false, "", err
	}
	
	field, err := getRequiredString(params, "field")
	if err != nil {
		return false, "", err
	}
	
	substring, err := getRequiredString(params, "substring")
	if err != nil {
		return false, "", err
	}
	
	sessionID, err := getRequiredInt32(params, "session_id")
	if err != nil {
		return false, "", err
	}
	
	strip, err := c.queries.GetStrip(ctx, database.GetStripParams{
		Session:  sessionID,
		Callsign: callsign,
	})
	
	if errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Sprintf("Strip '%s' does not exist", callsign), nil
	}
	if err != nil {
		return false, "", fmt.Errorf("database error: %w", err)
	}
	
	actual, err := getFieldValue(strip, field)
	if err != nil {
		return false, "", err
	}
	
	// Convert to string
	actualStr := fmt.Sprintf("%v", actual)
	
	if strings.Contains(actualStr, substring) {
		return true, fmt.Sprintf("Field '%s' contains '%s'", field, substring), nil
	}
	
	return false, fmt.Sprintf("Field '%s' is '%s' which does not contain '%s'", field, actualStr, substring), nil
}

// getFieldValue extracts a field value from a struct using reflection
func getFieldValue(strip database.Strip, fieldName string) (interface{}, error) {
	v := reflect.ValueOf(strip)
	
	// Try to find the field by name (case-insensitive)
	for i := 0; i < v.NumField(); i++ {
		field := v.Type().Field(i)
		if strings.EqualFold(field.Name, fieldName) {
			return v.Field(i).Interface(), nil
		}
	}
	
	return nil, fmt.Errorf("field '%s' not found in strip", fieldName)
}
