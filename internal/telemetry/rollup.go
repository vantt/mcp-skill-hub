package telemetry

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// rollupSchema holds long-lived daily aggregates. Raw events expire after the
// short raw retention window; rollups keep day-granular counts for much longer
// so funnel and dead-skill reports do not depend on raw history.
const rollupSchema = `
CREATE TABLE IF NOT EXISTS telemetry_daily_rollups (
  day TEXT NOT NULL,
  skill_id TEXT NOT NULL,
  metric TEXT NOT NULL,
  count INTEGER NOT NULL CHECK (count >= 0),
  PRIMARY KEY (day, skill_id, metric)
) STRICT;`

const rollupDayLayout = "2006-01-02"

// RollupRow is one daily aggregate. SkillID is empty for overall metrics.
type RollupRow struct {
	Day     string `json:"day"`
	SkillID string `json:"skill_id"`
	Metric  string `json:"metric"`
	Count   int64  `json:"count"`
}

type rollupKey struct {
	skillID string
	metric  string
	count   int64
}

const (
	insertEventOrIgnoreSQL = `INSERT OR IGNORE INTO telemetry_events(id,occurred_at,kind,resolution_id,payload_json) VALUES(?,?,?,?,?)`
	insertEventSQL         = `INSERT INTO telemetry_events(id,occurred_at,kind,resolution_id,payload_json) VALUES(?,?,?,?,?)`
)

