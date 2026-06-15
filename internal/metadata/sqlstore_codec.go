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

func decodeJSON(raw sql.NullString, value any) error {
	if !raw.Valid || raw.String == "" || raw.String == "null" {
		return nil
	}
	if err := json.Unmarshal([]byte(raw.String), value); err != nil {
		return fmt.Errorf("unmarshal json: %w", err)
	}
	return nil
}

func marshalStrings(values []string) string {
	return marshalJSON(values, "[]")
}

func marshalInt64s(values []int64) string {
	return marshalJSON(values, "[]")
}

func marshalInt64sVariadic(values ...[]int64) string {
	if len(values) == 0 {
		return "[]"
	}
	return marshalInt64s(values[0])
}

func marshalUserMetadata(value map[string]string) string {
	if len(value) == 0 {
		return "null"
	}
	return marshalJSON(value, "null")
}

func marshalJSON(value any, fallback string) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return fallback
	}
	return string(raw)
}

func extractWriteIntentValues(intent *model.WriteIntent) (any, any, any, any) {
	if intent == nil {
		return nil, nil, nil, nil
	}
	return emptyStringOrNil(intent.ID), emptyStringOrNil(intent.Stage), unixNanoOrNil(intent.StartedAt), unixNanoOrNil(intent.UpdatedAt)
}

func emptyStringOrNil(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func emptyStringOrDefault(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func unixNanoOrNil(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().UnixNano()
}

func unixNanoToTime(timestamp int64) time.Time {
	if timestamp <= 0 {
		return time.Time{}
	}
	return time.Unix(0, timestamp).UTC()
}

func unixNanoToTimeOpt(value sql.NullInt64) time.Time {
	if !value.Valid || value.Int64 <= 0 {
		return time.Time{}
	}
	return unixNanoToTime(value.Int64)
}

func ensureContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
