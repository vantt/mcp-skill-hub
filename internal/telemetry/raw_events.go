package telemetry

import (
	"context"
	"encoding/json"
	"time"
)

// rawEventsStore reads valid stored envelopes from the telemetry database
// within the time range [from, to]. Dates in YYYY-MM-DD format are expanded
// to full UTC day boundaries so they behave inclusively.
func rawEventsStore(ctx context.Context, config Config, from, to string) ([]Event, error) {
	if err := maintainStore(ctx, config); err != nil {
		return nil, err
	}
	database, err := openDatabase(ctx, config)
	if err != nil {
		return nil, err
	}
	defer database.Close()

	fromBound := from
	if len(from) == 10 {
		fromBound = from + "T00:00:00.000000000Z"
	}
	toBound := to
	if len(to) == 10 {
		toBound = to + "T23:59:59.999999999Z"
	}

	rows, err := database.QueryContext(ctx, `
SELECT id,occurred_at,kind,payload_json
FROM telemetry_events
WHERE (? = '' OR occurred_at >= ?) AND (? = '' OR occurred_at <= ?)
ORDER BY occurred_at,id`, fromBound, fromBound, toBound, toBound)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []Event{}
	for rows.Next() {
		var id, occurredAt, kind, raw string
		if err := rows.Scan(&id, &occurredAt, &kind, &raw); err != nil {
			return nil, err
		}
		var envelope storedEnvelope
		if err := json.Unmarshal([]byte(raw), &envelope); err != nil || validateStored(envelope, id, occurredAt, kind) != nil {
			continue
		}
		parsedTime, err := time.Parse(time.RFC3339Nano, envelope.OccurredAt)
		if err != nil {
			parsedTime, _ = time.Parse(time.RFC3339, envelope.OccurredAt)
		}
		result = append(result, Event{
			Version:         envelope.Version,
			ID:              envelope.ID,
			Type:            envelope.Type,
			OccurredAt:      parsedTime,
			SessionIDHash:   envelope.SessionIDHash,
			RequestID:       envelope.RequestID,
			ResolutionID:    envelope.ResolutionID,
			CatalogSnapshot: envelope.CatalogSnapshot,
			PolicyRevision:  envelope.PolicyRevision,
			Client:          envelope.Client,
			Payload:         envelope.Payload,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
