package telemetry

import (
	"context"
	"encoding/json"
	"time"
)

type CaseRecord struct {
	ResolutionID    string         `json:"-"`
	OccurredAt      time.Time      `json:"-"`
	Kind            string         `json:"-"`
	Client          Client         `json:"client"`
	CatalogSnapshot string         `json:"catalog_snapshot"`
	PriorVerified   bool           `json:"prior_verified"`
	Task            map[string]any `json:"task"`
	Operation       string         `json:"operation"`
	Request         map[string]any `json:"request"`
	Resolver        map[string]any `json:"resolver"`
	Chosen          string         `json:"chosen"`
	Followup        map[string]any `json:"followup,omitempty"`
}

func (r *Recorder) RecordCase(ctx context.Context, record CaseRecord) error {
	if r.config.ContentMode != ContentModeRedacted {
		return nil
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	r.gate.RLock()
	defer r.gate.RUnlock()
	if r.closed {
		return nil
	}
	database, err := openDatabase(ctx, r.config)
	if err != nil {
		return err
	}
	defer database.Close()

	day := record.OccurredAt.UTC().Format("2006-01-02")
	var count int
	err = database.QueryRowContext(ctx, "SELECT count(*) FROM telemetry_cases WHERE substr(occurred_at, 1, 10) = ?", day).Scan(&count)
	if err == nil && count >= 50 {
		return nil
	}

	err = database.QueryRowContext(ctx, "SELECT count(*) FROM telemetry_cases").Scan(&count)
	if err == nil && count >= 500 {
		_, _ = database.ExecContext(ctx, "DELETE FROM telemetry_cases WHERE resolution_id IN (SELECT resolution_id FROM telemetry_cases ORDER BY occurred_at ASC LIMIT 1)")
	}

	_, err = database.ExecContext(ctx, "INSERT OR IGNORE INTO telemetry_cases(resolution_id, occurred_at, kind, payload_json) VALUES(?, ?, ?, ?)",
		record.ResolutionID, record.OccurredAt.UTC().Format(time.RFC3339), record.Kind, string(encoded))
	return err
}

func (r *Recorder) PurgeCases(ctx context.Context) error {
	r.gate.RLock()
	defer r.gate.RUnlock()
	if r.closed {
		return nil
	}
	database, err := openDatabase(ctx, r.config)
	if err != nil {
		return err
	}
	defer database.Close()
	_, err = database.ExecContext(ctx, "DELETE FROM telemetry_cases")
	return err
}

func (r *Recorder) PruneCases(ctx context.Context) error {
	database, err := openDatabase(ctx, r.config)
	if err != nil {
		return err
	}
	defer database.Close()
	cutoff := time.Now().UTC().AddDate(0, 0, -90).Format(time.RFC3339)
	_, err = database.ExecContext(ctx, "DELETE FROM telemetry_cases WHERE occurred_at < ?", cutoff)
	return err
}

func (r *Recorder) Cases(ctx context.Context, since time.Time, kind string) ([]CaseRecord, error) {
	r.gate.RLock()
	defer r.gate.RUnlock()
	if r.closed {
		return nil, nil
	}
	database, err := openDatabase(ctx, r.config)
	if err != nil {
		return nil, err
	}
	defer database.Close()

	query := "SELECT payload_json FROM telemetry_cases WHERE 1=1"
	var args []any
	if !since.IsZero() {
		query += " AND occurred_at >= ?"
		args = append(args, since.UTC().Format(time.RFC3339))
	}
	if kind != "" {
		query += " AND kind = ?"
		args = append(args, kind)
	}
	query += " ORDER BY occurred_at ASC"

	rows, err := database.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cases []CaseRecord
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var record CaseRecord
		if err := json.Unmarshal([]byte(payload), &record); err != nil {
			return nil, err
		}
		cases = append(cases, record)
	}
	return cases, rows.Err()
}
