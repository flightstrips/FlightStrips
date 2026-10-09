package postgres

import (
	"context"
	"fmt"
)

// AcknowledgeCdmMilestone updates only the pending flag for the exact exported
// milestone. The acknowledgement preserves concurrent changes to other CDM fields.
func (r *stripRepository) AcknowledgeCdmMilestone(ctx context.Context, session int32, callsign string, milestone string, value string) (int64, error) {
	var valueKey, pendingKey string
	switch milestone {
	case "AOBT":
		valueKey, pendingKey = "aobt", "aobtViffPending"
	case "ATOT":
		valueKey, pendingKey = "atot", "atotViffPending"
	default:
		return 0, fmt.Errorf("unsupported CDM milestone %q", milestone)
	}
	result, err := r.db.Exec(ctx, `
UPDATE strips
SET cdm_data = jsonb_set(cdm_data, ARRAY[$4::text], 'false'::jsonb)
WHERE session = $1 AND callsign = $2
  AND cdm_data ->> $3::text = $5
  AND cdm_data -> $4::text = 'true'::jsonb`, session, callsign, valueKey, pendingKey, value)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}
