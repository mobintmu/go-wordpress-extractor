package extract

import (
	"reflect"
	"strings"
	"testing"

	"go-wordpress/internal/siteconfig"

	"github.com/PuerkitoBio/goquery"
)

func TestReadSpecs(t *testing.T) {
	page := `<main>
	<div class="specs"><h3> Key Specs </h3><ul>
		<li><p class="k">Finish</p><p class="v">Honed</p></li>
		<li><p class="k">Item Size</p><p class="v">23.62&quot; x 23.62&quot;</p></li>
	</ul></div>
	<div class="specs"><h3>Details</h3><ul>
		<li><p class="k">Outdoor Use</p><p class="v">Floor | Wall</p></li>
		<li><p class="k">Recommended Grout</p><p class="v"> <a href="/g">Laticrete  Grout- 8lb</a> </p></li>
		<li><p class="k">Empty</p><p class="v"> </p></li>
		<li><p class="v">no name</p></li>
	</ul></div>
	<table class="flat"><tr><th>Weight</th><td>24.4 lbs</td></tr></table>
	</main>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	root := doc.Find("main")

	got := readSpecs(root, siteconfig.Specs{Group: "div.specs", Title: "h3", Row: "li", Name: ".k", Value: ".v"})
	want := []Spec{
		{"Key Specs", "Finish", "Honed"},
		{"Key Specs", "Item Size", `23.62" x 23.62"`},
		{"Details", "Outdoor Use", "Floor | Wall"},
		{"Details", "Recommended Grout", "Laticrete Grout- 8lb"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("grouped:\n got %+v\nwant %+v", got, want)
	}

	got = readSpecs(root, siteconfig.Specs{Row: "table.flat tr", Name: "th", Value: "td"})
	if want := []Spec{{"", "Weight", "24.4 lbs"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("ungrouped: got %+v, want %+v", got, want)
	}

	if got := readSpecs(root, siteconfig.Specs{Row: "dl.none", Name: "dt", Value: "dd"}); got != nil {
		t.Errorf("missing block: got %+v, want nil", got)
	}
}

func TestReadSpecsExclude(t *testing.T) {
	page := `<main><div class="g"><h3>Details <i class="tip">group help</i></h3>
		<p><span class="n">Look <span class="tip"><b>Choose from</b> many looks.</span></span><span class="v">Handmade</span></p>
		<p><span class="n">Edge</span><span class="v">Pressed <span class="tip">help</span></span></p>
		<p><span class="n"><span class="tip">only help</span></span><span class="v">dropped</span></p>
	</div></main>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	sp := siteconfig.Specs{Group: "div.g", Title: "h3", Row: "p", Name: ".n", Value: ".v", Exclude: ".tip"}
	got := readSpecs(doc.Find("main"), sp)
	want := []Spec{{"Details", "Look", "Handmade"}, {"Details", "Edge", "Pressed"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("exclude:\n got %+v\nwant %+v", got, want)
	}

	sp.Exclude = ""
	if got := readSpecs(doc.Find("main"), sp); len(got) != 3 || got[0].Name != "Look Choose from many looks." {
		t.Errorf("without exclude: got %+v", got)
	}
}
