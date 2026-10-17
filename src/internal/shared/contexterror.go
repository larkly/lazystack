package shared

import (
	"errors"
	"fmt"
)

// contextError is an error whose user-facing text was written by the
// caller: context such as "image X may be left behind" stays verbatim and
// only the API errors inside are reduced to their friendly message.
// Error() keeps the raw text for logs and the audit trail.
type contextError struct {
	raw      string
	friendly string
	causes   []error
}

func (e *contextError) Error() string   { return e.raw }
func (e *contextError) Unwrap() []error { return e.causes }

// ContextErrorf formats an error around one or more causes. Error arguments
// are shown raw by Error() and as ParseError(arg).FriendlyMessage by
// ParseError, so the context survives sanitizing. Use %v, not %w, for
// errors; they are unwrappable either way.
func ContextErrorf(format string, args ...any) error {
	friendly := make([]any, len(args))
	var causes []error
	for i, a := range args {
		friendly[i] = a
		if err, ok := a.(error); ok && err != nil {
			causes = append(causes, err)
			friendly[i] = ParseError(err).FriendlyMessage
		}
	}
	return &contextError{
		raw:      fmt.Sprintf(format, args...),
		friendly: fmt.Sprintf(format, friendly...),
		causes:   causes,
	}
}

// ContextError wraps cause with friendly as the text ParseError shows, for
// messages ContextErrorf cannot format, such as a list of failures.
func ContextError(cause error, friendly string) error {
	return &contextError{raw: cause.Error(), friendly: friendly, causes: []error{cause}}
}

// asContextError finds a contextError in err's chain.
func asContextError(err error) (*contextError, bool) {
	var ce *contextError
	return ce, errors.As(err, &ce)
}
