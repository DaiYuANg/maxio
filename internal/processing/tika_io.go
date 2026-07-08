package processing

import (
	"bytes"
	"fmt"
	"io"
)

func readLimitedBytes(reader io.Reader, limit int64) ([]byte, bool, error) {
	var buffer bytes.Buffer
	count, err := io.Copy(&buffer, io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, false, fmt.Errorf("copy limited bytes: %w", err)
	}
	if count > limit {
		return buffer.Bytes()[:limit], true, nil
	}
	return buffer.Bytes(), false, nil
}

func countLimited(reader io.Reader, limit int64) (int64, bool, error) {
	limited := io.LimitReader(reader, limit+1)
	count, err := io.Copy(io.Discard, limited)
	if err != nil {
		return 0, false, fmt.Errorf("count limited bytes: %w", err)
	}
	if count > limit {
		return limit, true, nil
	}
	return count, false, nil
}

func readTikaRequestBody(reader io.Reader, maxBytes int64) ([]byte, bool, error) {
	var buffer bytes.Buffer
	count, err := io.Copy(&buffer, io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, false, fmt.Errorf("copy tika request body: %w", err)
	}
	if count > maxBytes {
		return nil, true, nil
	}
	return buffer.Bytes(), false, nil
}
