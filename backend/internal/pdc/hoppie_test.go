package pdc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassify_ARINCPDCRequestRecognized(t *testing.T) {
	raw := "RCD\nSAS123-EKCH-GATE A10-ESSA\nATIS A\n-TYP/A320\n-RMK/NO SID"

	assert.Equal(t, MsgPDCRequest, classify(raw))
}

func TestParseClassicPDCRequest_WithRemarks(t *testing.T) {
	req, err := parsePDCRequest("REQUEST PREDEP CLEARANCE SAS123 A320 TO ESSA AT EKCH STAND A10 ATIS A\nNO SID")

	require.NoError(t, err)
	assert.Equal(t, "SAS123", req.Callsign)
	assert.Equal(t, "A320", req.Aircraft)
	assert.Equal(t, "EKCH", req.Departure)
	assert.Equal(t, "ESSA", req.Destination)
	assert.Equal(t, "A10", req.Stand)
	assert.Equal(t, "A", req.Atis)
	assert.Equal(t, "NO SID", req.Remarks)
}

func TestParseARINCPDCRequest_WithRemarks(t *testing.T) {
	req, err := parsePDCRequest("RCD\nSAS123-EKCH-GATE A10-ESSA\nATIS A\n-TYP/A320\n-RMK/NO SID")

	require.NoError(t, err)
	assert.Equal(t, "SAS123", req.Callsign)
	assert.Equal(t, "A320", req.Aircraft)
	assert.Equal(t, "EKCH", req.Departure)
	assert.Equal(t, "ESSA", req.Destination)
	assert.Equal(t, "A10", req.Stand)
	assert.Equal(t, "A", req.Atis)
	assert.Equal(t, "NO SID", req.Remarks)
}
