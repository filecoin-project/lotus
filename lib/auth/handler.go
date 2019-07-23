package auth

import (
	"net/http"
	"strings"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/gbrlsnchs/jwt/v3"
	logging "github.com/ipfs/go-log"
)

var log = logging.Logger("auth")

type Handler struct {
	Secret *jwt.HMACSHA
	Next http.HandlerFunc
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	token := r.Header.Get("Authorization")
	if token != "" {
		if !strings.HasPrefix(token, "Bearer ") {
			log.Warn("missing Bearer prefix in auth header")
			w.WriteHeader(401)
			return
		}
		token = token[len("Bearer "):]

		var payload jwtPayload
		if _, err := jwt.Verify([]byte(token), h.Secret, &payload); err != nil {
			log.Warnf("JWT Verification failed: %s", err)
			w.WriteHeader(401)
			return
		}

		ctx = api.WithPerm(ctx, payload.Allow)
	}

	h.Next(w, r.WithContext(ctx))
}

type jwtPayload struct {
	Allow []string
}

/*func init() {
	p := jwtPayload{
		Allow: []string{"read", "write"},
	}
	r, _ := jwt.Sign(&p, secret)
	log.Infof("WRITE TOKEN: %s", string(r))
}
*/