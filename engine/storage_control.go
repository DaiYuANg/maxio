package engine

import "strings"

func (e *Engine) SetControlToken(token string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.controlToken = strings.TrimSpace(token)
}
