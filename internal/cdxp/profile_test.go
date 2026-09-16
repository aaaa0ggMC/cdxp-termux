package cdxp

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const richProfile = `{
  "name": "rich",
  "description": "复杂 profile",
  "tags": ["a", 1, true],
  "model": "gpt-5.6-luna",
  "model_reasoning_effort": "low",
  "model_context_window": 200000,
  "provider": {
    "id": "ks",
    "name": "KS Proxy",
    "base_url": "http://127.0.0.1:6799/v1/",
    "wire_api": "responses",
    "env_key": "KS_API_KEY",
    "requires_openai_auth": true,
    "extra": {
      "http_headers": { "X-Test": "cdxp" },
      "request_max_retries": 1,
      "nothing": null
    }
  },
  "auth": { "api_key_env": "KS_API_KEY" },
  "env": { "KS_API_KEY": "env-key" },
  "extra_config": { "tools.web_search": true },
  "codex_args": ["--no-alt-screen"],
  "launch": { "sandbox": "workspace-write", "approval": "never" }
}`

func newTestApp(t *testing.T) (*App, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	out, errb := &bytes.Buffer{}, &bytes.Buffer{}
	return &App{
		Stdout:   out,
		Stderr:   errb,
		Stdin:    strings.NewReader(""),
		Getenv:   os.Getenv,
		Home:     t.TempDir(),
		pal:      palette{},
		Exec:     func(string, []string, []string) error { return nil },
		LookPath: func(string) (string, error) { return "/usr/bin/codex", nil },
		HTTP:     &http.Client{Timeout: time.Second},
	}, out, errb
}

