package auth

import (
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"gopkg.in/macaron.v1"
	"net/http"
)

const (
	authorizationHeader = "Authorization"
	authenticateHeader  = "WWW-Authenticate"
	basicAuth           = "Basic "
	authRequest         = basicAuth + "realm=\"Authorization Required\""
)

func Basic(auth []byte) macaron.Handler {
	auth = sha512Sum([]byte(basicAuth + base64.StdEncoding.EncodeToString(auth)))
	return func(ctx *macaron.Context) {
		val := ctx.Req.Header.Get(authorizationHeader)
		if val == "" || subtle.ConstantTimeCompare(auth, sha512Sum([]byte(val))) != 1 {
			ctx.Header().Set(authenticateHeader, authRequest)
			ctx.Resp.WriteHeader(http.StatusUnauthorized)
		}
	}
}

func sha512Sum(data []byte) []byte {
	sum := sha512.Sum512(data)
	return sum[:]
}
