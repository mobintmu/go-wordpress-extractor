package e2e

import (
	"strings"
	"testing"
)

func TestExtractSuccess(t *testing.T) {
	e := Start(t, Options{})
	s := e.Shop

	t.Run("sale price, lazy image, joined description, request_id echoed", func(t *testing.T) {
		r := e.Post("/api/v1/extract", e.Key, map[string]any{"url": s.URL(hostShop, "/product/ok/"), "request_id": "client-123"})
		r.Success(t)
		checks := map[string]string{
			"request_id":             "client-123",
			"site_id":                "shop",
			"config_version":         "2026-11-11.1",
			"url":                    s.URL(hostShop, "/product/ok/"),
			"product.title":          "بادام درختی",
			"product.url":            s.URL(hostShop, "/product/ok/"),
			"product.image_url":      s.URL(hostShop, "/uploads/almond.jpg"),
			"product.description":    "First paragraph.\nSecond paragraph.",
			"product.price.state":    "available",
			"product.price.raw":      "500,000 تومان 400,000 تومان",
			"product.price.currency": "IRT",
		}
		for path, want := range checks {
			if got := r.Str(path); got != want {
				t.Errorf("%s = %q, want %q", path, got, want)
			}
		}
		if r.Int("product.price.current") != 400000 || r.Int("product.price.regular") != 500000 {
			t.Errorf("prices = %d/%d, want 400000/500000", r.Int("product.price.current"), r.Int("product.price.regular"))
		}
		if n := len(r.At("warnings").([]any)); n != 0 {
			t.Errorf("want no warnings, got %s", r.Raw)
		}
		if r.Str("started_at") == "" || r.Str("finished_at") == "" || r.Int("duration_ms") < 0 {
			t.Errorf("timing fields missing: %s", r.Raw)
		}
	})

	t.Run("request_id is generated when missing", func(t *testing.T) {
		r := e.Post("/api/v1/extract", e.Key, map[string]any{"url": s.URL(hostShop, "/product/ok/")})
		r.Success(t)
		if !strings.HasPrefix(r.Str("request_id"), "req_") {
			t.Errorf("generated request_id = %q", r.Str("request_id"))
		}
	})

	t.Run("canonical on the site's domains wins", func(t *testing.T) {
		r := e.Extract(s.URL(hostShop, "/product/canonical/"))
		r.Success(t)
		if got, want := r.Str("product.url"), "http://"+hostWWW+":"+s.Port+"/product/canonical/"; got != want {
			t.Errorf("product.url = %s, want %s", got, want)
		}
	})

	t.Run("canonical on a foreign domain is ignored", func(t *testing.T) {
		r := e.Extract(s.URL(hostShop, "/product/foreign-canonical/"))
		r.Success(t)
		if got := r.Str("product.url"); got != s.URL(hostShop, "/product/foreign-canonical/") {
			t.Errorf("product.url = %s", got)
		}
	})

	t.Run("redirect within the site's domains", func(t *testing.T) {
		r := e.Extract(s.URL(hostShop, "/product/redirect-ok/"))
		r.Success(t)
		if got := r.Str("product.url"); got != "http://"+hostWWW+":"+s.Port+"/product/ok/" {
			t.Errorf("product.url = %s, want the final URL", got)
		}
	})

	t.Run("out of stock keeps the shown price", func(t *testing.T) {
		r := e.Extract(s.URL(hostShop, "/product/out-of-stock/"))
		r.Success(t)
		if r.Str("product.price.state") != "out_of_stock" || r.Int("product.price.current") != 120000 {
			t.Errorf("got %s", r.Raw)
		}
		if r.Has("product.price.regular") && r.At("product.price.regular") != nil {
			t.Errorf("regular should be null: %s", r.Raw)
		}
	})

	t.Run("missing order button means out of stock", func(t *testing.T) {
		r := e.Extract(s.URL(hostShop, "/product/no-button/"))
		r.Success(t)
		if r.Str("product.price.state") != "out_of_stock" || r.Int("product.price.current") != 400000 {
			t.Errorf("got %s", r.Raw)
		}
	})

	t.Run("call for price wins over out of stock", func(t *testing.T) {
		r := e.Extract(s.URL(hostShop, "/product/call/"))
		r.Success(t)
		if r.Str("product.price.state") != "call_for_price" {
			t.Errorf("state = %s", r.Str("product.price.state"))
		}
		if r.At("product.price.current") != nil || !r.Has("product.price.current") {
			t.Errorf("current must be null, never 0: %s", r.Raw)
		}
	})

	t.Run("price range returns the lowest price", func(t *testing.T) {
		r := e.Extract(s.URL(hostShop, "/product/range/"))
		r.Success(t)
		if r.Int("product.price.current") != 100000 {
			t.Errorf("current = %d, want 100000", r.Int("product.price.current"))
		}
	})

	t.Run("no price gives unknown with PRICE_MISSING and FIELD_MISSING", func(t *testing.T) {
		r := e.Extract(s.URL(hostShop, "/product/no-price/"))
		r.Success(t)
		if r.Str("product.price.state") != "unknown" || r.At("product.price.current") != nil {
			t.Errorf("got %s", r.Raw)
		}
		codes := map[string]string{}
		for _, w := range r.At("warnings").([]any) {
			wm := w.(map[string]any)
			codes[wm["code"].(string)+"/"+wm["field"].(string)] = wm["message"].(string)
		}
		if _, ok := codes["PRICE_MISSING/price"]; !ok {
			t.Errorf("want PRICE_MISSING warning: %s", r.Raw)
		}
		if _, ok := codes["FIELD_MISSING/description"]; !ok {
			t.Errorf("want FIELD_MISSING description warning: %s", r.Raw)
		}
		if r.At("product.description") != nil {
			t.Errorf("description must be null")
		}
	})

	t.Run("missing optional image gives a warning", func(t *testing.T) {
		r := e.Extract(s.URL(hostShop, "/product/no-image/"))
		r.Success(t)
		if r.Str("warnings.0.code") != "FIELD_MISSING" || r.Str("warnings.0.field") != "image_url" {
			t.Errorf("got %s", r.Raw)
		}
		if !r.Has("product.image_url") || r.At("product.image_url") != nil {
			t.Errorf("image_url must be present and null: %s", r.Raw)
		}
	})

	t.Run("rial prices are converted to toman", func(t *testing.T) {
		r := e.Extract(s.URL(hostRial, "/product/rial/"))
		r.Success(t)
		if r.Int("product.price.current") != 400000 || r.Int("product.price.regular") != 500000 {
			t.Errorf("got %s", r.Raw)
		}
		if !strings.Contains(r.Str("product.price.raw"), "ریال") {
			t.Errorf("raw must keep the page text: %s", r.Str("product.price.raw"))
		}
	})

	t.Run("windows-1256 page is decoded", func(t *testing.T) {
		r := e.Extract(s.URL(hostShop, "/product/windows-1256/"))
		r.Success(t)
		if r.Str("product.title") != "بادام ممتاز" {
			t.Errorf("title = %q", r.Str("product.title"))
		}
	})

	t.Run("robots.txt is fetched and respected for allowed paths", func(t *testing.T) {
		before := s.Hits("/robots.txt")
		e.Extract(s.URL(hostShop, "/product/ok/")).Success(t)
		if s.Hits("/robots.txt") <= before {
			t.Errorf("robots.txt was not fetched")
		}
	})
}
