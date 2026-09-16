package cdxp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Profile is one parsed profile file plus its on-disk origin.
type Profile struct {
	// Name is what the user types: the file base name without .json.
	Name string
	// Path is the profile file the launcher will hand to codex.
	Path string

	raw *object
}

// namedProfile pairs a profile name with the file it resolved to.
type namedProfile struct {
	Name string
	Path string
}

// LoadProfile reads and parses a profile file. Numbers keep their original
// spelling and object keys their original order, so TOML rendering stays byte
// for byte what the user wrote.
func LoadProfile(path string) (*Profile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	v, err := parseJSON(f)
	if err != nil {
		return nil, err
	}
	raw, ok := v.(*object)
	if !ok {
		return nil, errors.New("profile must be a JSON object")
	}
	name := strings.TrimSuffix(filepath.Base(path), ".json")
	return &Profile{Name: name, Path: path, raw: raw}, nil
}

// Get walks a dot separated path such as provider.base_url.
func (p *Profile) Get(path string) (any, bool) {
	var cur any = p.raw
	for _, seg := range strings.Split(path, ".") {
		obj, ok := cur.(*object)
		if !ok {
			return nil, false
		}
		cur, ok = obj.Get(seg)
		if !ok || cur == nil {
			return nil, false
		}
	}
	return cur, true
}

// Str renders a scalar profile value the way `jq -r` did; missing keys and null
// become the empty string.
func (p *Profile) Str(path string) string {
	v, ok := p.Get(path)
	if !ok {
		return ""
	}
	return scalarString(v)
}

// StrOr is Str with a default for absent values.
func (p *Profile) StrOr(path, fallback string) string {
	if v := p.Str(path); v != "" {
		return v
	}
	return fallback
}

// TypeOf reports the JSON type at key, or "absent" when the key is missing.
func (p *Profile) TypeOf(key string) string {
	v, ok := p.Get(key)
	if !ok {
		return "absent"
	}
	switch v.(type) {
	case string:
		return "string"
	case json.Number:
		return "number"
	case bool:
		return "boolean"
	case []any:
		return "array"
	case *object:
		return "object"
	}
	return "absent"
}

// ObjectAt returns the JSON object at path, or nil when it is absent.
func (p *Profile) ObjectAt(path string) *object {
	v, ok := p.Get(path)
	if !ok {
		return nil
	}
	m, _ := v.(*object)
	return m
}

// StringSliceAt returns the string elements of the array at path, dropping
// everything else — the shell version's `select(type == "string")`.
func (p *Profile) StringSliceAt(path string) []string {
	v, ok := p.Get(path)
	if !ok {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// StringsAt returns every element of the array at path rendered as text, which
// is what `map(tostring)` did for tags.
func (p *Profile) StringsAt(path string) []string {
	v, ok := p.Get(path)
	if !ok {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		out = append(out, scalarString(e))
	}
	return out
}

// EnvValue reads a profile "env" entry, which doubles as a fallback source for
// the API key so a profile can be self contained.
func (p *Profile) EnvValue(name string) string {
	v, ok := p.ObjectAt("env").Get(name)
	if !ok {
		return ""
	}
	return scalarString(v)
}

func scalarString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	case bool:
		return strconv.FormatBool(t)
	default:
		return encodeJSON(v)
	}
}

// jsonQuote renders s as a JSON string literal, matching jq's tojson.
func jsonQuote(s string) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return `""`
	}
	return strings.TrimRight(b.String(), "\n")
}

var tomlBareKey = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// tomlValue renders a JSON value as a TOML literal: strings are JSON quoted,
// arrays and inline tables nest, and null reports ok == false so callers can
// skip it.
func tomlValue(v any) (string, bool) {
	switch t := v.(type) {
	case nil:
		return "", false
	case string:
		return jsonQuote(t), true
	case json.Number:
		return t.String(), true
	case bool:
		return strconv.FormatBool(t), true
	case []any:
		parts := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := tomlValue(e); ok {
				parts = append(parts, s)
			}
		}
		return "[" + strings.Join(parts, ", ") + "]", true
	case *object:
		parts := make([]string, 0, t.Len())
		for _, k := range t.Keys() {
			s, ok := tomlValue(t.At(k))
			if !ok {
				continue
			}
			parts = append(parts, tomlKey(k)+" = "+s)
		}
		return "{" + strings.Join(parts, ", ") + "}", true
	}
	return "", false
}

func tomlKey(k string) string {
	if tomlBareKey.MatchString(k) {
		return k
	}
	return jsonQuote(k)
}

// ------------------------------------------------------------------ 目录

func (a *App) codexHome() string {
	if v := a.Getenv("CODEX_HOME"); v != "" {
		return v
	}
	return filepath.Join(a.Home, ".codex")
}

// DefaultProfilesDir is the fallback profile directory, used by help output and
// whenever no profile directory exists yet.
func (a *App) DefaultProfilesDir() string {
	return filepath.Join(a.codexHome(), "codex-profiles")
}

