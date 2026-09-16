package cdxp

import (
	"path/filepath"
	"strings"
)

// launchSpec is the fully resolved command line for one profile.
type launchSpec struct {
	ProfileName  string
	File         string
	Args         []string
	EnvPairs     []string
	EnvKey       string
	Model        string
	Base         string
	Wire         string
	ProviderID   string
	ProviderName string
}

// modelKeys are the model_* options forwarded verbatim as -c overrides.
var modelKeys = []string{
	"model_reasoning_effort",
	"model_reasoning_summary",
	"model_verbosity",
	"model_context_window",
	"model_max_output_tokens",
	"review_model",
}

// BuildCodexArgs translates a profile into the `-c` overrides, images, launch
// flags and default codex arguments.
func (a *App) BuildCodexArgs(p *Profile) (*launchSpec, error) {
	spec := &launchSpec{
		ProfileName: p.Name,
		File:        p.Path,
	}

	model := p.Str("model")
	if model == "" {
		return nil, dief("profile 缺少 \"model\" 字段：%s", p.Path)
	}
	spec.Model = model

	pid := p.Str("provider.id")
	if pid == "" {
		pid = strings.TrimSuffix(filepath.Base(p.Path), ".json")
	}
	if !validProviderID(pid) {
		return nil, dief("provider.id 只能包含字母数字、下划线、连字符：%s", pid)
	}
	spec.ProviderID = pid

	name := p.StrOr("provider.name", pid)
	spec.ProviderName = name

	base := p.Str("provider.base_url")
	if base == "" {
		return nil, dief("profile 缺少 \"provider.base_url\"：%s", p.Path)
	}
	base = strings.TrimSuffix(base, "/")
	spec.Base = base

	wire := p.StrOr("provider.wire_api", "responses")
	if wire != "chat" && wire != "responses" {
		return nil, dief("provider.wire_api 只能是 chat 或 responses：%s", wire)
	}
	spec.Wire = wire

	envKey := p.StrOr("provider.env_key", "CODEXP_API_KEY")
	spec.EnvKey = envKey

	prefix := "model_providers." + pid
	args := []string{
		"-c", "model_provider=" + jsonQuote(pid),
		"-c", prefix + ".name=" + jsonQuote(name),
		"-c", prefix + ".base_url=" + jsonQuote(base),
		"-c", prefix + ".wire_api=" + jsonQuote(wire),
		"-c", prefix + ".env_key=" + jsonQuote(envKey),
	}
	if p.TypeOf("provider.requires_openai_auth") == "boolean" {
		args = append(args, "-c", prefix+".requires_openai_auth="+p.Str("provider.requires_openai_auth"))
	}

	extra := p.ObjectAt("provider.extra")
	for _, k := range extra.Keys() {
		v, ok := tomlValue(extra.At(k))
		if !ok {
			continue
		}
		args = append(args, "-c", prefix+"."+k+"="+v)
	}

	for _, k := range modelKeys {
		if p.TypeOf(k) == "absent" {
			continue
		}
		v, ok := tomlValue(valueOf(p, k))
		if !ok {
			continue
		}
		args = append(args, "-c", k+"="+v)
	}

	args = append(args, "-c", "model="+jsonQuote(model))

	for _, img := range p.StringSliceAt("images") {
		path := expandTilde(img, a.Home)
		if isFile(path) {
			args = append(args, "--image="+path)
		} else {
			a.warn("images 里的文件不存在，已跳过：%s", path)
		}
	}

	config := p.ObjectAt("extra_config")
	for _, k := range config.Keys() {
		v, ok := tomlValue(config.At(k))
		if !ok {
			continue
		}
		args = append(args, "-c", k+"="+v)
	}

	if sb := p.Str("launch.sandbox"); sb != "" {
		args = append(args, "-s", sb)
	}
	if ap := p.Str("launch.approval"); ap != "" {
		args = append(args, "-a", ap)
	}
	if cd := p.Str("launch.cd"); cd != "" {
		args = append(args, "-C", cd)
	}

	args = append(args, p.StringSliceAt("codex_args")...)

	spec.Args = args
	return spec, nil
}

// BuildEnvPairs collects the environment a profile injects: its own env map
// first, then the resolved provider key so the key always wins.
func (a *App) BuildEnvPairs(p *Profile, spec *launchSpec, key string) []string {
	var pairs []string
	env := p.ObjectAt("env")
	for _, k := range env.Keys() {
		pairs = append(pairs, k+"="+scalarString(env.At(k)))
	}
	if spec.EnvKey != "" && key != "" {
		pairs = append(pairs, spec.EnvKey+"="+key)
	}
	return pairs
}

// PrintCommand renders the pending command for --dry-run, masking the key.
func (a *App) PrintCommand(spec *launchSpec, key string) {
	c := a.pal
	var out strings.Builder
	for _, arg := range spec.Args {
		out.WriteString(" " + shellQuote(arg))
	}
	a.printf("%sCODEX_PROFILE=%s%s", c.Dim, spec.ProfileName, c.Rst)
	for _, pair := range spec.EnvPairs {
		if spec.EnvKey != "" && strings.HasPrefix(pair, spec.EnvKey+"=") {
			a.printf(" %s=%s", spec.EnvKey, maskKey(key))
			continue
		}
		a.printf(" %s", shellQuote(pair))
	}
	a.printf(" \\\n  codex%s\n", out.String())
}

func valueOf(p *Profile, path string) any {
	v, _ := p.Get(path)
	return v
}

func validProviderID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

// expandTilde resolves the leading ~ the shell would have expanded.
func expandTilde(path, home string) string {
	switch {
	case path == "~":
		return home
	case strings.HasPrefix(path, "~/"):
		return filepath.Join(home, path[2:])
	}
	return path
}
