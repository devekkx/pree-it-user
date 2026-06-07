package router

import (
	"github.com/devekkx/pree-it-user/internal/handler"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

func Setup(h *handler.Handler) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(otelMiddleware())
	r.Use(requestIDMiddleware())

	// Health check — no auth required.
	r.GET("/health", h.HealthCheck)

	v1 := r.Group("/api/v1/users")
	{
		// Authenticated endpoints (gateway sets X-User-ID).
		v1.GET("/me", h.GetMyProfile)
		v1.PATCH("/me", h.UpdateMyProfile)
		v1.DELETE("/me", h.DeactivateMyProfile)

		v1.GET("/search", h.SearchProfiles)
		v1.POST("/batch", h.GetProfilesByIDs)

		v1.GET("/username/:username", h.GetProfileByUsername)
		v1.GET("/:id", h.GetProfileByID)
	}

	return r
}

// otelMiddleware extracts trace context from incoming requests.
func otelMiddleware() gin.HandlerFunc {
	propagator := otel.GetTextMapPropagator()
	return func(c *gin.Context) {
		ctx := propagator.Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// requestIDMiddleware ensures every response includes a request ID for tracing.
func requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := c.GetHeader("X-Request-ID")
		if reqID != "" {
			c.Writer.Header().Set("X-Request-ID", reqID)
		}
		c.Next()
	}
}
