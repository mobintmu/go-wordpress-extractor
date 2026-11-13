package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"os"
	"strings"

	"go-wordpress/docs"
	"go-wordpress/internal/apikey"
	"go-wordpress/internal/config"
	"go-wordpress/internal/extract"
	"go-wordpress/internal/health"
	"go-wordpress/internal/limiter"
	"go-wordpress/internal/metrics"
	"go-wordpress/internal/openapi"
	"go-wordpress/internal/server/drain"
	"go-wordpress/internal/sites"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"go.uber.org/zap"
)

// RegisterRoutes wires every endpoint:
//
//	POST /api/v1/extract      API key in X-API-Key
//	GET  /api/v1/sites        API key in X-API-Key
//	GET  /api/v1/active       API key in X-API-Key
//	GET  /metrics             API key in X-API-Key (JSON, or Prometheus text)
//	GET  /health, /ready      public
//	GET  /swagger/*           operator credentials (Basic)
//
// Swagger 2.0 docs are generated from the annotations with:
//
//	swag init --parseInternal -g cmd/server/main.go -o docs
//
// At startup that doc is converted to OpenAPI 3 (with the request examples
// from internal/openapi) and served to Swagger UI as /swagger/doc.json.
func RegisterRoutes(engine *gin.Engine, h *health.Health, ex *extract.Handler, st *sites.Handler, keys *apikey.Store, m *metrics.Metrics, lim *limiter.Limiter, d *drain.State, cfg *config.Config) {
	engine.GET("/health", h.Handle)
	engine.GET("/ready", h.Ready)

	v1 := engine.Group("/api/v1")
	auth := apikey.Middleware(keys, func() { m.Reject("unauthorized") })
	v1.POST("/extract", auth, ex.Handle)
	v1.GET("/sites", auth, st.List)
	v1.GET("/active", auth, ActiveHandler(lim))

	// Failed scrapes are not counted in extract_rejected_total, which
	// describes client API traffic.
	engine.GET("/metrics", apikey.Middleware(keys, nil), MetricsHandler(m, lim, d))

	// Swagger UI is served at /swagger/index.html and its OpenAPI 3 doc at
	// /swagger/doc.json. The doc's server is "/", so the UI calls the
	// server it was loaded from.
	docs.SwaggerInfo.Host = ""
	docs.SwaggerInfo.BasePath = "/"
	docs.SwaggerInfo.Schemes = []string{"http"}
	if cfg.TLSEnabled() {
		docs.SwaggerInfo.Schemes = []string{"https"}
	}
	if !cfg.OpsEnabled() {
		zap.L().Info("Swagger UI is off (it returns 404); set APP_OPS_USER and APP_OPS_PASSWORD_HASH to turn it on")
	} else {
		zap.L().Info("Swagger UI enabled; operator login required",
			zap.String("path", "/swagger/index.html"), zap.String("user", cfg.OpsUser),
			zap.String("pepper_fingerprint", apikey.PepperFingerprint(cfg.APIKeyPepper)))
	}
	ops := engine.Group("/", OpsAuth(cfg))
	ops.GET("/swagger/*any", swaggerHandler())

	engine.NoRoute(notFound)
}

// swaggerHandler serves the OpenAPI 3 doc at /swagger/doc.json and the
// Swagger UI assets for every other /swagger/ path. A doc that fails to
// convert is a build bug, so it panics at startup instead of serving a
// broken UI.
func swaggerHandler() gin.HandlerFunc {
	doc, err := openapi.FromSwagger2([]byte(docs.SwaggerInfo.ReadDoc()))
	if err != nil {
		panic("swagger: " + err.Error())
	}
	ui := ginSwagger.WrapHandler(swaggerFiles.Handler)
	return func(c *gin.Context) {
		if c.Param("any") == "/doc.json" {
			c.Data(http.StatusOK, "application/json; charset=utf-8", doc)
			return
		}
		ui(c)
	}
}

// MetricsHandler godoc
//
//	@Summary		Instance load and metrics
//	@Description	Returns JSON by default: live load (capacity, active, available, busy sites, draining) and the counters since start.
//	@Description	Clients can poll each instance and send the next request to the one with the most available slots that is not draining.
//	@Description	Send Accept: text/plain (Prometheus does this) or ?format=prometheus to get the Prometheus text format instead.
//	@Tags			Operations
//	@Produce		json
//	@Produce		plain
//	@Security		ApiKeyAuth
//	@Param			format	query		string				false	"Response format"	Enums(json, prometheus)
//	@Success		200		{object}	metrics.Report		"JSON report, or the Prometheus text format when asked for"
//	@Failure		401		{object}	extract.APIError	"UNAUTHORIZED: the API key is missing, unknown, revoked, or expired."
//	@Router			/metrics [get]
func MetricsHandler(m *metrics.Metrics, lim *limiter.Limiter, d *drain.State) gin.HandlerFunc {
	instance, _ := os.Hostname()
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if wantsPrometheus(c) {
			c.Header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
			c.Status(http.StatusOK)
			m.Write(c.Writer)
			return
		}
		r := m.Snapshot()
		r.Instance = instance
		r.Load = load(lim, d)
		c.JSON(http.StatusOK, r)
	}
}

func load(lim *limiter.Limiter, d *drain.State) metrics.Load {
	l := metrics.Load{
		Draining:     d.Draining(),
		Capacity:     lim.Capacity(),
		Active:       lim.Active(),
		PerSiteLimit: lim.PerSite(),
		SitesActive:  lim.SitesActive(),
	}
	l.Available = max(l.Capacity-l.Active, 0)
	if l.Capacity > 0 {
		l.Utilization = float64(l.Active) / float64(l.Capacity)
	}
	return l
}

// wantsPrometheus picks the text format for ?format=prometheus, or when
// the Accept header names text/plain or OpenMetrics and not JSON, as
// Prometheus scrapers send.
func wantsPrometheus(c *gin.Context) bool {
	switch c.Query("format") {
	case "prometheus", "text":
		return true
	case "json":
		return false
	}
	accept := c.GetHeader("Accept")
	if strings.Contains(accept, "application/json") {
		return false
	}
	return strings.Contains(accept, "text/plain") || strings.Contains(accept, "application/openmetrics-text")
}

// ActiveResponse is the body of GET /api/v1/active.
type ActiveResponse struct {
	ExtractActiveRequests int `json:"extract_active_requests" binding:"required" example:"0"`
} //@name ActiveRequests

// ActiveHandler godoc
//
//	@Summary		Number of extractions running now
//	@Description	Returns how many extraction requests this instance is processing at this moment.
//	@Tags			Operations
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Success		200	{object}	ActiveResponse
//	@Failure		401	{object}	extract.APIError	"UNAUTHORIZED: the API key is missing, unknown, revoked, or expired."
//	@Router			/api/v1/active [get]
func ActiveHandler(lim *limiter.Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, ActiveResponse{ExtractActiveRequests: lim.Active()})
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
				zap.L().Warn("operator login failed: unknown user", zap.String("user", user))
			default:
				zap.L().Warn("operator login failed: wrong password, or the hash was made with a different APP_API_KEY_PEPPER",
					zap.String("user", user), zap.String("pepper_fingerprint", apikey.PepperFingerprint(cfg.APIKeyPepper)))
			}
			c.Header("WWW-Authenticate", `Basic realm="operations", charset="UTF-8"`)
			c.AbortWithStatusJSON(http.StatusUnauthorized, extract.APIError{Code: "UNAUTHORIZED", Message: "Operator credentials are required."})
			return
		}
		c.Next()
	}
}
