package metadata

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lyonbrown4d/maxio/internal/model"
)

func ensureContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func marshalUserMetadata(values map[string]string) string {
	if len(values) == 0 {
		return "{}"
	}
	data, err := json.Marshal(values)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func unixNanoOrNil(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UnixNano()
}

func extractWriteIntentValues(intent *model.WriteIntent) (any, any, any, any) {
	if intent == nil || strings.TrimSpace(intent.ID) == "" {
		return nil, nil, nil, nil
	}
	stage := strings.TrimSpace(intent.Stage)
	if stage == "" {
		stage = model.WriteIntentStageUnknown
	}
	return strings.TrimSpace(intent.ID), stage, unixNanoOrNil(intent.StartedAt), unixNanoOrNil(intent.UpdatedAt)
}

func emptyStringOrDefault(value sql.NullString) string {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return model.WriteIntentStageUnknown
	}
	return value.String
}

func affectedRowCount(result sql.Result, op string) (int64, error) {
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("%s rows: %w", op, err)
	}
	return affected, nil
}

func hasAffectedRow(result sql.Result, op string) (bool, error) {
	affected, err := affectedRowCount(result, op)
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func requireAffectedRow(result sql.Result, missing error, op string) error {
	affected, err := affectedRowCount(result, op)
	if err != nil {
		return err
	}
	if affected == 0 {
		return missing
	}
	return nil
}