// writeProfile drops a profile file into dir and returns its path.
func writeProfile(t *testing.T, dir, name, body string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	path := filepath.Join(dir, name+".json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestProfilesDirPrefersEnvOverride(t *testing.T) {
	a, _, _ := newTestApp(t)
	custom := filepath.Join(t.TempDir(), "custom")
	if err := os.MkdirAll(custom, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CDXP_PROFILES_DIR", custom)

	if got := a.ProfilesDir(); got != custom {
		t.Errorf("ProfilesDir() = %q, want %q", got, custom)
	}
	if got, want := a.DefaultProfilesDir(), filepath.Join(a.Home, ".codex", "codex-profiles"); got != want {
		t.Errorf("DefaultProfilesDir() = %q, want %q", got, want)
	}
}

func TestProfilesDirFallsBackWhenOverrideMissing(t *testing.T) {
	a, _, _ := newTestApp(t)
	t.Setenv("CDXP_PROFILES_DIR", filepath.Join(t.TempDir(), "does-not-exist"))
	if got, want := a.ProfilesDir(), a.DefaultProfilesDir(); got != want {
		t.Errorf("ProfilesDir() = %q, want the default %q", got, want)
	}
}

func TestCollectProfilesOrderAndOverrides(t *testing.T) {
	a, _, _ := newTestApp(t)
	first := filepath.Join(t.TempDir(), "first")
	// CODEX_HOME points at a second tree, so its codex-profiles directory is
	// consulted after CDXP_PROFILES_DIR.
	second := filepath.Join(t.TempDir(), "second")
	writeProfile(t, first, "b", `{"model":"m"}`)
	writeProfile(t, first, "a", `{"model":"m"}`)
	writeProfile(t, second, "b", `{"model":"shadowed"}`)
	writeProfile(t, filepath.Join(second, "codex-profiles"), "c", `{"model":"m"}`)
	if err := os.WriteFile(filepath.Join(first, ".hidden.json"), []byte(`{"model":"m"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CDXP_PROFILES_DIR", first)
	t.Setenv("CODEX_HOME", second)

	got := a.CollectProfiles()
	var names []string
	for _, p := range got {
		names = append(names, p.Name)
	}
	if want := "a,b,c"; strings.Join(names, ",") != want {
		t.Fatalf("names = %v, want %q", names, want)
	}
	if got[1].Path != filepath.Join(first, "b.json") {
		t.Errorf("b resolved to %q, want the first directory to win", got[1].Path)
	}
}

func TestResolveProfileAcceptsPathAndName(t *testing.T) {
	a, _, _ := newTestApp(t)
	dir := filepath.Join(t.TempDir(), "p")
	path := writeProfile(t, dir, "deepseek", `{"model":"m"}`)
	t.Setenv("CDXP_PROFILES_DIR", dir)

	if got, err := a.ResolveProfile("deepseek"); err != nil || got != path {
		t.Errorf("ResolveProfile(name) = %q, %v; want %q", got, err, path)
	}
	if got, err := a.ResolveProfile(path); err != nil || got != path {
		t.Errorf("ResolveProfile(path) = %q, %v; want %q", got, err, path)
	}
	if _, err := a.ResolveProfile("nope"); err == nil {
		t.Error("ResolveProfile(nope) unexpectedly succeeded")
	}
}

func TestResolveAPIKeyPrecedence(t *testing.T) {
	a, _, _ := newTestApp(t)
	path := writeProfile(t, t.TempDir(), "p", `{
		"model": "m",
		"provider": {"base_url": "https://x.example/v1", "env_key": "P_KEY"},
		"auth": {"api_key_env": "P_KEY", "api_key": "inline-key"}
	}`)
	p, err := LoadProfile(path)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("P_KEY", "")
	if got := a.ResolveAPIKey(p, "cli-key"); got != "cli-key" {
		t.Errorf("override = %q, want cli-key", got)
	}
	t.Setenv("P_KEY", "from-env")
	if got := a.ResolveAPIKey(p, ""); got != "from-env" {
		t.Errorf("env = %q, want from-env", got)
	}
	t.Setenv("P_KEY", "")
	if got := a.ResolveAPIKey(p, ""); got != "inline-key" {
		t.Errorf("inline = %q, want inline-key", got)
	}

	cmdPath := writeProfile(t, t.TempDir(), "q", `{
		"model": "m",
		"provider": {"base_url": "https://x.example/v1"},
		"auth": {"api_key_cmd": "printf 'cmd-key\nsecond-line'"}
	}`)
	q, err := LoadProfile(cmdPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := a.ResolveAPIKey(q, ""); got != "cmd-key" {
		t.Errorf("api_key_cmd = %q, want cmd-key", got)
	}
}

// The profile's own env map is a fallback so a profile can be self contained.
func TestResolveAPIKeyUsesProfileEnv(t *testing.T) {
	a, _, _ := newTestApp(t)
	path := writeProfile(t, t.TempDir(), "p", `{
		"model": "m",
		"provider": {"base_url": "https://x.example/v1"},
		"auth": {"api_key_env": "P_KEY"},
		"env": {"P_KEY": "profile-key"}
	}`)
	p, err := LoadProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("P_KEY", "")
	if got := a.ResolveAPIKey(p, ""); got != "profile-key" {
		t.Errorf("profile env = %q, want profile-key", got)
	}
	if got := a.KeyStatusShort(p); got != "✓ env P_KEY" {
		t.Errorf("KeyStatusShort = %q", got)
	}
}

func TestBuildCodexArgs(t *testing.T) {
	a, _, _ := newTestApp(t)
	p, err := LoadProfile(writeProfile(t, t.TempDir(), "rich", richProfile))
	if err != nil {
		t.Fatal(err)
	}

	spec, err := a.BuildCodexArgs(p)
	if err != nil {
		t.Fatalf("BuildCodexArgs: %v", err)
	}
	want := []string{
		"-c", `model_provider="ks"`,
		"-c", `model_providers.ks.name="KS Proxy"`,
		"-c", `model_providers.ks.base_url="http://127.0.0.1:6799/v1"`,
		"-c", `model_providers.ks.wire_api="responses"`,
		"-c", `model_providers.ks.env_key="KS_API_KEY"`,
		"-c", `model_providers.ks.requires_openai_auth=true`,
		"-c", `model_providers.ks.http_headers={X-Test = "cdxp"}`,
		"-c", `model_providers.ks.request_max_retries=1`,
		"-c", `model_reasoning_effort="low"`,
		"-c", `model_context_window=200000`,
		"-c", `model="gpt-5.6-luna"`,
		"-c", `tools.web_search=true`,
		"-s", "workspace-write",
		"-a", "never",
		"--no-alt-screen",
	}
	if strings.Join(spec.Args, "\n") != strings.Join(want, "\n") {
		t.Errorf("args =\n%s\nwant\n%s", strings.Join(spec.Args, "\n"), strings.Join(want, "\n"))
	}
	if spec.ProviderID != "ks" || spec.ProviderName != "KS Proxy" {
		t.Errorf("provider = %q/%q", spec.ProviderID, spec.ProviderName)
	}
	if spec.Wire != "responses" || spec.EnvKey != "KS_API_KEY" || spec.Model != "gpt-5.6-luna" {
		t.Errorf("spec = %+v", spec)
	}
}

func TestBuildCodexArgsRequiresModelAndBaseURL(t *testing.T) {
	a, _, _ := newTestApp(t)
	for _, body := range []string{
		`{"provider":{"base_url":"https://x.example/v1"}}`,
		`{"model":"m"}`,
		`{"model":"m","provider":{"base_url":"https://x.example/v1","wire_api":"grpc"}}`,
	} {
		p, err := LoadProfile(writeProfile(t, t.TempDir(), "p", body))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := a.BuildCodexArgs(p); err == nil {
			t.Errorf("BuildCodexArgs(%s) unexpectedly succeeded", body)
		}
	}
}

func TestBuildCodexArgsImages(t *testing.T) {
	a, out, errb := newTestApp(t)
	imgPath := filepath.Join(t.TempDir(), "pic.png")
	if err := os.WriteFile(imgPath, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = out
	body := `{"model":"m","provider":{"base_url":"https://x.example/v1"},
	          "images":["` + imgPath + `","~/missing.png"]}`
	p, err := LoadProfile(writeProfile(t, t.TempDir(), "p", body))
	if err != nil {
		t.Fatal(err)
	}
	spec, err := a.BuildCodexArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(spec.Args, " ")
	if !strings.Contains(joined, "--image="+imgPath) {
		t.Errorf("existing image not attached: %s", joined)
	}
	if strings.Contains(joined, "missing.png") {
		t.Errorf("missing image should be skipped: %s", joined)
	}
	if !strings.Contains(errb.String(), "已跳过") {
		t.Errorf("expected a warning about the missing image, got %q", errb.String())
	}
}

func TestBuildEnvPairsKeyWins(t *testing.T) {
	a, _, _ := newTestApp(t)
	p, err := LoadProfile(writeProfile(t, t.TempDir(), "p", `{
		"model": "m",
		"provider": {"base_url": "https://x.example/v1", "env_key": "P_KEY"},
		"env": {"P_KEY": "from-profile", "OTHER": 7}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	spec, err := a.BuildCodexArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	pairs := a.BuildEnvPairs(p, spec, "resolved-key")
	want := []string{"P_KEY=from-profile", "OTHER=7", "P_KEY=resolved-key"}
	if strings.Join(pairs, ",") != strings.Join(want, ",") {
		t.Errorf("pairs = %v, want %v", pairs, want)
	}
}

func TestUnknownFieldsKeepsSourceOrder(t *testing.T) {
	a, _, _ := newTestApp(t)
	p, err := LoadProfile(writeProfile(t, t.TempDir(), "p", `{
		"model": "m",
		"zeta": 1,
		"provider": {"base_url": "https://x.example/v1"},
		"alpha": 2
	}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = a
	if got, want := encodeJSON(p.unknownFields()), `{"zeta":1,"alpha":2}`; got != want {
		t.Errorf("unknownFields = %s, want %s", got, want)
	}
}
