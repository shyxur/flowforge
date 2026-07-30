package api

import (
	"net/http"

	"github.com/shyxur/flowforge/internal/ports"
	"go.uber.org/zap"
)

func NewRouter(h *Handler, auth Authenticator, limiter ports.RateLimiter, logger *zap.Logger) http.Handler {
	public := http.NewServeMux()
	public.HandleFunc("GET /healthz", h.Health)
	public.HandleFunc("GET /readyz", h.Ready)

	v1 := http.NewServeMux()
	v1.HandleFunc("POST /v1/tasks", h.CreateTask)
	v1.HandleFunc("GET /v1/tasks", h.ListTasks)
	v1.HandleFunc("GET /v1/tasks/{id}", h.GetTask)
	v1.HandleFunc("POST /v1/tasks/{id}/retry", h.RetryTask)
	v1.HandleFunc("POST /v1/tasks/{id}/cancel", h.CancelTask)
	v1.HandleFunc("DELETE /v1/tasks/{id}", h.DeleteTask)
	v1.HandleFunc("GET /v1/queues/{name}/stats", h.QueueStats)
	v1.HandleFunc("GET /v1/workers", h.ListWorkers)
	v1.HandleFunc("POST /v1/workers/{id}/heartbeat", h.WorkerHeartbeat)
	v1.HandleFunc("GET /v1/dlq", h.ListDLQ)
	v1.HandleFunc("POST /v1/dlq/{id}/requeue", h.RequeueDLQ)
	v1.HandleFunc("GET /v1/events/tasks", h.StreamTaskEvents)

	var protected http.Handler = v1
	protected = RateLimitMiddleware(limiter)(protected)
	protected = AuthMiddleware(auth)(protected)
	public.Handle("/v1/", protected)

	var handler http.Handler = public
	handler = LoggingMiddleware(logger)(handler)
	handler = RecoveryMiddleware(logger)(handler)
	return handler
}