// insertEvent stores one raw event and, only when the row was actually
// inserted, increments its daily rollups in the same transaction. Replays of an
// existing event ID therefore never double count.
func insertEvent(ctx context.Context, tx *sql.Tx, envelope storedEnvelope, insertSQL string) (bool, error) {
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, insertSQL, envelope.ID, envelope.OccurredAt, envelope.Type, nullIfEmpty(envelope.ResolutionID), string(encoded))
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected != 1 {
		return false, nil
	}
	if len(envelope.OccurredAt) < len(rollupDayLayout) {
		return false, errors.New("stored event timestamp is too short to derive a rollup day")
	}
	day := envelope.OccurredAt[:len(rollupDayLayout)]
	keys, err := rollupKeys(ctx, envelope, tx)
	if err != nil {
		return false, err
	}
	for _, key := range keys {
		val := key.count
		if val == 0 {
			val = 1
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO telemetry_daily_rollups(day,skill_id,metric,count) VALUES(?,?,?,?) ON CONFLICT(day,skill_id,metric) DO UPDATE SET count = count + excluded.count`, day, key.skillID, key.metric, val); err != nil {
			return false, err
		}
	}
	return true, nil
}

// rollupKeys maps one validated event to its rollup keys. It is pure except for
// feedback events without a skill, which need one indexed lookup of the source
// resolution's top skill.
func rollupKeys(ctx context.Context, envelope storedEnvelope, tx *sql.Tx) ([]rollupKey, error) {
	payload := envelope.Payload
	text := func(field string) string { value, _ := payload[field].(string); return value }
	flag := func(field string) bool { value, _ := payload[field].(bool); return value }

	switch envelope.Type {
	case EventServerMetric:
		metricName := text("metric_name")
		val, _ := payload["metric_value"].(float64) // JSON numbers are float64
		return []rollupKey{{skillID: text("skill_id"), metric: metricName, count: int64(val)}}, nil
	case EventResolutionCompleted:
		var keys []rollupKey
		if status := text("status"); status != "" {
			keys = append(keys, rollupKey{skillID: "", metric: "resolution:" + status})
		}
		if state, top := text("setup_state"), text("top_skill_id"); state != "" && top != "" {
			keys = append(keys, rollupKey{skillID: top, metric: "setup:" + state})
		}
		return keys, nil
	case EventResolutionFailed:
		return []rollupKey{{skillID: "", metric: "resolution:failed"}}, nil
	case EventResolutionRecommended:
		top := text("top_skill_id")
		var keys []rollupKey
		if top != "" {
			keys = append(keys, rollupKey{skillID: top, metric: "recommended:primary"})
		}
		recommended, _ := stringTokens(payload["recommended_skill_ids"])
		for _, skillID := range recommended {
			if skillID != top {
				keys = append(keys, rollupKey{skillID: skillID, metric: "recommended:supporting"})
			}
		}
		return keys, nil
	case EventSkillDoctorChecked:
		return []rollupKey{{skillID: text("skill_id"), metric: "doctor:" + text("status")}}, nil
	case EventTranscriptToolObserved:
		tool := text("tool")
		keys := []rollupKey{{skillID: text("skill_id"), metric: "transcript:" + tool}}
		if tool == "Skill" {
			metric := "native:no_resolve"
			if flag("resolved_before") {
				metric = "native:resolved_before"
			}
			keys = append(keys, rollupKey{skillID: text("skill_id"), metric: metric})
		}
		return keys, nil
	}

	if envelope.Type == EventSkillLoaded && text("basis") == LoadBasisServerObserved {
		skillID := text("skill_id")
		if text("status") == "review_required" {
			return []rollupKey{{skillID: skillID, metric: "blocked:review_required"}}, nil
		}
		keys := []rollupKey{{skillID: skillID, metric: "load:" + text("resource_kind")}}
		if flag("first_activation") {
			keys = append(keys, rollupKey{skillID: skillID, metric: "activation:" + text("attribution")})
		}
		return keys, nil
	}

	if !feedbackEventType(envelope.Type) {
		return nil, nil
	}
	negative := false
	var metrics []string
	if envelope.Type == EventSkillUtilityReported {
		if text("utility") == "harmful" {
			negative = true
			if strings.HasSuffix(envelope.ID, ":utility") && tx != nil {
				primaryID := strings.TrimSuffix(envelope.ID, ":utility")
				var primaryStatus sql.NullString
				err := tx.QueryRowContext(ctx, `
SELECT json_extract(payload_json, '$.payload.status')
FROM telemetry_events
WHERE id = ?
  AND json_valid(payload_json)
LIMIT 1`, primaryID).Scan(&primaryStatus)
				if err == nil && primaryStatus.Valid {
					status := primaryStatus.String
					if status == "failed" || status == "rejected" || status == "abandoned" {
						negative = false
					}
				}
			}
		}
	} else if status := text("status"); status != "" {
		metrics = append(metrics, "feedback:"+status)
		negative = status == "failed" || status == "rejected" || status == "abandoned"
		// Only the primary event counts the reason, so a report that also
		// carries utility is not counted twice.
		if reasons, _ := stringTokens(payload["reason_codes"]); slices.Contains(reasons, FeedbackReasonSetupFailed) {
			metrics = append(metrics, "feedback:"+FeedbackReasonSetupFailed)
		}
	}
	if negative {
		metrics = append(metrics, "feedback:negative")
		if flag("after_load") {
			metrics = append(metrics, "feedback:negative_after_load")
		}
	}
	if len(metrics) == 0 {
		return nil, nil
	}
	skillID := text("skill_id")
	if skillID == "" && envelope.ResolutionID != "" {
		var err error
		if skillID, err = resolutionTopSkill(ctx, tx, envelope.ResolutionID); err != nil {
			return nil, err
		}
	}
	keys := make([]rollupKey, len(metrics))
	for index, metric := range metrics {
		keys[index] = rollupKey{skillID: skillID, metric: metric}
	}
	return keys, nil
}

// resolutionTopSkill returns the top skill recorded by the earliest resolver
// envelope of a resolution, or "" when none recorded one.
func resolutionTopSkill(ctx context.Context, tx *sql.Tx, resolutionID string) (string, error) {
	var topSkill sql.NullString
	err := tx.QueryRowContext(ctx, `
SELECT json_extract(payload_json,'$.payload.top_skill_id')
FROM telemetry_events
WHERE resolution_id = ?
  AND kind IN ('resolution.recommended','resolution.completed','resolution.failed')
  AND json_valid(payload_json)
  AND json_type(payload_json,'$.payload.top_skill_id') = 'text'
ORDER BY occurred_at,id
LIMIT 1`, resolutionID).Scan(&topSkill)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !topSkill.Valid || !validToken(topSkill.String) {
		return "", nil
	}
	return topSkill.String, nil
}

// pruneRollups removes daily aggregates older than the rollup retention.
func pruneRollups(ctx context.Context, tx *sql.Tx, config Config) error {
	cutoff := config.Clock().UTC().Add(-config.RollupRetention).Format(rollupDayLayout)
	_, err := tx.ExecContext(ctx, `DELETE FROM telemetry_daily_rollups WHERE day < ?`, cutoff)
	return err
}

func normalizeRollupRange(from, to string) (string, string, error) {
	for name, value := range map[string]string{"from": from, "to": to} {
		if value == "" {
			continue
		}
		parsed, err := time.Parse(rollupDayLayout, value)
		if err != nil || parsed.Format(rollupDayLayout) != value {
			return "", "", fmt.Errorf("rollup %s must be a YYYY-MM-DD day", name)
		}
	}
	if from != "" && to != "" && from > to {
		return "", "", errors.New("rollup from must not be after to")
	}
	return from, to, nil
}

// rollupsStore reads daily aggregates for the inclusive day range. An empty
// bound is open-ended. Rows are ordered by day, skill, and metric.
func rollupsStore(ctx context.Context, config Config, from, to string) ([]RollupRow, error) {
	if err := maintainStore(ctx, config); err != nil {
		return nil, err
	}
	database, err := openDatabase(ctx, config)
	if err != nil {
		return nil, err
	}
	defer database.Close()
	rows, err := database.QueryContext(ctx, `
SELECT day,skill_id,metric,count
FROM telemetry_daily_rollups
WHERE (? = '' OR day >= ?) AND (? = '' OR day <= ?)
ORDER BY day,skill_id,metric`, from, from, to, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []RollupRow{}
	for rows.Next() {
		var row RollupRow
		if err := rows.Scan(&row.Day, &row.SkillID, &row.Metric, &row.Count); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
