package httperror

import "net/http"

type HTTPError struct {
	Code    int
	Message string

	Cause error
}

func (e *HTTPError) Error() string {
	result := http.StatusText(e.Code)
	if e.Message != "" {
		result += ": " + e.Message
	}
	if e.Cause != nil {
		result += " - " + e.Cause.Error()
	}
	return result
}

func New(code int, message string, cause error) error {
	return &HTTPError{code, message, cause}
}

func BadRequest(message string, cause error) error {
	return New(http.StatusBadRequest, message, cause)
}

func Forbidden(message string) error {
	return New(http.StatusForbidden, message, nil)
}

func NotFound(message string) error {
	return New(http.StatusNotFound, message, nil)
}

func InternalError(message string, cause error) error {
	return New(http.StatusInternalServerError, message, cause)
}
