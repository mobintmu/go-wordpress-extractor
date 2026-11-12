package e2e

import (
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"go-wordpress/internal/apikey"
)

const sitesPath = "/api/v1/sites"

// listSites calls GET /api/v1/sites with key and returns the sites by id.
func (e *Env) listSites(t *testing.T, key string) (*Result, map[string]map[string]any) {
	t.Helper()
	r := e.Get(sitesPath, map[string]string{"X-API-Key": key})
	if r.Status != http.StatusOK {
		t.Fatalf("list sites: %s", r)
	}
	list, ok := r.At("sites").([]any)
	if !ok {
		t.Fatalf("sites is not a list: %s", r.Raw)
	}
	if int64(len(list)) != r.Int("count") {
		t.Errorf("count %d does not match %d sites", r.Int("count"), len(list))
	}
	byID := map[string]map[string]any{}
	prev := ""
	for _, s := range list {
		site := s.(map[string]any)
		id := site["site_id"].(string)
		if id <= prev {
			t.Errorf("sites are not sorted by site_id: %q after %q", id, prev)
		}
		prev = id
		byID[id] = site
	}
	return r, byID
}

func TestListSitesAuthentication(t *testing.T) {
	e := Start(t, Options{})
	id, _ := apikey.Parse(e.Key)

	var first []byte
	for name, key := range map[string]string{
		"missing":      "",
		"malformed":    "not-a-key",
		"unknown id":   apikey.Format("zzzzzzzz", strings.Repeat("A", apikey.SecretLength)),
		"wrong secret": apikey.Format(id, strings.Repeat("A", apikey.SecretLength)),
		"revoked":      e.Revoked,
		"expired":      e.Expired,
	} {
		t.Run(name, func(t *testing.T) {
			headers := map[string]string{}
			if key != "" {
				headers["X-API-Key"] = key
			}
			r := e.Get(sitesPath, headers)
			r.APIError(t, http.StatusUnauthorized, "UNAUTHORIZED")
			if first == nil {
				first = r.Raw
			} else if string(first) != string(r.Raw) {
				t.Errorf("every 401 must be identical: %s vs %s", first, r.Raw)
			}
		})
	}
	t.Run("operator credentials are not accepted", func(t *testing.T) {
		e.Get(sitesPath, map[string]string{"Authorization": opsAuthHeader()}).APIError(t, http.StatusUnauthorized, "UNAUTHORIZED")
	})
	t.Run("valid key", func(t *testing.T) {
		e.listSites(t, e.Key)
	})
}

func TestListSitesContent(t *testing.T) {
	e := Start(t, Options{ExtraSites: map[string]map[string]any{
		"usd.test": {
			"schema_version": 1, "site_id": "usd", "version": "u1", "extends": "woocommerce",
			"domains": []string{"usd.test"},
			"fetch":   map[string]any{"delay_ms": 0, "retries": 0},
			"locale":  map[string]any{"price_unit": "usd"},
		},
		"eur.test": {
			"schema_version": 1, "site_id": "eur", "version": "u1", "extends": "woocommerce",
			"domains": []string{"eur.test"},
			"fetch":   map[string]any{"delay_ms": 0, "retries": 0},
			"locale":  map[string]any{"price_unit": "eur"},
		},
	}})
	r, sites := e.listSites(t, e.Key)

	t.Run("enabled sites are listed with their details", func(t *testing.T) {
		shop, ok := sites["shop"]
		if !ok {
			t.Fatalf("site shop is missing: %s", r.Raw)
		}
		if d, _ := shop["domains"].([]any); len(d) != 2 || d[0] != hostShop || d[1] != hostWWW {
			t.Errorf("shop domains = %v", shop["domains"])
		}
		if shop["version"] != "2026-11-11.1" || shop["template"] != "woocommerce" || shop["currency"] != "IRT" {
			t.Errorf("shop = %v", shop)
		}
	})

	t.Run("currency follows the price unit", func(t *testing.T) {
		for id, want := range map[string]string{"rial": "IRT", "usd": "USD", "eur": "EUR"} {
			if got := sites[id]["currency"]; got != want {
				t.Errorf("%s currency = %v, want %s", id, got, want)
			}
		}
	})

	t.Run("disabled and invalid sites are left out", func(t *testing.T) {
		for _, id := range []string{"disabled", "broken"} {
			if _, ok := sites[id]; ok {
				t.Errorf("site %s is listed", id)
			}
		}
	})

	t.Run("loaded_at is a recent RFC 3339 time", func(t *testing.T) {
		at, err := time.Parse(time.RFC3339Nano, r.Str("loaded_at"))
		if err != nil {
			t.Fatalf("loaded_at %q: %v", r.Str("loaded_at"), err)
		}
		if age := time.Since(at); age < 0 || age > time.Minute {
			t.Errorf("loaded_at %s is not recent", at)
		}
	})

	t.Run("listing does not contact any site", func(t *testing.T) {
		if n := e.Shop.TotalHits(); n != 0 {
			t.Errorf("the shop was fetched %d times", n)
		}
	})
}

func TestListSitesHotReload(t *testing.T) {
	e := Start(t, Options{})
	first, sites := e.listSites(t, e.Key)
	if _, ok := sites["newsite"]; ok {
		t.Fatal("newsite is listed before its file exists")
	}

	t.Run("a new site appears", func(t *testing.T) {
		e.WriteSite("new.test", map[string]any{
			"schema_version": 1, "site_id": "newsite", "version": "v2", "extends": "woocommerce",
			"domains": []string{"new.test"},
			"fetch":   map[string]any{"delay_ms": 0, "retries": 0},
		})
		eventually(t, func() bool {
			r, s := e.listSites(t, e.Key)
			return s["newsite"]["version"] == "v2" && r.Str("loaded_at") != first.Str("loaded_at")
		})
	})

	t.Run("a disabled site disappears", func(t *testing.T) {
		e.WriteSite("new.test", map[string]any{
			"schema_version": 1, "site_id": "newsite", "version": "v3", "extends": "woocommerce",
			"enabled": false, "domains": []string{"new.test"},
			"fetch": map[string]any{"delay_ms": 0, "retries": 0},
		})
		eventually(t, func() bool {
			_, s := e.listSites(t, e.Key)
			_, listed := s["newsite"]
			return !listed
		})
	})
}

func TestListSitesActiveFile(t *testing.T) {
	e := Start(t, Options{})
	ids := func() []string {
		_, s := e.listSites(t, e.Key)
		out := make([]string, 0, len(s))
		for id := range s {
			out = append(out, id)
		}
		sort.Strings(out)
		return out
	}

	t.Run("without the file every enabled site is listed", func(t *testing.T) {
		if got := ids(); len(got) < 3 {
			t.Errorf("sites = %v", got)
		}
	})

	t.Run("only active sites are listed", func(t *testing.T) {
		// www. matches shop by its second domain; disabled and broken stay
		// out; unknown.test has no config.
		e.WriteActive(`{"active": ["` + hostWWW + `", "` + hostRial + `", "` + hostDisabled + `", "broken.test", "unknown.test"]}`)
		eventually(t, func() bool { return strings.Join(ids(), ",") == "rial,shop" })
	})

	t.Run("an invalid file keeps the previous list", func(t *testing.T) {
		e.WriteActive(`{"active": [`)
		time.Sleep(time.Second)
		if got := strings.Join(ids(), ","); got != "rial,shop" {
			t.Errorf("sites = %s", got)
		}
	})

	t.Run("an empty list lists nothing", func(t *testing.T) {
		e.WriteActive(`{"active": []}`)
		eventually(t, func() bool { return len(ids()) == 0 })
	})
}
