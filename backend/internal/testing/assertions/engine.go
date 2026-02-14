package assertions

import (
	"context"
	"fmt"
	"log/slog"

	"FlightStrips/internal/database"
	"FlightStrips/internal/testing/recorder"
)

// Engine executes assertions during replay
type Engine struct {
	registry  *Registry
	sessionID int32
	results   []AssertionResult
}

// NewEngine creates a new assertion engine
func NewEngine(queries *database.Queries, sessionID int32) *Engine {
	return &Engine{
		registry:  NewRegistry(queries),
		sessionID: sessionID,
		results:   []AssertionResult{},
	}
}

// ExecuteAssertions runs all checks for a given assertion
func (e *Engine) ExecuteAssertions(ctx context.Context, assertion recorder.Assertion) AssertionResult {
	slog.Debug("Executing assertions",
		slog.Int("after_event", assertion.AfterEventIndex),
		slog.String("description", assertion.Description),
		slog.Int("checks", len(assertion.Checks)))

	result := AssertionResult{
		AfterEventIndex: assertion.AfterEventIndex,
		Description:     assertion.Description,
		CheckResults:    []CheckResult{},
		AllPassed:       true,
	}

	for _, check := range assertion.Checks {
		// Add session_id to params if not present
		if check.Params == nil {
			check.Params = make(map[string]interface{})
		}
		check.Params["session_id"] = e.sessionID

		// Merge callsign, field, expected etc. into params
		if check.Callsign != "" {
			check.Params["callsign"] = check.Callsign
		}
		if check.Field != "" {
			check.Params["field"] = check.Field
		}
		if check.Expected != nil {
			check.Params["expected"] = check.Expected
		}

		// Execute the check
		checkResult := e.registry.ExecuteCheck(ctx, CheckType(check.Type), check.Params)
		result.CheckResults = append(result.CheckResults, checkResult)

		if !checkResult.Passed || checkResult.Error != nil {
			result.AllPassed = false
		}

		// Log check result
		if checkResult.Passed {
			slog.Debug("Check passed",
				slog.String("type", string(checkResult.Type)),
				slog.String("description", checkResult.Description),
				slog.String("message", checkResult.Message))
		} else {
			slog.Error("Check failed",
				slog.String("type", string(checkResult.Type)),
				slog.String("description", checkResult.Description),
				slog.String("message", checkResult.Message),
				slog.Any("error", checkResult.Error))
		}
	}

	e.results = append(e.results, result)
	return result
}

// GetResults returns all assertion results
func (e *Engine) GetResults() []AssertionResult {
	return e.results
}

// Summary returns a summary of all assertion results
func (e *Engine) Summary() AssertionSummary {
	summary := AssertionSummary{
		TotalAssertions: len(e.results),
		TotalChecks:     0,
		PassedChecks:    0,
		FailedChecks:    0,
		ErroredChecks:   0,
	}

	for _, result := range e.results {
		if result.AllPassed {
			summary.PassedAssertions++
		} else {
			summary.FailedAssertions++
		}

		for _, check := range result.CheckResults {
			summary.TotalChecks++
			if check.Error != nil {
				summary.ErroredChecks++
			} else if check.Passed {
				summary.PassedChecks++
			} else {
				summary.FailedChecks++
			}
		}
	}

	return summary
}

// PrintResults prints all assertion results
func (e *Engine) PrintResults() {
	summary := e.Summary()

	fmt.Println("\n=== Assertion Results ===")
	fmt.Printf("Total Assertions: %d\n", summary.TotalAssertions)
	fmt.Printf("Passed: %d, Failed: %d\n\n", summary.PassedAssertions, summary.FailedAssertions)

	for _, result := range e.results {
		status := "✓ PASS"
		if !result.AllPassed {
			status = "✗ FAIL"
		}

		fmt.Printf("%s [Event %d] %s\n", status, result.AfterEventIndex, result.Description)

		for _, check := range result.CheckResults {
			checkStatus := "  ✓"
			if check.Error != nil {
				checkStatus = "  ✗ ERROR"
			} else if !check.Passed {
				checkStatus = "  ✗"
			}

			fmt.Printf("%s %s: %s\n", checkStatus, check.Description, check.Message)
			if check.Error != nil {
				fmt.Printf("     Error: %v\n", check.Error)
			}
		}
		fmt.Println()
	}

	fmt.Println("=== Summary ===")
	fmt.Printf("Checks: %d total, %d passed, %d failed, %d errors\n",
		summary.TotalChecks, summary.PassedChecks, summary.FailedChecks, summary.ErroredChecks)

	if summary.FailedChecks > 0 || summary.ErroredChecks > 0 {
		fmt.Printf("\n❌ ASSERTIONS FAILED\n")
	} else {
		fmt.Printf("\n✅ ALL ASSERTIONS PASSED\n")
	}
}

// HasFailures returns true if any assertion failed
func (e *Engine) HasFailures() bool {
	summary := e.Summary()
	return summary.FailedChecks > 0 || summary.ErroredChecks > 0
}

// AssertionSummary contains summary statistics
type AssertionSummary struct {
	TotalAssertions  int
	PassedAssertions int
	FailedAssertions int
	TotalChecks      int
	PassedChecks     int
	FailedChecks     int
	ErroredChecks    int
}
