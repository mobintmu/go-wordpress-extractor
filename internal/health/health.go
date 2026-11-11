package health

import (
	"net/http"

	"go-wordpress/internal/extract"
	"go-wordpress/internal/server/drain"

	"github.com/gin-gonic/gin"
)

// Health serves /health and /ready.
type Health struct {
	drain *drain.State
}

func New(d *drain.State) *Health {
	return &Health{drain: d}
}

// Handle godoc
//
//	@Summary	Check that the process is running
//	@Tags		Operations
//	@Produce	json
//	@Success	200	{object}	HealthResponse
//	@Router		/health [get]
func (h *Health) Handle(c *gin.Context) {
	c.JSON(http.StatusOK, HealthResponse{Message: "OK"})
}

// Ready godoc
//
//	@Summary	Check that this instance accepts new requests
//	@Tags		Operations
//	@Produce	json
//	@Success	200	{object}	HealthResponse
//	@Failure	503	{object}	extract.APIError	"SHUTTING_DOWN: this instance is draining."
//	@Router		/ready [get]
func (h *Health) Ready(c *gin.Context) {
	if h.drain.Draining() {
		c.JSON(http.StatusServiceUnavailable, extract.APIError{Code: "SHUTTING_DOWN", Message: "This instance is shutting down."})
		return
	}
	c.JSON(http.StatusOK, HealthResponse{Message: "OK"})
}

type HealthResponse struct {
	Message string `json:"message" binding:"required" example:"OK"`
} //@name HealthResponse
