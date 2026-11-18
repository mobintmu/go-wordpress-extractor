package fetch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"unicode/utf8"

	"go-wordpress/internal/aicrawler"
	"go-wordpress/internal/diagnose"
	"go-wordpress/internal/siteconfig"

	"github.com/PuerkitoBio/goquery"
	"go.uber.org/zap"
)

// EngineAICrawler is the AI crawler's engine name.
const EngineAICrawler = siteconfig.EngineAICrawler

// AIPage is what the AI crawler produced for a page: the model's answer and
// the cleaned page it must be checked against.
type AIPage struct {
	Product       *aicrawler.Product
	Clean         *aicrawler.Page
	Model         string
	PromptVersion string
	// FetchedWith is the engine that fetched the HTML: "http" or "browser".
	FetchedWith string
	Cached      bool
	// Evidence describes the LLM call, for failures and logs.
	Evidence diagnose.AIEvidence
}

// AIEngine is the "ai-crawler-avalai" strategy. It fetches the page with
// the http engine (or the browser engine), cleans it and asks an LLM for the
// product. The page body stays in the Page, so it serves as evidence and
// for selector learning.
type AIEngine struct {
	http     *Fetcher
	browser  *BrowserFetcher
	client   *aicrawler.Client
	cache    *aicrawler.Cache
	maxChars int
	schema   map[string]any
	log      *zap.Logger
}

// NewAIEngine builds the engine; it returns nil when client is nil (the AI
// crawler is off), so the router skips it. browser may be nil.
func NewAIEngine(httpEngine *Fetcher, browser *BrowserFetcher, client *aicrawler.Client, cache *aicrawler.Cache, maxChars int, log *zap.Logger) *AIEngine {
	if client == nil {
		return nil
	}
	return &AIEngine{http: httpEngine, browser: browser, client: client, cache: cache, maxChars: maxChars, schema: aicrawler.ProductSchema(), log: log}
}

// Name makes the AI crawler the "ai-crawler-avalai" strategy.
func (e *AIEngine) Name() string { return EngineAICrawler }

// Client returns the LLM client, for selector learning.
func (e *AIEngine) Client() *aicrawler.Client { return e.client }

// Fetch fetches target, then extracts the product with the LLM.
func (e *AIEngine) Fetch(ctx context.Context, site *siteconfig.Site, target *url.URL) (*Page, *diagnose.Failure) {
	browserFirst := site.Fetch.AIFetch == EngineBrowser
	if browserFirst && e.browser == nil {
		return &Page{RequestedURL: target.String()}, diagnose.New(diagnose.EngineUnavailable, diagnose.ConfidenceHigh,
			"The site config sets fetch.ai_fetch to \"browser\", but the browser engine is not enabled on this instance.").
			WithAction("Set APP_BROWSER_ENABLED=true on this instance, or remove fetch.ai_fetch from the site config.")
	}

	fetchedWith := EngineHTTP
	var page *Page
	var fail *diagnose.Failure
	if browserFirst {
		fetchedWith = EngineBrowser
		page, fail = e.browser.Fetch(ctx, site, target)
	} else {
		page, fail = e.http.Fetch(ctx, site, target)
	}
	if fail != nil {
		return page, fail
	}
	if f := checkPage(page); f != nil {
		// A JavaScript shell is rendered once in Chrome when the browser
		// engine is enabled (APP_BROWSER_ENABLED).
		if f.Code != diagnose.JSRenderedPage || browserFirst || e.browser == nil {
			return page, f
		}
		e.log.Debug("ai crawler: javascript page, rendering in the browser", zap.String("url", target.String()))
		fetchedWith = EngineBrowser
		page, fail = e.browser.Fetch(ctx, site, target)
		if fail != nil {
			return page, fail
		}
		if f := checkPage(page); f != nil {
			return page, f
		}
	}

	final := page.FinalURL
	if final == nil {
		final = target
	}
	clean, err := aicrawler.Clean(page.Body, final, e.maxChars)
	if err != nil {
		return page, diagnose.New(diagnose.InternalError, diagnose.ConfidenceHigh, "The HTML could not be parsed: %v.", err).At(diagnose.StagePageCheck)
	}
	ai := &AIPage{Clean: clean, Model: e.client.Model(), PromptVersion: aicrawler.PromptVersion, FetchedWith: fetchedWith}
	ai.Evidence = diagnose.AIEvidence{Model: ai.Model, InputChars: utf8.RuneCountInString(clean.Text)}
	page.AI = ai

	ctx = aicrawler.WithCall(ctx, aicrawler.PurposeExtract, target.Hostname())
	key := cacheKey(ai.Model, final.String(), clean.Text)
	if c, ok := e.cache.Get(key); ok {
		e.client.CacheHit(ctx)
		ai.Product, ai.Cached = c.Product, true
		ai.Evidence.InputTokens, ai.Evidence.OutputTokens, ai.Evidence.ProviderRequestID = c.InputTokens, c.OutputTokens, c.RequestID
		return page, nil
	}

	msgs := []aicrawler.Message{
		{Role: "system", Content: aicrawler.SystemPrompt},
		{Role: "user", Content: aicrawler.UserPrompt(final.String(), clean.Text)},
	}
	prod, fail := e.ask(ctx, msgs, ai)
	if fail != nil {
		return page, fail
	}
	ai.Product = prod
	e.cache.Put(key, aicrawler.Cached{Product: prod, InputTokens: ai.Evidence.InputTokens, OutputTokens: ai.Evidence.OutputTokens, RequestID: ai.Evidence.ProviderRequestID})
	return page, nil
}

