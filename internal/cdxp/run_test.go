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

// testApp returns an App whose effects are captured instead of performed.
func testApp(t *testing.T, stdin string) (*App, *bytes.Buffer, *bytes.Buffer, *execCall) {
	t.Helper()
	out, errb := &bytes.Buffer{}, &bytes.Buffer{}
	call := &execCall{}
	return &App{
		Stdout:   out,
		Stderr:   errb,
		Stdin:    strings.NewReader(stdin),
		Getenv:   os.Getenv,
		Home:     t.TempDir(),
		pal:      palette{},
		Exec:     call.run,
		LookPath: func(string) (string, error) { return "/usr/bin/codex", nil },
		HTTP:     &http.Client{Timeout: time.Second},
	}, out, errb, call
}

type execCall struct {
	argv0 string
	argv  []string
	env   []string
	calls int
}

func (c *execCall) run(argv0 string, argv, env []string) error {
	c.argv0, c.argv, c.env, c.calls = argv0, argv, env, c.calls+1
	return nil
}

func (c *execCall) value(key string) string {
	for _, kv := range c.env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			return v
		}
	}
	return ""
}

func TestRunListsProfiles(t *testing.T) {
	a, out, _, _ := testApp(t, "")
	dir := filepath.Join(t.TempDir(), "p")
	writeProfile(t, dir, "demo", `{"model":"m","provider":{"base_url":"https://x.example/v1"}}`)
	t.Setenv("CDXP_PROFILES_DIR", dir)
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), "ch"))

	if code := a.Run(nil); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got := out.String(); !strings.Contains(got, "demo") || !strings.Contains(got, "Codex profiles (1)") {
		t.Errorf("unexpected listing:\n%s", got)
	}
}

