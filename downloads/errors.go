package downloads

import (
	"gopkg.in/macaron.v1"
	"log"
	"net/http"
	"reflect"
)

type HTTPError struct {
	Status  int
	Message string

	Cause error
}

func (e *HTTPError) Error() string {
	result := http.StatusText(e.Status)
	if e.Message != "" {
		result += ": " + e.Message
	}
	if e.Cause != nil {
		result += " - " + e.Cause.Error()
	}
	return result
}

func BadRequest(message string, cause error) error {
	return &HTTPError{http.StatusBadRequest, message, cause}
}

func InternalError(message string, cause error) error {
	return &HTTPError{http.StatusInternalServerError, message, cause}
}

func BadGateway(message string, cause error) error {
	return &HTTPError{http.StatusBadGateway, message, cause}
}

func ErrorHandler(log *log.Logger) macaron.ReturnHandler {
	return func(ctx *macaron.Context, vals []reflect.Value) {
		switch len(vals) {
		case 0:
			return
		case 1:
			if vals[0].IsNil() {
				if ctx.Resp.Status() == 0 {
					ctx.Status(http.StatusOK)
				}
				return
			}

			err := vals[0].Interface().(error)
			log.Println(err)

			httpError, ok := err.(*HTTPError)
			if ok {
				ctx.Status(httpError.Status)
				if httpError.Message != "" {
					ctx.Write([]byte(httpError.Message))
				}
			} else {
				ctx.Status(http.StatusInternalServerError)
			}
		default:
			panic("Unsupported number of return arguments")
		}
	}
}
