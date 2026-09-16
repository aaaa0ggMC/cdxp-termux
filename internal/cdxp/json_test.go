package cdxp

import (
	"strings"
	"testing"
)

func TestParseJSONKeepsOrderAndNumberSpelling(t *testing.T) {
	src := `{"z":1,"a":{"y":[1,2],"x":null},"m":1.50,"flag":true}`
	v, err := parseJSON(strings.NewReader(src))
	if err != nil {
		t.Fatalf("parseJSON: %v", err)
	}
	obj, ok := v.(*object)
	if !ok {
		t.Fatalf("got %T, want *object", v)
	}
	if got, want := strings.Join(obj.Keys(), ","), "z,a,m,flag"; got != want {
		t.Errorf("key order = %q, want %q", got, want)
	}
	if got, want := encodeJSON(obj), src; got != want {
		t.Errorf("round trip = %q, want %q", got, want)
	}
	inner, ok := obj.At("a").(*object)
	if !ok {
		t.Fatalf("nested value is %T, want *object", obj.At("a"))
	}
	if got, want := strings.Join(inner.Keys(), ","), "y,x"; got != want {
		t.Errorf("nested key order = %q, want %q", got, want)
	}
}

func TestParseJSONRejectsTrailingData(t *testing.T) {
	if _, err := parseJSON(strings.NewReader(`{"a":1} {"b":2}`)); err == nil {
		t.Fatal("expected an error for trailing data")
	}
}

// The expected literals come from the jq `toml` helper in the shell version.
func TestTomlValue(t *testing.T) {
	v, err := parseJSON(strings.NewReader(`{
		"str": "KS Proxy",
		"num": 120000,
		"float": 1.5,
		"yes": true,
		"nul": null,
		"arr": [1, "two"],
		"table": {"http_headers": {"X-Test": "cdxp"}, "retries": 3},
		"odd key": 1
	}`))
	if err != nil {
		t.Fatalf("parseJSON: %v", err)
	}
	obj := v.(*object)

	cases := []struct {
		key  string
		want string
		ok   bool
	}{
		{"str", `"KS Proxy"`, true},
		{"num", "120000", true},
		{"float", "1.5", true},
		{"yes", "true", true},
		{"nul", "", false},
		{"arr", `[1, "two"]`, true},
		{"table", `{http_headers = {X-Test = "cdxp"}, retries = 3}`, true},
		{"odd key", "1", true},
	}
	for _, tc := range cases {
		got, ok := tomlValue(obj.At(tc.key))
		if ok != tc.ok || got != tc.want {
			t.Errorf("tomlValue(%s) = %q, %v; want %q, %v", tc.key, got, ok, tc.want, tc.ok)
		}
	}
}
