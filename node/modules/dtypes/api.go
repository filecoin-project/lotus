package dtypes

import (
	"time"

	"github.com/gbrlsnchs/jwt/v3"
	"github.com/multiformats/go-multiaddr"
)

type APIAlg jwt.HMACSHA

type APIEndpoint multiaddr.Multiaddr

type NodeStartTime time.Time
