package diagnose

import (
	"net/http"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
)

func TestSplitSelector(t *testing.T) {
	cases := map[string][][]string{
		"div.a div.b > p.c":        {{"div.a", "div.a div.b", "div.a div.b > p.c"}},
		"a b, c":                   {{"a", "a b"}, {"c"}},
		`div[data-x="a b"] span`:   {{`div[data-x="a b"]`, `div[data-x="a b"] span`}},
		"button:not(.a .b) i":      {{"button:not(.a .b)", "button:not(.a .b) i"}},
		"div.product.type-product": {{"div.product.type-product"}},
		"  h1 ~ p + span ":         {{"h1", "h1 ~ p", "h1 ~ p + span"}},
	}
	for in, want := range cases {
		if got := SplitSelector(in); !reflect.DeepEqual(got, want) {
			t.Errorf("SplitSelector(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTracePicksDeepestAlternative(t *testing.T) {
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(`<div class="a"><div class="b"><p>x</p></div></div>`))
	steps, deepest := Trace(doc.Selection, "section.z p, div.a div.b span")
	if len(steps) != 3 || steps[1].Matches != 1 || steps[2].Matches != 0 {
		t.Fatalf("steps = %+v", steps)
	}
	if !deepest.HasClass("b") {
		t.Fatalf("deepest should be div.b")
	}
}

func TestNewSampleTruncatesOnRuneBoundary(t *testing.T) {
	s := NewSample(ReasonBodyStart, strings.Repeat("ب", MaxSampleBytes))
	if !s.Truncated || len(s.Content) > MaxSampleBytes || !utf8.ValidString(s.Content) || s.OriginalLength != 2*MaxSampleBytes {
		t.Fatalf("sample: truncated=%v len=%d valid=%v", s.Truncated, len(s.Content), utf8.ValidString(s.Content))
	}
	if NewSample("x", "short").Truncated {
		t.Fatal("short sample must not be truncated")
	}
}

func TestFilterHeaders(t *testing.T) {
	h := http.Header{"Set-Cookie": {"a=b"}, "Server": {"nginx"}, "Cf-Ray": {"1"}, "X-Secret": {"s"}}
	got := FilterHeaders(h)
	if !reflect.DeepEqual(got, map[string]string{"server": "nginx", "cf-ray": "1"}) {
		t.Fatalf("got %v", got)
	}
}

func signals(t *testing.T, html string, h http.Header) PageSignals {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	return Signals(doc, []byte(html), h)
}

func TestClassifyMissingProductOrder(t *testing.T) {
	long := strings.Repeat("real content ", 100)
	cases := []struct {
		name, html string
		h          http.Header
		want       string
	}{
		{"header beats everything", "<p>" + long + "</p>", http.Header{"Cf-Mitigated": {"challenge"}}, BotProtection},
		{"captcha markup", `<div class="g-recaptcha"></div>`, nil, BotProtection},
		{"empty", "", nil, EmptyPage},
		{"almost no text", "<p>hi</p>", nil, EmptyPage},
		{"app shell", `<div id="__next"></div><script>` + strings.Repeat("x", 6000) + `</script>`, nil, JSRenderedPage},
		{"layout change", "<article>" + long + "</article><script>" + strings.Repeat("x", 9000) + "</script>", nil, ""},
	}
	for _, c := range cases {
		f := ClassifyMissingProduct(signals(t, c.html, c.h), 200)
		got := ""
		if f != nil {
			got = f.Code
		}
		if got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestCodeTable(t *testing.T) {
	for _, c := range Codes() {
		f := New(c, ConfidenceHigh, "x")
		if f.Owner == "" || f.Message == "" || f.SuggestedAction == "" {
			t.Errorf("%s: incomplete table entry %+v", c, f)
		}
		if f.Stage == "" && c != DeadlineExceeded && c != InternalError {
			t.Errorf("%s: missing stage", c)
		}
	}
	if New(DeadlineExceeded, ConfidenceHigh, "x").At(StageFetch).Stage != StageFetch {
		t.Error("At should set the stage of stage-less codes")
	}
}
