package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Idempotency(db *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost && r.Method != http.MethodPut {
				next.ServeHTTP(w, r)
				return
			}
			key := r.Header.Get("Idempotency-Key")

			if key == "" {
				http.Error(w, "Idempotency-Key header required", http.StatusBadRequest)
				return
			}

			var cachedStatus int
			var cachedBody string

			err := db.QueryRow(r.Context(), `SELECT response_status, response_body FROM idempotency_keys
                 WHERE key = $1 AND created_at > $2`, key, time.Now().Add(-24*time.Hour)).Scan(&cachedStatus, &cachedBody)

			if err != nil {
				w.Header().Set("X-Idempotent-Replayed", "true")
				w.WriteHeader(cachedStatus)
				w.Write([]byte(cachedBody))
				return
			}

			ctx := context.WithValue(r.Context(), "idempotency_key", key)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
