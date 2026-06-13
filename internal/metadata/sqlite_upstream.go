package metadata

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/lyonbrown4d/maxio/model"
)

const sqliteUpstreamColumns = `id, name, endpoint, region, weight, priority, buckets, enabled, created_at, updated_at`

const sqliteUpstreamUpsertSQL = `INSERT INTO metadata_upstreams (
	id, name, endpoint, region, weight, priority, buckets, enabled, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	name = excluded.name,
	endpoint = excluded.endpoint,
	region = excluded.region,
	weight = excluded.weight,
	priority = excluded.priority,
	buckets = excluded.buckets,
	enabled = excluded.enabled,
	updated_at = excluded.updated_at`

func (s *SQLiteMetadata) ListUpstreams(ctx context.Context) ([]model.Upstream, error) {
	rows, err := s.queryContext(
		ctx,
		`SELECT `+sqliteUpstreamColumns+`
		   FROM metadata_upstreams
		  ORDER BY priority ASC, name ASC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("query upstreams: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && s.logger != nil {
			s.logger.Error("close sqlite rows", "rows", "upstreams", "error", closeErr)
		}
	}()

	upstreams := make([]model.Upstream, 0)
	for rows.Next() {
		upstream, err := scanUpstream(rows)
		if err != nil {
			return nil, err
		}
		upstreams = append(upstreams, upstream)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate upstreams: %w", err)
	}
	return upstreams, nil
}

func (s *SQLiteMetadata) GetUpstream(ctx context.Context, id string) (model.Upstream, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return model.Upstream{}, false, ErrBadRequest
	}

	row := s.queryRowContext(
		ctx,
		`SELECT `+sqliteUpstreamColumns+`
		   FROM metadata_upstreams
		  WHERE id = ?
		  LIMIT 1`,
		id,
	)
	upstream, err := scanUpstream(row)
	if err != nil {
		if errorsIsNoRows(err) {
			return model.Upstream{}, false, nil
		}
		return model.Upstream{}, false, fmt.Errorf("query upstream: %w", err)
	}
	return upstream, true, nil
}

func (s *SQLiteMetadata) UpsertUpstream(ctx context.Context, upstream model.Upstream) (model.Upstream, error) {
	upstream, err := normalizeUpstream(upstream)
	if err != nil {
		return model.Upstream{}, err
	}

	now := time.Now().UTC()
	if upstream.CreatedAt.IsZero() {
		upstream.CreatedAt = now
	}
	upstream.UpdatedAt = now

	_, execErr := s.execContext(
		ctx,
		sqliteUpstreamUpsertSQL,
		upstream.ID,
		upstream.Name,
		upstream.Endpoint,
		upstream.Region,
		upstream.Weight,
		upstream.Priority,
		marshalStrings(upstream.Buckets),
		boolToInt(upstream.Enabled),
		upstream.CreatedAt.UnixNano(),
		upstream.UpdatedAt.UnixNano(),
	)
	if execErr != nil {
		return model.Upstream{}, fmt.Errorf("upsert upstream: %w", execErr)
	}
	stored, _, err := s.GetUpstream(ctx, upstream.ID)
	if err != nil {
		return model.Upstream{}, err
	}
	return stored, nil
}

func (s *SQLiteMetadata) DeleteUpstream(ctx context.Context, id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, ErrBadRequest
	}

	result, err := s.execContext(ctx, "DELETE FROM metadata_upstreams WHERE id = ?", id)
	if err != nil {
		return false, fmt.Errorf("delete upstream: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete upstream rows: %w", err)
	}
	return affected > 0, nil
}

func scanUpstream(scanner interface{ Scan(dest ...any) error }) (model.Upstream, error) {
	var (
		upstream model.Upstream
		buckets  sql.NullString
		enabled  int
		created  int64
		updated  int64
	)
	if err := scanner.Scan(
		&upstream.ID,
		&upstream.Name,
		&upstream.Endpoint,
		&upstream.Region,
		&upstream.Weight,
		&upstream.Priority,
		&buckets,
		&enabled,
		&created,
		&updated,
	); err != nil {
		return model.Upstream{}, fmt.Errorf("scan upstream: %w", err)
	}
	if err := decodeJSON(buckets, &upstream.Buckets); err != nil {
		return model.Upstream{}, fmt.Errorf("decode upstream buckets: %w", err)
	}
	upstream.Enabled = enabled != 0
	upstream.CreatedAt = unixNanoToTime(created)
	upstream.UpdatedAt = unixNanoToTime(updated)
	return upstream, nil
}

func normalizeUpstream(upstream model.Upstream) (model.Upstream, error) {
	upstream.ID = strings.TrimSpace(upstream.ID)
	upstream.Name = strings.TrimSpace(upstream.Name)
	upstream.Endpoint = strings.TrimSpace(upstream.Endpoint)
	upstream.Region = strings.TrimSpace(upstream.Region)
	if upstream.ID == "" {
		upstream.ID = upstream.Name
	}
	if upstream.Name == "" {
		upstream.Name = upstream.ID
	}
	if upstream.ID == "" || upstream.Name == "" || upstream.Endpoint == "" {
		return model.Upstream{}, ErrBadRequest
	}
	upstream.Buckets = normalizeStringList(upstream.Buckets)
	return upstream, nil
}

func normalizeStringList(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
