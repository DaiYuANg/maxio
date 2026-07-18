package processing

import (
	"fmt"
	"io"
)

func closeResource(closer io.Closer) {
	if err := closeResourceError(closer); err != nil {
		return
	}
}

func closeResourceError(closer io.Closer) error {
	if closer == nil {
		return nil
	}
	if err := closer.Close(); err != nil {
		return fmt.Errorf("close resource: %w", err)
	}
	return nil
}
