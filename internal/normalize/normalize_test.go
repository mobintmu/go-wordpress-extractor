package normalize

import (
	"net/url"
	"testing"
)

func TestLowestPrice(t *testing.T) {
	cases := []struct {
		text, unit string
		want       int64 // -1 for nil
		err        bool
	}{
		{"499,000 تومان", "toman", 499000, false},
		{"۴۹۹,۰۰۰ تومان", "toman", 499000, false},
		{"٤٩٩٬٠٠٠ تومان", "toman", 499000, false},
		{"۴۵۰٬۰۰۰ تومان", "toman", 450000, false},
		{"100,000 – 200,000 تومان", "toman", 100000, false},
		{"200,000 تومان 150,000 تومان", "toman", 150000, false},
		{"5,000,000 ریال", "rial", 500000, false},
		{"1.250.000", "toman", 1250000, false},
		{"0 تومان", "toman", -1, false},
		{"تماس بگیرید", "toman", -1, true},
		{"99999999999999999999", "toman", -1, true},
	}
	for _, c := range cases {
		got, err := LowestPrice(c.text, c.unit)
		if (err != nil) != c.err {
			t.Errorf("%q: err = %v", c.text, err)
			continue
		}
		if c.want == -1 {
			if got != nil {
				t.Errorf("%q: want nil, got %d", c.text, *got)
			}
			continue
		}
		if got == nil || *got != c.want {
			t.Errorf("%q: got %v, want %d", c.text, got, c.want)
		}
	}
}

func TestText(t *testing.T) {
	for in, want := range map[string]string{
		"  <b>Hello</b>\n\t world ": "Hello world",
		"a b":                       "a b",
		"‏بادام​":                   "بادام",
		"می‌خواهم":                  "می‌خواهم",
	} {
		if got := Text(in); got != want {
			t.Errorf("Text(%q) = %q, want %q", in, got, want)
		}
	}
	if got := Paragraphs([]string{" a ", "", "  ", "b"}, "\n"); got != "a\nb" {
		t.Errorf("Paragraphs = %q", got)
	}
	if !ContainsAny("لطفا  تماس   بگیرید", []string{"تماس بگیرید"}) || ContainsAny("x", []string{""}) {
		t.Error("ContainsAny")
	}
}

func TestResolveURL(t *testing.T) {
	base, _ := url.Parse("https://shop.test/product/a/")
	for in, want := range map[string]string{
		"/img/a.jpg":               "https://shop.test/img/a.jpg",
		"b.jpg":                    "https://shop.test/product/a/b.jpg",
		"//cdn.test/c.jpg":         "https://cdn.test/c.jpg",
		"data:image/gif;base64,R0": "",
		"javascript:alert(1)":      "",
		"https://x.test/a.jpg 300w, https://x.test/b.jpg 600w": "https://x.test/a.jpg",
		"": "",
	} {
		if got := ResolveURL(base, in); got != want {
			t.Errorf("ResolveURL(%q) = %q, want %q", in, got, want)
		}
	}
}
