package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/filecoin-project/go-lotus/api"
	logging "github.com/ipfs/go-log"
)

var log = logging.Logger("auth")

type Handler struct {
	Verify func(ctx context.Context, token string) ([]string, error)
	Next   http.HandlerFunc
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

		allow, err := h.Verify(ctx, token)
		if err != nil {
			log.Warnf("JWT Verification failed: %s", err)
			w.WriteHeader(401)
			return
		}

		ctx = api.WithPerm(ctx, allow)
	}

	h.Next(w, r.WithContext(ctx))
}
