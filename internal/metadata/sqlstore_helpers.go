package metadata

import (
	"context"
	"database/sql"
	"encoding/json"
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
