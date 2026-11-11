package apikey

import (
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

const pepper = "unit-pepper-0123456789abcdef-0123456789abcdef"

func TestParse(t *testing.T) {
	g, err := Generate(pepper, "svc", 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Key) != len(Prefix)+IDLength+1+SecretLength {
		t.Fatalf("key length = %d", len(g.Key))
	}
	id, err := Parse(g.Key)
	if err != nil || id != g.Entry.ID {
		t.Fatalf("Parse = %q, %v", id, err)
	}
	for _, bad := range []string{"", "gwp_", "abc_k7f3a9c2_" + strings.Repeat("a", 43), "gwp_K7F3A9C2_" + strings.Repeat("a", 43),
		"gwp_k7f3a9c2-" + strings.Repeat("a", 43), "gwp_k7f3a9c2_" + strings.Repeat("a", 42), "gwp_k7f3a9c2_" + strings.Repeat("+", 43)} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) should fail", bad)
		}
	}
}

func TestSecretUnderscoreDoesNotBreakParsing(t *testing.T) {
	key := Format("abcd1234", "_"+strings.Repeat("x", SecretLength-1))
	if id, err := Parse(key); err != nil || id != "abcd1234" {
		t.Fatalf("Parse = %q %v", id, err)
	}
}

func TestHashEqual(t *testing.T) {
	h := Hash(pepper, "value")
	if !HashEqual(pepper, "value", h) {
		t.Fatal("same value must match")
	}
	if HashEqual(pepper, "other", h) || HashEqual("another-pepper-0123456789abcdef-xx", "value", h) || HashEqual(pepper, "value", "!!!") {
		t.Fatal("mismatch accepted")
	}
}

func TestFileRoundTrip(t *testing.T) {
	now := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	a, _ := Generate(pepper, "catalog-service", 8760*time.Hour, now)
	b, _ := Generate(pepper, "pricing_service", 0, now.Add(time.Second))
	b.Entry.Status = StatusRevoked
	for _, format := range []string{FormatEnv, FormatJSON} {
		data, err := Encode([]Entry{a.Entry, b.Entry}, format)
		if err != nil {
			t.Fatal(err)
		}
		got, err := ParseFile(data, format)
		if err != nil {
			t.Fatalf("%s: %v\n%s", format, err, data)
		}
		if len(got) != 2 || got[0].Hash != a.Entry.Hash || got[1].Status != StatusRevoked || got[1].ExpiresAt != nil || !got[0].ExpiresAt.Equal(*a.Entry.ExpiresAt) {
			t.Fatalf("%s round trip: %+v", format, got)
		}
	}
	env, _ := Encode([]Entry{a.Entry}, FormatEnv)
	want := "APIKEY_" + strings.ToUpper(a.Entry.ID) + "=hmac-sha256,active,2026-11-08T10:00:00Z,2027-11-08T10:00:00Z,catalog-service," + a.Entry.Hash
	if !strings.Contains(string(env), want) {
		t.Fatalf("env line:\n%s\nwant %s", env, want)
	}
}

func TestParseFileRejectsWholeFile(t *testing.T) {
	g, _ := Generate(pepper, "svc", 0, time.Now())
	good, _ := Encode([]Entry{g.Entry}, FormatEnv)
	cases := map[string]string{
		"duplicate id":   string(good) + strings.SplitN(string(good), "\n", 2)[1],
		"lowercase name": "APIKEY_abcd1234=hmac-sha256,active,2026-11-08T10:00:00Z,never,svc," + g.Entry.Hash,
		"other variable": "OTHER=1",
		"bad status":     "APIKEY_ABCD1234=hmac-sha256,paused,2026-11-08T10:00:00Z,never,svc," + g.Entry.Hash,
		"bad hash":       "APIKEY_ABCD1234=hmac-sha256,active,2026-11-08T10:00:00Z,never,svc,short",
		"bad time":       "APIKEY_ABCD1234=hmac-sha256,active,yesterday,never,svc," + g.Entry.Hash,
		"bad name":       "APIKEY_ABCD1234=hmac-sha256,active,2026-11-08T10:00:00Z,never,bad name," + g.Entry.Hash,
		"bad algorithm":  "APIKEY_ABCD1234=md5,active,2026-11-08T10:00:00Z,never,svc," + g.Entry.Hash,
	}
	for name, data := range cases {
		if _, err := ParseFile([]byte(data), FormatEnv); err == nil {
			t.Errorf("%s: should be rejected", name)
		}
	}
	if _, err := ParseFile([]byte(`{"version":1,"keys":[],"extra":1}`), FormatJSON); err == nil {
		t.Error("unknown JSON fields should be rejected")
	}
	if _, err := ParseFile([]byte(`{"version":2,"keys":[]}`), FormatJSON); err == nil {
		t.Error("unknown JSON version should be rejected")
	}
}

func TestStoreVerify(t *testing.T) {
	now := time.Now()
	active, _ := Generate(pepper, "active", time.Hour, now)
	revoked, _ := Generate(pepper, "revoked", 0, now)
	revoked.Entry.Status = StatusRevoked
	expired, _ := Generate(pepper, "expired", time.Minute, now.Add(-time.Hour))
	s := &Store{pepper: pepper, log: zap.NewNop(), now: time.Now, dummy: Hash(pepper, "dummy")}
	snap := &Snapshot{byID: map[string]Entry{}}
	for _, e := range []Entry{active.Entry, revoked.Entry, expired.Entry} {
		snap.byID[e.ID] = e
	}
	s.current.Store(snap)

	if id, ok := s.Verify(active.Key); !ok || id != active.Entry.ID {
		t.Fatal("active key rejected")
	}
	for name, key := range map[string]string{
		"revoked": revoked.Key, "expired": expired.Key, "empty": "", "garbage": "nope",
		"unknown":      Format("zzzzzzzz", strings.Repeat("a", SecretLength)),
		"wrong secret": Format(active.Entry.ID, strings.Repeat("a", SecretLength)),
	} {
		if _, ok := s.Verify(key); ok {
			t.Errorf("%s key accepted", name)
		}
	}
}
