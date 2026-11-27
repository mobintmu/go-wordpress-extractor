// Package sites serves the list of websites the service can extract from.
package sites

import (
	"net/http"
	"sort"
	"time"

	"go-wordpress/internal/config"
	"go-wordpress/internal/siteconfig"

	"github.com/gin-gonic/gin"
)

// Site is one website that accepts extraction requests.
type Site struct {
	SiteID   string   `json:"site_id" binding:"required" example:"hmland"`
	Domains  []string `json:"domains" binding:"required" example:"hmland.ir,www.hmland.ir"`
	Version  string   `json:"version" binding:"required" example:"2026-11-11.1"`
	Template string   `json:"template" binding:"required" example:"woodmart"`
	// auto: the AI crawler reads the currency from each page.
	Currency string `json:"currency" binding:"required" enums:"IRT,USD,EUR,auto" example:"IRT"`
	// config: a file in configs/sites/. learned: selectors the AI crawler learned and proved on several pages. default: the built-in AI rule, which accepts any domain (domains is ["*"]).
	Origin string `json:"origin" binding:"required" enums:"config,learned,default" example:"config"`
	// Fetch engine. Set for the default rule and for sites that use an AI engine.
	Engine string `json:"engine,omitempty" enums:"ai-crawler-avalai,ai-crawler-cloudflare,ai-crawler-gapgpt" example:"ai-crawler-avalai"`
	// LLM model the AI engine uses. Set for the default rule and for sites that use an AI engine; empty when that engine is not configured on this instance.
	Model string `json:"model,omitempty" example:"deepseek-v4.1-flash"`
} //@name Site

// ListResponse is the body of GET /api/v1/sites.
type ListResponse struct {
	Count    int       `json:"count" binding:"required" example:"1"`
	LoadedAt time.Time `json:"loaded_at" binding:"required" example:"2026-11-11T08:00:00Z"`
	Sites    []Site    `json:"sites" binding:"required"`
	// True when APP_DEFAULT_ENGINE names an AI engine: a URL on any domain is accepted, not only the listed ones.
	AcceptsAnyDomain bool `json:"accepts_any_domain" binding:"required" example:"false"`
	// The engine serving websites without a rule (APP_DEFAULT_ENGINE). Absent when it is none.
	DefaultAIEngine string `json:"default_ai_engine,omitempty" enums:"ai-crawler-avalai,ai-crawler-cloudflare,ai-crawler-gapgpt" example:"ai-crawler-avalai"`
} //@name SiteList

// Handler serves GET /api/v1/sites.
type Handler struct {
	registry *siteconfig.Registry
	models   map[string]string
}

// NewHandler builds the handler.
func NewHandler(r *siteconfig.Registry, cfg *config.Config) *Handler {
	h := &Handler{registry: r, models: map[string]string{}}
	for _, e := range siteconfig.AIEngines {
		if m := cfg.AIModel(e); m != "" {
			h.models[e] = m
		}
	}
	return h
}

// List godoc
//
//	@Summary		List websites available for extraction
//	@Description	Returns the sites named in configs/active_sites.json whose config is valid and enabled, sorted by site_id.
//	@Description	Disabled and invalid configs, and active domains without a config, are left out. Without active_sites.json, every enabled site is returned.
//	@Description	The list follows config hot reloads; loaded_at is when the current configs were loaded.
//	@Description	Active configs learned by the AI crawler are listed with origin learned. When APP_DEFAULT_ENGINE names an AI engine, the list ends with the entry ai-crawler-default (origin default, domains ["*"]) showing that engine and its model, accepts_any_domain is true, and default_ai_engine names the engine.
//	@Tags			Extraction
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Success		200	{object}	ListResponse
//	@Failure		401	{object}	extract.APIError	"UNAUTHORIZED: the API key is missing, unknown, revoked, or expired."
//	@Router			/api/v1/sites [get]
func (h *Handler) List(c *gin.Context) {
	c.JSON(http.StatusOK, Build(h.registry.Snapshot(), h.models))
}

// Build lists the active, enabled, valid sites of a snapshot, then the
// active learned configs that serve a host, then the default AI rule.
// models maps each configured AI engine to its model.
func Build(snap *siteconfig.Snapshot, models map[string]string) ListResponse {
	out := ListResponse{LoadedAt: snap.LoadedAt.UTC(), Sites: []Site{}}
	add := func(e *siteconfig.Entry) {
		s := Site{
			SiteID:   e.SiteID,
			Domains:  append([]string{}, e.Site.Domains...),
			Version:  e.Version,
			Template: e.Template,
			Currency: e.Site.Locale.Currency(),
			Origin:   e.Origin,
		}
		// An unresolved "default" (APP_DEFAULT_ENGINE=none) is left out:
		// engine is always a concrete engine name in this response.
		if e.Site.IsAI() && e.Site.Fetch.Engine != siteconfig.EngineDefault {
			s.Engine, s.Model = e.Site.Fetch.Engine, models[e.Site.Fetch.Engine]
		}
		out.Sites = append(out.Sites, s)
	}
	for _, e := range snap.Entries {
		if e.Status() == "enabled" && snap.IsActive(e) {
			add(e)
		}
	}
	for _, e := range snap.Learned {
		// Listed only while it really serves its host: active, valid, and
		// no site config claims the host.
		if e.LearnedStatus() == siteconfig.LearnedActive && e.Status() == "enabled" && snap.Lookup(e.Domains[0]) == e {
			add(e)
		}
	}
	sort.Slice(out.Sites, func(i, j int) bool { return out.Sites[i].SiteID < out.Sites[j].SiteID })
	if d := snap.Default; d != nil {
		out.Sites = append(out.Sites, Site{
			SiteID:   d.SiteID,
			Domains:  []string{"*"},
			Version:  d.Version,
			Template: "",
			Currency: siteconfig.CurrencyAuto,
			Origin:   siteconfig.OriginDefault,
			Engine:   d.Site.Fetch.Engine,
			Model:    models[d.Site.Fetch.Engine],
		})
		out.AcceptsAnyDomain = true
		out.DefaultAIEngine = d.Site.Fetch.Engine
	}
	out.Count = len(out.Sites)
	return out
}
