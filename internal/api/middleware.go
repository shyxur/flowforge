package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/shyxur/windylane/internal/domain"
	"github.com/shyxur/windylane/internal/ports"
	"go.uber.org/zap"
)

type principalContextKey struct{}

type Authenticator interface {
	Authenticate(ctx context.Context, rawKey string) (*domain.Principal, error)
}

func AuthMiddleware(auth Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			scheme, rawKey, found := strings.Cut(header, " ")
			if !found || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(rawKey) == "" {
				writeAPIError(w, http.StatusUnauthorized, "unauthorized", "a valid Bearer API key is required", nil)
				return
			}
			principal, err := auth.Authenticate(r.Context(), strings.TrimSpace(rawKey))
			if err != nil {
				writeAPIError(w, http.StatusUnauthorized, "unauthorized", "a valid Bearer API key is required", nil)
				return
			}
			ctx := context.WithValue(r.Context(), principalContextKey{}, principal)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func MustPrincipal(ctx context.Context) *domain.Principal {
	principal, ok := ctx.Value(principalContextKey{}).(*domain.Principal)
	if !ok {
		panic("authenticated route missing principal")
	}
	return principal
}

type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriterWrapper) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriterWrapper) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (rw *responseWriterWrapper) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

func LoggingMiddleware(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			wrapper := &responseWriterWrapper{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(wrapper, r)
			logger.Info("http request",
				zap.String("method", r.Method), zap.String("path", r.URL.Path),
				zap.Int("status", wrapper.statusCode), zap.Duration("duration", time.Since(start)))
		})
	}
}

func RecoveryMiddleware(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					logger.Error("http handler panic recovered", zap.Any("error", err))
					writeAPIError(w, http.StatusInternalServerError, "internal_error", "internal server error", nil)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func RateLimitMiddleware(limiter ports.RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limiter == nil {
				next.ServeHTTP(w, r)
				return
			}
			principal := MustPrincipal(r.Context())
			key := "org:" + principal.OrgID.String() + ":api"
			ok, err := limiter.Allow(r.Context(), key)
			if err != nil || !ok {
				writeAPIError(w, http.StatusTooManyRequests, "rate_limit_exceeded", "rate limit exceeded", nil)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
