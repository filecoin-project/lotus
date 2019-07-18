package jsonrpc

import (
	"github.com/gbrlsnchs/jwt/v3"
)

var secret = jwt.NewHS256([]byte("todo: get me from the repo"))

type jwtPayload struct {
	Allow []string
}


