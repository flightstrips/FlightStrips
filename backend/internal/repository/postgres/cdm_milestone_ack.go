package postgres

import (
	"context"
	"fmt"
)

// AcknowledgeCdmMilestone updates only the pending flag for the exact exported
// milestone. The acknowledgement preserves concurrent changes to other CDM fields.
func (r *stripRepository) AcknowledgeCdmMilestone(ctx context.Context, session int32, callsign string, milestone string, value string) (int64, error) {
	return r.AcknowledgeCdmDeferredWork(ctx, session, callsign, milestone, value, "")
}

// AcknowledgeCdmDeferredWork fences acknowledgements by pending value and the
// exact human-origin token. It cannot clear a newer controller's pending work.
func (r *stripRepository) AcknowledgeCdmDeferredWork(ctx context.Context, session int32, callsign, milestone, value, authority string) (int64, error) {
	var valueKey, pendingKey, authorityKey string
	switch milestone {
	case "AOBT":
		valueKey, pendingKey, authorityKey = "aobt", "aobtViffPending", "aobtViffAuthority"
	case "ATOT":
		valueKey, pendingKey, authorityKey = "atot", "atotViffPending", "atotViffAuthority"
	case "READY":
		valueKey, pendingKey, authorityKey = "asrt", "readySyncPending", "readyViffAuthority"
	default:
		return 0, fmt.Errorf("unsupported CDM milestone %q", milestone)
	}
	result, err := r.db.Exec(ctx, `
UPDATE strips
SET cdm_data = jsonb_set(cdm_data, ARRAY[$4::text], 'false'::jsonb) - $6::text
WHERE session = $1 AND callsign = $2
  AND cdm_data ->> $3::text = $5
  AND cdm_data -> $4::text = 'true'::jsonb
  AND COALESCE(cdm_data ->> $6::text, '') = $7`, session, callsign, valueKey, pendingKey, value, authorityKey, authority)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

func (r *stripRepository) AcknowledgeCdmViffAuthority(ctx context.Context, session int32, callsign, token string) (int64, error) {
	result, err := r.db.Exec(ctx, `UPDATE strips SET cdm_data = cdm_data - 'viffAuthority' WHERE session = $1 AND callsign = $2 AND cdm_data ->> 'viffAuthority' = $3`, session, callsign, token)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}
