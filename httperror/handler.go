package httperror

import (
	"gopkg.in/macaron.v1"
	"log"
	"net/http"
	"reflect"
)

func Handler() macaron.ReturnHandler {
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

			httpError, ok := err.(*HTTPError)

			if !ok || httpError.Message != "" || httpError.Cause != nil {
				logger := ctx.GetVal(reflect.TypeOf((*log.Logger)(nil))).Interface().(*log.Logger)
				logger.Println(err)
			}

			if !ctx.Written() {
				if ok {
					http.Error(ctx.Resp, httpError.Message, httpError.Code)
				} else {
					http.Error(ctx.Resp, "Internal Server Error", http.StatusInternalServerError)
				}
			}
		default:
			panic("Unsupported number of return arguments")
		}
	}
}
