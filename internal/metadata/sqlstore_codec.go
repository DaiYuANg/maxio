package metadata

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/lyonbrown4d/maxio/internal/model"
)

func marshalStrings(values []string) string {
	return marshalJSON(values, "[]")
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

func ensureContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
