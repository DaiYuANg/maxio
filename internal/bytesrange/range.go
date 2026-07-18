// Package bytesrange parses and applies HTTP byte ranges.
package bytesrange

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/samber/mo"
)

var ErrInvalidRange = errors.New("invalid byte range")

type Spec struct {
	Start   int64
	End     int64
	Size    int64
	Partial bool
}

func Parse(header string, size int64) (Spec, error) {
	if size < 0 {
		return Spec{}, fmt.Errorf("%w: negative size", ErrInvalidRange)
	}
	header = strings.TrimSpace(header)
	if header == "" {
		return Full(size), nil
	}
	if size == 0 {
		return Spec{}, fmt.Errorf("%w: empty object", ErrInvalidRange)
	}
	if !strings.HasPrefix(strings.ToLower(header), "bytes=") {
		return Spec{}, fmt.Errorf("%w: unsupported unit", ErrInvalidRange)
	}
	value := strings.TrimSpace(header[len("bytes="):])
	if value == "" || strings.Contains(value, ",") {
		return Spec{}, fmt.Errorf("%w: unsupported range set", ErrInvalidRange)
	}
	start, end, err := parseBounds(value, size)
	if err != nil {
		return Spec{}, err
	}
	return Spec{Start: start, End: end, Size: size, Partial: true}, nil
}

func Full(size int64) Spec {
	return Spec{Start: 0, End: size - 1, Size: size}
}

func UnsatisfiedContentRange(size int64) string {
	return "bytes */" + strconv.FormatInt(size, 10)
}

func (spec Spec) ContentLength() int64 {
	if spec.Size == 0 {
		return 0
	}
	return spec.End - spec.Start + 1
}

func (spec Spec) ContentRange() string {
	return fmt.Sprintf("bytes %d-%d/%d", spec.Start, spec.End, spec.Size)
}

func (spec Spec) Slice(data []byte) []byte {
	if !spec.Partial {
		return data
	}
	if spec.Start >= int64(len(data)) {
		return nil
	}
	end := min(spec.End, int64(len(data)-1))
	return data[spec.Start : end+1]
}

func parseBounds(value string, size int64) (int64, int64, error) {
	parts := strings.SplitN(value, "-", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("%w: missing separator", ErrInvalidRange)
	}
	if parts[0] == "" {
		return suffixBounds(parts[1], size)
	}
	start, err := parseNonNegative(parts[0])
	if err != nil {
		return 0, 0, err
	}
	end := size - 1
	if parts[1] != "" {
		end, err = parseNonNegative(parts[1])
		if err != nil {
			return 0, 0, err
		}
	}
	if start >= size || end < start {
		return 0, 0, fmt.Errorf("%w: unsatisfied range", ErrInvalidRange)
	}
	return start, min(end, size-1), nil
}

func suffixBounds(value string, size int64) (int64, int64, error) {
	suffix, err := parseNonNegative(value)
	if err != nil {
		return 0, 0, err
	}
	if suffix == 0 {
		return 0, 0, fmt.Errorf("%w: empty suffix", ErrInvalidRange)
	}
	if suffix >= size {
		return 0, size - 1, nil
	}
	return size - suffix, size - 1, nil
}

func parseNonNegative(value string) (int64, error) {
	result := mo.TupleToResult(strconv.ParseInt(strings.TrimSpace(value), 10, 64))
	if result.IsError() {
		return 0, fmt.Errorf("%w: invalid number", ErrInvalidRange)
	}
	number := result.OrElse(0)
	if number < 0 {
		return 0, fmt.Errorf("%w: invalid number", ErrInvalidRange)
	}
	return number, nil
}
