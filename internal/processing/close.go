package processing

import "io"

func closeResource(closer io.Closer) {
	if closer == nil {
		return
	}
	if err := closer.Close(); err != nil {
		_ = err
	}
}
