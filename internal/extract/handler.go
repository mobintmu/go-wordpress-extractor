package extract

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go-wordpress/internal/apikey"
	"go-wordpress/internal/config"
	"go-wordpress/internal/limiter"
	"go-wordpress/internal/metrics"
	"go-wordpress/internal/server/drain"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Request limits.
const (
	MaxBodyBytes     = 1 << 20
	MaxURLLength     = 2048
	MaxRequestIDSize = 128
)

// API error codes for non-200 responses.
const (
	CodeInvalidRequest  = "INVALID_REQUEST"
	CodeInvalidURL      = "INVALID_URL"
	CodeTooManyRequests = "TOO_MANY_REQUESTS"
	CodeShuttingDown    = "SHUTTING_DOWN"
)

// APIError is the body of 400, 401, 429, and 503 responses.
type APIError struct {
	Code    string `json:"code" binding:"required" example:"INVALID_URL"`
	Message string `json:"message" binding:"required" example:"The url must be an absolute http or https URL."`
} //@name ApiError

// Handler serves POST /api/v1/extract.
type Handler struct {
	svc     *Service
	limiter *limiter.Limiter
	metrics *metrics.Metrics
	drain   *drain.State
	cfg     *config.Config
	log     *zap.Logger
}

// NewHandler builds the handler.
func NewHandler(svc *Service, lim *limiter.Limiter, m *metrics.Metrics, d *drain.State, cfg *config.Config, log *zap.Logger) *Handler {
	m.SetActive(lim.Active)
	return &Handler{svc: svc, limiter: lim, metrics: m, drain: d, cfg: cfg, log: log}
}

func (h *Handler) reject(c *gin.Context, status int, code, msg, reason string) {
	h.metrics.Reject(reason)
	c.AbortWithStatusJSON(status, APIError{Code: code, Message: msg})
}

// Handle godoc
//
//	@Summary		Extract one product from a product page
//	@Description	Fetches one product page and returns the product (status success) or the root cause with evidence (status failed).
//	@Description	Both results return HTTP 200, so clients must read the status field. Only error.code HTTP_NOT_FOUND means the product is gone.
//	@Description	Each request responds within APP_MAX_REQUEST_DURATION (3 minutes); set the client timeout to 190 seconds.
//	@Tags			Extraction
//	@Accept			json
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			request	body		Request		true	"One product page URL"
//	@Success		200		{object}	Response	"Processing ran. Read the status field for the result."
//	@Failure		400		{object}	APIError	"INVALID_REQUEST: body is not valid JSON, exceeds 1 MiB, or url is missing. INVALID_URL: url is malformed or not http/https."
//	@Failure		401		{object}	APIError	"UNAUTHORIZED: the API key is missing, unknown, revoked, or expired."
//	@Failure		429		{object}	APIError	"TOO_MANY_REQUESTS: no capacity, or the site already has a running extraction."
//	@Header			429		{integer}	Retry-After	"Seconds to wait before retrying."
//	@Failure		503		{object}	APIError	"SHUTTING_DOWN: this instance is draining."
//	@Router			/api/v1/extract [post]
func (h *Handler) Handle(c *gin.Context) {
	if h.drain.Draining() {
		h.reject(c, http.StatusServiceUnavailable, CodeShuttingDown, "This instance is shutting down.", "shutting_down")
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxBodyBytes)
	var req Request
	dec := json.NewDecoder(c.Request.Body)
	if err := dec.Decode(&req); err != nil {
		var tooBig *http.MaxBytesError
		msg := "The request body must be a JSON object."
		if errors.As(err, &tooBig) {
			msg = fmt.Sprintf("The request body exceeds %d bytes.", MaxBodyBytes)
		}
		h.reject(c, http.StatusBadRequest, CodeInvalidRequest, msg, "invalid_request")
		return
	}
	if strings.TrimSpace(req.URL) == "" {
		h.reject(c, http.StatusBadRequest, CodeInvalidRequest, "The url field is required.", "invalid_request")
		return
	}
	if len(req.RequestID) > MaxRequestIDSize {
		h.reject(c, http.StatusBadRequest, CodeInvalidRequest, fmt.Sprintf("request_id must be at most %d characters.", MaxRequestIDSize), "invalid_request")
		return
	}
	target, err := ValidateURL(req.URL)
	if err != nil {
		h.reject(c, http.StatusBadRequest, CodeInvalidURL, err.Error(), "invalid_url")
		return
	}
	if req.RequestID == "" {
		req.RequestID = newRequestID()
	}

	release, ok := h.limiter.TryAcquire()
	if !ok {
		c.Header("Retry-After", "1")
		h.reject(c, http.StatusTooManyRequests, CodeTooManyRequests, "This instance is at capacity.", "capacity")
		return
	}
	defer release()

	ctx, cancel := context.WithTimeout(c.Request.Context(), h.cfg.MaxRequestDuration)
	defer cancel()

	resp, err := h.svc.Run(ctx, req.RequestID, target)
	if errors.Is(err, ErrSiteBusy) {
		c.Header("Retry-After", strconv.Itoa(int(math.Ceil(h.limiter.SiteWait().Seconds()))+1))
		h.reject(c, http.StatusTooManyRequests, CodeTooManyRequests, "This site already has a running extraction.", "site_busy")
		return
	}

	keyID, _ := c.Get(apikey.ContextKeyID)
	fields := []zap.Field{
		zap.String("request_id", resp.RequestID),
		zap.String("site_id", resp.SiteID),
		zap.String("config_version", resp.ConfigVersion),
		zap.Any("key_id", keyID),
		zap.String("url", resp.URL),
		zap.String("outcome", resp.Status),
		zap.Int64("duration_ms", resp.DurationMS),
	}
	code, priceState := "", ""
	if resp.Error != nil {
		code = resp.Error.Code
		fields = append(fields, zap.String("code", code), zap.String("stage", string(resp.Error.Stage)))
	}
	if resp.Product != nil {
		priceState = resp.Product.Price.State
		fields = append(fields, zap.String("price_state", priceState))
	}

	if c.Request.Context().Err() != nil {
		// The client went away; the work is discarded.
		h.metrics.ObserveExtraction("cancelled", resp.SiteID, code, priceState, time.Duration(resp.DurationMS)*time.Millisecond)
		h.log.Info("extraction cancelled by client", fields...)
		return
	}
	h.metrics.ObserveExtraction(resp.Status, resp.SiteID, code, priceState, time.Duration(resp.DurationMS)*time.Millisecond)
	h.log.Info("extraction finished", fields...)
	c.JSON(http.StatusOK, resp)
}

// ValidateURL checks that raw is an absolute http(s) URL with a host.
func ValidateURL(raw string) (*url.URL, error) {
	invalid := errors.New("The url must be an absolute http or https URL.")
	raw = strings.TrimSpace(raw)
	if len(raw) > MaxURLLength {
		return nil, fmt.Errorf("The url must be at most %d characters.", MaxURLLength)
	}
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.Opaque != "" {
		return nil, invalid
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "https" || u.Hostname() == "" {
		return nil, invalid
	}
	if u.User != nil {
		return nil, errors.New("The url must not contain user credentials.")
	}
	if p := u.Port(); p != "" {
		if n, err := strconv.Atoi(p); err != nil || n < 1 || n > 65535 {
			return nil, invalid
		}
	}
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	return u, nil
}

func newRequestID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "req_" + hex.EncodeToString(b)
}
