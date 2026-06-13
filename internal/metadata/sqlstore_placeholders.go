package metadata

import (
	"fmt"
	"strconv"
	"strings"
)

type placeholderRewriteState struct {
	builder      strings.Builder
	inSingle     bool
	inDouble     bool
	position     int
	prevWasSlash bool
}

func rewriteQuestionPlaceholdersToPostgres(query string) string {
	var state placeholderRewriteState
	for i := range len(query) {
		state.write(query[i])
	}
	return state.builder.String()
}

func (s *placeholderRewriteState) write(ch byte) {
	if s.writeEscaped(ch) {
		return
	}
	if s.writeQuote(ch) {
		return
	}
	if s.writePlaceholder(ch) {
		return
	}
	s.writeByte(ch)
}

func (s *placeholderRewriteState) writeEscaped(ch byte) bool {
	if s.prevWasSlash {
		s.prevWasSlash = false
		s.writeByte(ch)
		return true
	}
	if ch != '\\' {
		return false
	}
	s.prevWasSlash = true
	s.writeByte(ch)
	return true
}

func (s *placeholderRewriteState) writeQuote(ch byte) bool {
	if ch == '\'' && !s.inDouble {
		s.inSingle = !s.inSingle
		s.writeByte(ch)
		return true
	}
	if ch != '"' || s.inSingle {
		return false
	}
	s.inDouble = !s.inDouble
	s.writeByte(ch)
	return true
}

func (s *placeholderRewriteState) writePlaceholder(ch byte) bool {
	if ch != '?' || s.inSingle || s.inDouble {
		return false
	}
	s.position++
	s.writeString("$")
	s.writeString(strconv.Itoa(s.position))
	return true
}

func (s *placeholderRewriteState) writeByte(ch byte) {
	if err := s.builder.WriteByte(ch); err != nil {
		panic(fmt.Errorf("write query byte: %w", err))
	}
}

func (s *placeholderRewriteState) writeString(value string) {
	if _, err := s.builder.WriteString(value); err != nil {
		panic(fmt.Errorf("write query string: %w", err))
	}
}
