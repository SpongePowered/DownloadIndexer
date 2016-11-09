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

			loggerVal := ctx.GetVal(reflect.TypeOf((*log.Logger)(nil)))
			if loggerVal.IsValid() {
				loggerVal.Interface().(*log.Logger).Println(err)
			} else {
				log.Println(err)
			}

			if !ctx.Written() {
				if httpError, ok := err.(*HTTPError); ok {
					http.Error(ctx.Resp, httpError.Message, httpError.Code)
				} else {
					http.Error(ctx.Resp, "", http.StatusInternalServerError)
				}
			}
		default:
			panic("Unsupported number of return arguments")
		}
	}
}