// ask calls the model and parses its answer. An answer that is not valid
// JSON is asked for once more.
func (e *AIEngine) ask(ctx context.Context, msgs []aicrawler.Message, ai *AIPage) (*aicrawler.Product, *diagnose.Failure) {
	for attempt := 0; ; attempt++ {
		comp, cerr := e.client.Complete(ctx, msgs, "product", e.schema)
		if comp != nil {
			ai.Evidence.ProviderRequestID = comp.RequestID
			ai.Evidence.HTTPStatus = comp.Status
			ai.Evidence.FinishReason = comp.FinishReason
			ai.Evidence.InputTokens += comp.InputTokens
			ai.Evidence.OutputTokens += comp.OutputTokens
			ai.Evidence.Output = truncate(comp.Content, 500)
		}
		if cerr != nil {
			ai.Evidence.HTTPStatus, ai.Evidence.ProviderCode = cerr.Status, cerr.ProviderCode
			if cerr.RequestID != "" {
				ai.Evidence.ProviderRequestID = cerr.RequestID
			}
			return nil, diagnose.New(cerr.Code, diagnose.ConfidenceHigh, "Model %s: %s.", ai.Model, cerr.Message)
		}
		if comp.Refusal != "" {
			return nil, diagnose.New(diagnose.AIBadOutput, diagnose.ConfidenceHigh,
				"Model %s refused to answer: %s.", ai.Model, truncate(comp.Refusal, 200))
		}
		if comp.FinishReason != "" && comp.FinishReason != "stop" {
			return nil, diagnose.New(diagnose.AIBadOutput, diagnose.ConfidenceHigh,
				"Model %s stopped with finish_reason %q before finishing its answer.", ai.Model, comp.FinishReason).
				WithAction("Raise APP_AI_CRAWLER_MAX_OUTPUT_TOKENS if finish_reason is \"length\"; otherwise retry.")
		}
		prod, err := aicrawler.ParseProduct(comp.Content)
		if err == nil {
			return prod, nil
		}
		if attempt > 0 || ctx.Err() != nil {
			return nil, diagnose.New(diagnose.AIBadOutput, diagnose.ConfidenceHigh,
				"Model %s did not return valid product JSON after one repair request: %v.", ai.Model, err)
		}
		msgs = append(msgs,
			aicrawler.Message{Role: "assistant", Content: comp.Content},
			aicrawler.Message{Role: "user", Content: aicrawler.RepairPrompt})
		ctx = aicrawler.WithCall(ctx, aicrawler.PurposeRepair, aicrawler.CallHost(ctx))
	}
}

// checkPage runs the page checks the selector pipeline runs: a challenge
// page, an empty page or a JavaScript shell is not sent to the model.
func checkPage(p *Page) *diagnose.Failure {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(p.Body))
	if err != nil {
		return diagnose.New(diagnose.InternalError, diagnose.ConfidenceHigh, "The HTML could not be parsed: %v.", err).At(diagnose.StagePageCheck)
	}
	s := diagnose.Signals(doc, p.Body, p.Headers)
	if f := diagnose.CheckBlocked(s, p.Status); f != nil {
		return f
	}
	if f := diagnose.ClassifyMissingProduct(s, p.Status); f != nil {
		if f.Code == diagnose.JSRenderedPage {
			f.WithAction("Enable the browser engine on this instance (APP_BROWSER_ENABLED=true): the AI crawler then renders JavaScript pages in Chrome.")
		}
		return f
	}
	return nil
}

func cacheKey(model, pageURL, text string) string {
	h := sha256.New()
	for _, s := range []string{model, aicrawler.PromptVersion, pageURL, text} {
		h.Write([]byte(s))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

var _ Engine = (*AIEngine)(nil)
