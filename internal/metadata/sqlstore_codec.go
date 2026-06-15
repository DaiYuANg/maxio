package metadata

import (
	"database/sql"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	codecx "github.com/arcgolabs/dbx/codec"
)

var metadataBoolIntCodec = codecx.New[bool]("bool_int", decodeBoolInt, encodeBoolInt)

func init() {
	codecx.MustRegister(metadataBoolIntCodec)
}

func decodeBoolInt(src any) (bool, error) {
	switch value := src.(type) {
	case nil:
		return false, nil
	case bool:
		return value, nil
	case []byte:
		return parseBoolInt(string(value))
	case sql.RawBytes:
		return parseBoolInt(string(value))
	case string:
		return parseBoolInt(value)
	default:
		return decodeNumericBoolInt(src)
	}
}

func decodeNumericBoolInt(src any) (bool, error) {
	value := reflect.ValueOf(src)
	if value.CanInt() {
		return value.Int() != 0, nil
	}
	if value.CanUint() {
		return value.Uint() != 0, nil
	}
	return false, fmt.Errorf("unsupported bool_int source %T", src)
}

func encodeBoolInt(value bool) (any, error) {
	if value {
		return 1, nil
	}
	return 0, nil
}

func parseBoolInt(raw string) (bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false, nil
	}
	if value, err := strconv.ParseBool(trimmed); err == nil {
		return value, nil
	}
	value, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return false, fmt.Errorf("parse bool_int: %w", err)
	}
	return value != 0, nil
}