func TestRunLsNamesOnly(t *testing.T) {
	a, out, _, _ := testApp(t, "")
	dir := filepath.Join(t.TempDir(), "p")
	writeProfile(t, dir, "b", `{"model":"m"}`)
	writeProfile(t, dir, "a", `{"model":"m"}`)
	t.Setenv("CDXP_PROFILES_DIR", dir)

	if code := a.Run([]string{"ls", "-1"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got, want := out.String(), "a\nb\n"; got != want {
		t.Errorf("ls -1 = %q, want %q", got, want)
	}
}

func TestRunUnknownProfile(t *testing.T) {
	a, _, errb, call := testApp(t, "")
	dir := filepath.Join(t.TempDir(), "p")
	writeProfile(t, dir, "deepseek", `{"model":"m"}`)
	t.Setenv("CDXP_PROFILES_DIR", dir)

	if code := a.Run([]string{"deeps"}); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if call.calls != 0 {
		t.Error("codex should not have been started")
	}
	if got := errb.String(); !strings.Contains(got, "找不到 profile：deeps") || !strings.Contains(got, "你是不是想要") {
		t.Errorf("unexpected stderr:\n%s", got)
	}
}

func TestRunDryRunDoesNotExec(t *testing.T) {
	a, out, _, call := testApp(t, "")
	dir := filepath.Join(t.TempDir(), "p")
	writeProfile(t, dir, "demo", `{"model":"m","provider":{"base_url":"https://x.example/v1"}}`)
	t.Setenv("CDXP_PROFILES_DIR", dir)

	if code := a.Run([]string{"demo", "--dry-run"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if call.calls != 0 {
		t.Error("--dry-run must not start codex")
	}
	if got := out.String(); !strings.Contains(got, `codex -c model_provider=\"demo\"`) {
		t.Errorf("unexpected dry run output:\n%s", got)
	}
}

func TestRunLaunchHandsOverToCodex(t *testing.T) {
	a, _, errb, call := testApp(t, "")
	dir := filepath.Join(t.TempDir(), "p")
	writeProfile(t, dir, "demo", `{
		"model": "m",
		"provider": {"base_url": "https://x.example/v1", "env_key": "P_KEY"},
		"auth": {"api_key_env": "P_KEY"},
		"env": {"EXTRA": "1"}
	}`)
	t.Setenv("CDXP_PROFILES_DIR", dir)
	t.Setenv("P_KEY", "sk-test-key")

	if code := a.Run([]string{"demo", "exec", "hi"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if call.calls != 1 {
		t.Fatalf("exec calls = %d, want 1", call.calls)
	}
	if call.argv0 != "/usr/bin/codex" {
		t.Errorf("argv0 = %q", call.argv0)
	}
	if call.argv[0] != "codex" {
		t.Errorf("argv[0] = %q, want codex", call.argv[0])
	}
	if !strings.Contains(strings.Join(call.argv, " "), "exec hi") {
		t.Errorf("extra codex args were not passed through: %v", call.argv)
	}
	if got := call.value("CODEX_PROFILE"); got != "demo" {
		t.Errorf("CODEX_PROFILE = %q, want demo", got)
	}
	if got := call.value("CODEX_PROFILE_FILE"); got != filepath.Join(dir, "demo.json") {
		t.Errorf("CODEX_PROFILE_FILE = %q", got)
	}
	if got := call.value("P_KEY"); got != "sk-test-key" {
		t.Errorf("P_KEY = %q, want the resolved key", got)
	}
	if got := call.value("EXTRA"); got != "1" {
		t.Errorf("EXTRA = %q, want 1", got)
	}
	if errb.Len() != 0 {
		t.Errorf("unexpected stderr: %s", errb.String())
	}
}

func TestRunQuietSuppressesBanner(t *testing.T) {
	a, out, _, _ := testApp(t, "")
	dir := filepath.Join(t.TempDir(), "p")
	writeProfile(t, dir, "demo", `{"model":"m","provider":{"base_url":"https://x.example/v1"}}`)
	t.Setenv("CDXP_PROFILES_DIR", dir)

	if code := a.Run([]string{"demo", "-q", "--dry-run"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if strings.Contains(out.String(), "╭─ Codex profile") {
		t.Errorf("banner should have been suppressed:\n%s", out)
	}
}

func TestRunEnvEmitsExports(t *testing.T) {
	a, out, _, _ := testApp(t, "")
	dir := filepath.Join(t.TempDir(), "p")
	writeProfile(t, dir, "demo", `{
		"model": "m",
		"provider": {"base_url": "https://x.example/v1", "env_key": "P_KEY"},
		"auth": {"api_key_env": "P_KEY"}
	}`)
	t.Setenv("CDXP_PROFILES_DIR", dir)
	t.Setenv("P_KEY", "sk-a b")

	if code := a.Run([]string{"env", "demo"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	want := "export CODEX_PROFILE=demo\nexport CODEX_MODEL=m\nexport CODEX_BASE_URL=https://x.example/v1\nexport P_KEY=sk-a\\ b\n"
	if got := out.String(); got != want {
		t.Errorf("env output = %q, want %q", got, want)
	}
}

func TestRunNewAndRemove(t *testing.T) {
	a, out, _, _ := testApp(t, "y\n")
	dir := filepath.Join(t.TempDir(), "p")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CDXP_PROFILES_DIR", dir)

	if code := a.Run([]string{"new", "demo"}); code != 0 {
		t.Fatalf("new exit = %d, want 0", code)
	}
	path := filepath.Join(dir, "demo.json")
	if _, err := LoadProfile(path); err != nil {
		t.Fatalf("new did not write a valid profile: %v", err)
	}
	if !strings.Contains(out.String(), "已创建") {
		t.Errorf("unexpected new output: %s", out)
	}
	if st, err := os.Stat(path); err != nil || st.Mode().Perm() != 0o600 {
		t.Errorf("profile permissions = %v (err %v), want 0600", st.Mode().Perm(), err)
	}

	if code := a.Run([]string{"new", "demo"}); code != 1 {
		t.Errorf("re-creating an existing profile should fail, exit = %d", code)
	}

	if code := a.Run([]string{"rm", "demo"}); code != 0 {
		t.Fatalf("rm exit = %d, want 0", code)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("rm left %s behind", path)
	}
}

func TestRunRmCancelled(t *testing.T) {
	a, _, _, _ := testApp(t, "n\n")
	dir := filepath.Join(t.TempDir(), "p")
	writeProfile(t, dir, "demo", `{"model":"m"}`)
	t.Setenv("CDXP_PROFILES_DIR", dir)

	if code := a.Run([]string{"rm", "demo"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !isFile(filepath.Join(dir, "demo.json")) {
		t.Error("answering n must keep the profile")
	}
}

func TestRunBrokenProfileFails(t *testing.T) {
	a, _, errb, _ := testApp(t, "")
	dir := filepath.Join(t.TempDir(), "p")
	writeProfile(t, dir, "broken", `{"model": }`)
	t.Setenv("CDXP_PROFILES_DIR", dir)

	if code := a.Run([]string{"broken", "--dry-run"}); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "profile JSON 解析失败") {
		t.Errorf("unexpected stderr: %s", errb.String())
	}
}

func TestRunVersion(t *testing.T) {
	a, out, _, _ := testApp(t, "")
	if code := a.Run([]string{"version"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got, want := out.String(), "cdxp "+Version+"\n"; got != want {
		t.Errorf("version = %q, want %q", got, want)
	}
}
