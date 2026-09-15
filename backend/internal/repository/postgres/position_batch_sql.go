package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"FlightStrips/internal/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type positionKey struct {
	session  int32
	callsign string
}
type positionRead struct {
	repo     *stripRepository
	session  int32
	callsign string
}
type positionWrite struct {
	repo              *stripRepository
	session           int32
	callsign          string
	lat, lon          *float64
	alt               *int32
	bay               string
	sequence, version int32
}
type positionSnapshotDBRow struct {
	database.Strip
	Assignment []byte
}

func batchQueryContext(calls []*batchCall, operation string) (context.Context, trace.Span) {
	links := make([]trace.Link, 0, len(calls)-1)
	for _, call := range calls[1:] {
		if sc := trace.SpanContextFromContext(call.ctx); sc.IsValid() {
			links = append(links, trace.Link{SpanContext: sc})
		}
	}
	return otel.Tracer("position-batch").Start(calls[0].ctx, "position.batch."+operation,
		trace.WithLinks(links...), trace.WithAttributes(attribute.Int("position.batch.size", len(calls))))
}

// Each physical query is charged to its initiating report's DB counter; span
// links identify the other reports. Do not count one SQL statement N times.
func batchPositionReads(calls []*batchCall) {
	groups := make(map[*stripRepository][]*batchCall)
	for _, call := range calls {
		if err := call.ctx.Err(); err != nil {
			call.result <- batchResult{err: err}
			continue
		}
		r := call.input.(positionRead).repo
		groups[r] = append(groups[r], call)
	}
	for repo, group := range groups {
		if len(group) == 1 {
			v := group[0].input.(positionRead)
			snapshot, err := repo.GetPositionSnapshot(context.WithValue(group[0].ctx, positionBatchKey{}, (*positionBatchMember)(nil)), v.session, v.callsign)
			group[0].result <- batchResult{value: snapshot, err: err}
			continue
		}
		ctx, span := batchQueryContext(group, "snapshot")
		sessions, callsigns := make([]int32, len(group)), make([]string, len(group))
		for i, call := range group {
			v := call.input.(positionRead)
			sessions[i], callsigns[i] = v.session, v.callsign
		}
		rows, err := repo.pool.Query(ctx, `-- position batch snapshot
SELECT s.*, (SELECT to_jsonb(a) FROM stand_assignments a
 WHERE a.session_id = s.session AND a.callsign = s.callsign) AS assignment
FROM strips s JOIN unnest($1::int4[], $2::text[]) AS q(session, callsign)
 ON s.session = q.session AND s.callsign = q.callsign`, sessions, callsigns)
		results := make(map[positionKey]batchResult)
		if err == nil {
			var records []positionSnapshotDBRow
			records, err = pgx.CollectRows(rows, pgx.RowToStructByName[positionSnapshotDBRow])
			if err == nil {
				for _, row := range records {
					snapshot, conversionErr := positionSnapshotToModel(row.Strip, row.Assignment)
					results[positionKey{row.Session, row.Callsign}] = batchResult{value: snapshot, err: conversionErr}
				}
			}
		}
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
		for _, call := range group {
			v := call.input.(positionRead)
			result, found := results[positionKey{v.session, v.callsign}]
			if err != nil {
				result = batchResult{err: err}
			} else if !found {
				result = batchResult{err: pgx.ErrNoRows}
			}
			call.result <- result
		}
	}
}

func batchPositionWrites(calls []*batchCall) {
	groups := make(map[*stripRepository][]*batchCall)
	for _, call := range calls {
		if err := call.ctx.Err(); err != nil {
			call.result <- batchResult{err: err}
			continue
		}
		r := call.input.(positionWrite).repo
		groups[r] = append(groups[r], call)
	}
	for repo, group := range groups {
		if len(group) == 1 {
			v := group[0].input.(positionWrite)
			count, err := repo.UpdateAircraftPositionAndBay(context.WithValue(group[0].ctx, positionBatchKey{}, (*positionBatchMember)(nil)), v.session, v.callsign, v.lat, v.lon, v.alt, v.bay, v.sequence, v.version)
			group[0].result <- batchResult{value: count, err: err}
			continue
		}
		ctx, span := batchQueryContext(group, "persist")
		var sessions, sequences, versions []int32
		var callsigns, bays []string
		var lats, lons []*float64
		var alts []*int32
		seen := make(map[positionKey]bool)
		var err error
		for _, call := range group {
			v := call.input.(positionWrite)
			key := positionKey{v.session, v.callsign}
			if seen[key] {
				err = fmt.Errorf("duplicate aircraft in position database batch")
				break
			}
			seen[key] = true
			sessions = append(sessions, v.session)
			callsigns = append(callsigns, v.callsign)
			lats = append(lats, v.lat)
			lons = append(lons, v.lon)
			alts = append(alts, v.alt)
			bays = append(bays, v.bay)
			sequences = append(sequences, v.sequence)
			versions = append(versions, v.version)
		}
		updated := make(map[positionKey]bool)
		var tx pgx.Tx
		if err == nil {
			tx, err = repo.pool.Begin(ctx)
		}
		if err == nil {
			var rows pgx.Rows
			rows, err = tx.Query(ctx, `-- position batch persistence
UPDATE strips s SET position_latitude = q.lat, position_longitude = q.lon,
 position_altitude = q.alt, euroscope_seen_at = NOW(),
 sequence = CASE WHEN s.bay IS DISTINCT FROM q.bay THEN q.sequence ELSE s.sequence END,
 version = CASE WHEN s.bay IS DISTINCT FROM q.bay THEN s.version + 1 ELSE s.version END,
 bay = q.bay
FROM unnest($1::int4[], $2::text[], $3::float8[], $4::float8[], $5::int4[],
 $6::text[], $7::int4[], $8::int4[]) AS q(session, callsign, lat, lon, alt, bay, sequence, version)
WHERE s.session = q.session AND s.callsign = q.callsign AND s.version = q.version
RETURNING s.session, s.callsign`, sessions, callsigns, lats, lons, alts, bays, sequences, versions)
			if err == nil {
				for rows.Next() {
					var key positionKey
					if err = rows.Scan(&key.session, &key.callsign); err != nil {
						break
					}
					updated[key] = true
				}
				rows.Close()
				if err == nil {
					err = rows.Err()
				}
			}
		}
		if tx != nil {
			if err == nil {
				err = ctx.Err()
			}
			if err == nil {
				err = tx.Commit(ctx)
			}
			// No COMMIT is sent if cancellation interrupts the update. A cancelled
			// autocommit query can otherwise finish after its caller returns.
			rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
			_ = tx.Rollback(rollbackCtx)
			cancel()
		}
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
		var conflict *pgconn.PgError
		if errors.As(err, &conflict) && (conflict.Code == "40P01" || conflict.Code == "40001") {
			// PostgreSQL rolled back the entire statement. Let each service retry
			// once with a fresh snapshot on its ordinary individual path.
			err = nil
			clear(updated)
		}
		for _, call := range group {
			v := call.input.(positionWrite)
			var count int64
			if updated[positionKey{v.session, v.callsign}] {
				count = 1
			}
			call.result <- batchResult{value: count, err: err}
		}
	}
}