// ProfilesDir is where profiles are written: the first directory that exists,
// falling back to the default.
func (a *App) ProfilesDir() string {
	if dirs := a.ProfileDirs(); len(dirs) > 0 {
		return dirs[0]
	}
	return a.DefaultProfilesDir()
}

// ProfileDirs lists the profile directories that exist, in search order.
func (a *App) ProfileDirs() []string {
	var out []string
	seen := map[string]bool{}
	candidates := []string{
		a.Getenv("CDXP_PROFILES_DIR"),
		a.DefaultProfilesDir(),
		filepath.Join(a.Home, ".codex", "codex-profiles"),
		filepath.Join(a.Home, "codex-profiles"),
	}
	for _, d := range candidates {
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			out = append(out, d)
		}
	}
	return out
}

// CollectProfiles returns name/path pairs, with earlier directories winning.
func (a *App) CollectProfiles() []namedProfile {
	var out []namedProfile
	seen := map[string]bool{}
	for _, d := range a.ProfileDirs() {
		ents, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range ents {
			name := e.Name()
			if e.IsDir() || strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".json") {
				continue
			}
			base := strings.TrimSuffix(name, ".json")
			if seen[base] {
				continue
			}
			seen[base] = true
			out = append(out, namedProfile{Name: base, Path: filepath.Join(d, name)})
		}
	}
	return out
}

// KnownProfiles lists profile names in discovery order.
func (a *App) KnownProfiles() []string {
	profiles := a.CollectProfiles()
	out := make([]string, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, p.Name)
	}
	return out
}

// ResolveProfile accepts a bare name, a path, or a *.json path.
func (a *App) ResolveProfile(name string) (string, error) {
	if name == "" {
		return "", errors.New("empty profile name")
	}
	if strings.ContainsRune(name, '/') || strings.HasSuffix(name, ".json") {
		if isFile(name) {
			return name, nil
		}
		return "", fmt.Errorf("no profile file at %s", name)
	}
	for _, d := range a.ProfileDirs() {
		for _, cand := range []string{filepath.Join(d, name+".json"), filepath.Join(d, name)} {
			if isFile(cand) {
				return cand, nil
			}
		}
	}
	return "", fmt.Errorf("profile not found: %s", name)
}

// SuggestNames lists profile names containing needle.
func (a *App) SuggestNames(needle string) string {
	var b strings.Builder
	for _, p := range a.CollectProfiles() {
		if strings.Contains(p.Name, needle) {
			fmt.Fprintf(&b, "  %s\n", p.Name)
		}
	}
	return b.String()
}

func isFile(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular()
}

// ------------------------------------------------------------------ API key

// APIKeyEnvName is the environment variable the profile points at, if any.
func (p *Profile) APIKeyEnvName() string {
	return firstNonEmpty(p.Str("auth.api_key_env"), p.Str("api_key_env"))
}

func (p *Profile) inlineKey() string {
	return firstNonEmpty(p.Str("auth.api_key"), p.Str("api_key"))
}

func (p *Profile) keyCmd() string {
	return firstNonEmpty(p.Str("auth.api_key_cmd"), p.Str("api_key_cmd"))
}

// envOrProfile reads the live environment first, then the profile's own env map.
func (a *App) envOrProfile(p *Profile, name string) string {
	if v := a.Getenv(name); v != "" {
		return v
	}
	return p.EnvValue(name)
}

// ResolveAPIKey applies the documented precedence:
// --key override > api_key_env > api_key > api_key_cmd.
func (a *App) ResolveAPIKey(p *Profile, override string) string {
	if override != "" {
		return override
	}
	if en := p.APIKeyEnvName(); en != "" {
		if v := a.envOrProfile(p, en); v != "" {
			return v
		}
	}
	if v := p.inlineKey(); v != "" {
		return v
	}
	if cmd := p.keyCmd(); cmd != "" {
		if out, err := runShell(cmd); err == nil {
			if line, _, _ := strings.Cut(out, "\n"); strings.TrimSpace(line) != "" {
				return line
			}
		}
	}
	return ""
}

// KeyStatusShort is the cheap key probe used by ls and doctor; it never runs
// api_key_cmd.
func (a *App) KeyStatusShort(p *Profile) string {
	en := p.APIKeyEnvName()
	inline := p.inlineKey()
	cmd := p.keyCmd()
	switch {
	case en != "" && a.envOrProfile(p, en) != "":
		return "✓ env " + en
	case en != "" && inline != "":
		return fmt.Sprintf("✓ inline（env %s 未设置）", en)
	case en != "" && cmd != "":
		return fmt.Sprintf("? %s → cmd", en)
	case en != "":
		return fmt.Sprintf("✗ 缺 key（export %s=…）", en)
	case inline != "":
		return "✓ inline"
	case cmd != "":
		return "? cmd"
	}
	return "✗ 缺 key"
}

// runShell runs a profile's api_key_cmd through the shell, as the original did.
func runShell(cmd string) (string, error) {
	out, err := exec.Command("bash", "-c", cmd).Output()
	return string(out), err
}
