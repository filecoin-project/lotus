package jsonrpc

import (
	"github.com/gbrlsnchs/jwt/v3"
)

var secret = jwt.NewHS256([]byte("todo: get me from the repo"))

type jwtPayload struct {
	Allow []string
}

func init() {
	p := jwtPayload{
		Allow: []string{"read", "write"},
	}
	r, _ := jwt.Sign(&p, secret)
	log.Infof("WRITE TOKEN: %s", string(r))
}
