package git

import "fmt"

type causedError struct {
	Cause   error
	Message string
}

func (e *causedError) Error() string {
	return e.Message + ": " + e.Cause.Error()
}

func newError(cause error, message ...interface{}) error {
	return &causedError{cause, fmt.Sprintln(message)}
}
