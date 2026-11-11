package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"log"
	"net/http"

	"go-wordpress/docs"
	"go-wordpress/internal/apikey"
	"go-wordpress/internal/config"
	"go-wordpress/internal/extract"
	"go-wordpress/internal/health"
	"go-wordpress/internal/metrics"
	"go-wordpress/internal/sites"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// RegisterRoutes wires every endpoint:
//
//	POST /api/v1/extract      API key in X-API-Key
//	GET  /api/v1/sites        API key in X-API-Key
//	GET  /metrics             API key in X-API-Key
//	GET  /health, /ready      public
//	GET  /swagger/*           operator credentials (Basic)
//
// Swagger docs are generated from the annotations with:
//
//	swag init --parseInternal -g cmd/server/main.go -o docs
func RegisterRoutes(engine *gin.Engine, h *health.Health, ex *extract.Handler, st *sites.Handler, keys *apikey.Store, m *metrics.Metrics, cfg *config.Config) {
	engine.GET("/health", h.Handle)
	engine.GET("/ready", h.Ready)

	v1 := engine.Group("/api/v1")
	auth := apikey.Middleware(keys, func() { m.Reject("unauthorized") })
	v1.POST("/extract", auth, ex.Handle)
	v1.GET("/sites", auth, st.List)

	// Failed scrapes are not counted in extract_rejected_total, which
	// describes client API traffic.
	engine.GET("/metrics", apikey.Middleware(keys, nil), MetricsHandler(m))

	// Swagger UI and its doc.json are served at /swagger/index.html and
	// /swagger/doc.json. Host is left empty so the UI calls the server it
	// was loaded from.
	docs.SwaggerInfo.Host = ""
	docs.SwaggerInfo.BasePath = "/"
	docs.SwaggerInfo.Schemes = []string{"http"}
	if cfg.TLSEnabled() {
		docs.SwaggerInfo.Schemes = []string{"https"}
	}
	if !cfg.OpsEnabled() {
		log.Printf("ℹ️  Swagger UI is off (it returns 404). Set APP_OPS_USER and APP_OPS_PASSWORD_HASH to turn it on.")
	} else {
		log.Printf("✅ Swagger UI: /swagger/index.html (operator login required, user %q, pepper fingerprint %s)",
			cfg.OpsUser, apikey.PepperFingerprint(cfg.APIKeyPepper))
	}
	ops := engine.Group("/", OpsAuth(cfg))
	ops.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	engine.NoRoute(notFound)
}

// MetricsHandler godoc
//
//	@Summary		Prometheus metrics
//	@Description	Requests by outcome, site, failure code, and price state; duration; active requests; rejections.
//	@Tags			Operations
//	@Produce		plain
//	@Security		ApiKeyAuth
//	@Success		200	{string}	string				"Metrics in the Prometheus text format"
//	@Failure		401	{object}	extract.APIError	"UNAUTHORIZED: the API key is missing, unknown, revoked, or expired."
//	@Router			/metrics [get]
func MetricsHandler(m *metrics.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		c.Status(http.StatusOK)
		m.Write(c.Writer)
	}
}

func notFound(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusNotFound, extract.APIError{Code: "NOT_FOUND", Message: "The requested endpoint does not exist."})
}

// OpsAuth protects operational endpoints with HTTP Basic authentication.
// The password is checked against APP_OPS_PASSWORD_HASH, a keyed hash made
// with the API key pepper. When either setting is missing the endpoints
// return 404. Client API keys are never accepted here.
func OpsAuth(cfg *config.Config) gin.HandlerFunc {
	wantUser := sha256.Sum256([]byte(cfg.OpsUser))
	return func(c *gin.Context) {
		if !cfg.OpsEnabled() {
			notFound(c)
			return
		}
		user, pass, ok := c.Request.BasicAuth()
		gotUser := sha256.Sum256([]byte(user))
		userOK := subtle.ConstantTimeCompare(gotUser[:], wantUser[:]) == 1
		passOK := apikey.HashEqual(cfg.APIKeyPepper, pass, cfg.OpsPasswordHash)
		if !ok || !userOK || !passOK {
			switch {
			case !ok:
				// No credentials yet: the browser shows its login prompt.
			case !userOK:
				log.Printf("operator login failed: unknown user %q", user)
			default:
				log.Printf("operator login failed for %q: wrong password, or the hash was made with a different APP_API_KEY_PEPPER (server fingerprint %s)",
					user, apikey.PepperFingerprint(cfg.APIKeyPepper))
			}
			c.Header("WWW-Authenticate", `Basic realm="operations", charset="UTF-8"`)
			c.AbortWithStatusJSON(http.StatusUnauthorized, extract.APIError{Code: "UNAUTHORIZED", Message: "Operator credentials are required."})
			return
		}
		c.Next()
	}
}
