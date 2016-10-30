package downloads

import (
	"gopkg.in/macaron.v1"
	"log"
	"net/http"
	"reflect"
)

type StatusError struct {
	Status  int
	Message string

	Cause error
}

func (e *StatusError) Error() string {
	result := http.StatusText(e.Status)
	if e.Message != "" {
		result += ": " + e.Message
	}
	if e.Cause != nil {
		result += " - " + e.Cause.Error()
	}
	return result
}

func Error(code int, message string, cause error) error {
	return &StatusError{code, message, cause}
}

func BadRequest(message string, cause error) error {
	return Error(http.StatusBadRequest, message, cause)
}

func Forbidden(message string) error {
	return Error(http.StatusForbidden, message, nil)
}

func NotFound(message string) error {
	return Error(http.StatusNotFound, message, nil)
}

func InternalError(message string, cause error) error {
	return Error(http.StatusInternalServerError, message, cause)
}

func ErrorHandler() macaron.ReturnHandler {
	return func(ctx *macaron.Context, vals []reflect.Value) {
		switch len(vals) {
		case 0:
			return
		case 1:
			if vals[0].IsNil() {
				if !ctx.Written() {
					ctx.Resp.WriteHeader(http.StatusOK)
				}
				return
			}

			err := vals[0].Interface().(error)

			loggerVal := ctx.GetVal(reflect.TypeOf((*log.Logger)(nil)))
			if loggerVal.IsValid() {
				loggerVal.Interface().(*log.Logger).Println(err)
			} else {
				log.Println(err)
			}

			if httpError, ok := err.(*StatusError); ok {
				ctx.Resp.WriteHeader(httpError.Status)
				if httpError.Message != "" {
					ctx.Write([]byte(httpError.Message))
				}
			} else {
				ctx.Resp.WriteHeader(http.StatusInternalServerError)
			}
		default:
			panic("Unsupported number of return arguments")
		}
	}
}
