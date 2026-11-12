// Package sites serves the list of websites the service can extract from.
package sites

import (
	"net/http"
	"sort"
	"time"

	"go-wordpress/internal/siteconfig"

	"github.com/gin-gonic/gin"
)

// Site is one website that accepts extraction requests.
type Site struct {
	SiteID   string   `json:"site_id" binding:"required" example:"hmland"`
	Domains  []string `json:"domains" binding:"required" example:"hmland.ir,www.hmland.ir"`
	Version  string   `json:"version" binding:"required" example:"2026-11-11.1"`
	Template string   `json:"template" binding:"required" example:"woodmart"`
	Currency string   `json:"currency" binding:"required" enums:"IRT,USD,EUR" example:"IRT"`
} //@name Site

// ListResponse is the body of GET /api/v1/sites.
type ListResponse struct {
	Count    int       `json:"count" binding:"required" example:"1"`
	LoadedAt time.Time `json:"loaded_at" binding:"required" example:"2026-11-11T08:00:00Z"`
	Sites    []Site    `json:"sites" binding:"required"`
} //@name SiteList

// Handler serves GET /api/v1/sites.
type Handler struct {
	registry *siteconfig.Registry
}

// NewHandler builds the handler.
func NewHandler(r *siteconfig.Registry) *Handler { return &Handler{registry: r} }

// List godoc
//
//	@Summary		List websites available for extraction
//	@Description	Returns the sites named in configs/active_sites.json whose config is valid and enabled, sorted by site_id.
//	@Description	Disabled and invalid configs, and active domains without a config, are left out. Without active_sites.json, every enabled site is returned.
//	@Description	The list follows config hot reloads; loaded_at is when the current configs were loaded.
//	@Tags			Extraction
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Success		200	{object}	ListResponse
//	@Failure		401	{object}	extract.APIError	"UNAUTHORIZED: the API key is missing, unknown, revoked, or expired."
//	@Router			/api/v1/sites [get]
func (h *Handler) List(c *gin.Context) {
	c.JSON(http.StatusOK, Build(h.registry.Snapshot()))
}

// Build lists the active, enabled, valid sites of a snapshot.
func Build(snap *siteconfig.Snapshot) ListResponse {
	out := ListResponse{LoadedAt: snap.LoadedAt.UTC(), Sites: []Site{}}
	for _, e := range snap.Entries {
		if e.Status() != "enabled" || !snap.IsActive(e) {
			continue
		}
		domains := append([]string{}, e.Site.Domains...)
		out.Sites = append(out.Sites, Site{
			SiteID:   e.SiteID,
			Domains:  domains,
			Version:  e.Version,
			Template: e.Template,
			Currency: e.Site.Locale.Currency(),
		})
	}
	sort.Slice(out.Sites, func(i, j int) bool { return out.Sites[i].SiteID < out.Sites[j].SiteID })
	out.Count = len(out.Sites)
	return out
}
