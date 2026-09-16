package cdxp

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"
)

func arg0(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

// emptyProfile stands in for a profile file that failed to parse, so listings
// degrade to "?" instead of aborting.
func emptyProfile(np namedProfile) *Profile {
	return &Profile{Name: np.Name, Path: np.Path, raw: newObject()}
}

func (a *App) loadOrEmpty(np namedProfile) *Profile {
	p, err := LoadProfile(np.Path)
	if err != nil {
		return emptyProfile(np)
	}
	return p
}

// ---------------------------------------------------------------- ls

func (a *App) listProfiles(args []string) error {
	onlyNames := false
scan:
	for _, arg := range args {
		switch arg {
		case "-1", "--names", "--short":
			onlyNames = true
		default:
			break scan
		}
	}

	profiles := a.CollectProfiles()
	c := a.pal

	if len(profiles) == 0 {
		a.printf("还没有任何 profile。\n\n  %scdxp new%s    新建一个\n  目录：%s\n",
			c.Cyn, c.Rst, a.ProfilesDir())
		return nil
	}
	if onlyNames {
		for _, np := range profiles {
			a.printf("%s\n", np.Name)
		}
		return nil
	}

	dirList := strings.Join(a.ProfileDirs(), "、")
	if dirList == "" {
		dirList = a.ProfilesDir()
	}
	a.printf("%s%sCodex profiles%s (%d)  %s%s%s\n", c.Bld, c.Cyn, c.Rst, len(profiles), c.Dim, dirList, c.Rst)

	for _, np := range profiles {
		p := a.loadOrEmpty(np)
		model := p.StrOr("model", "?")
		provider := firstNonEmpty(p.Str("provider.name"), p.Str("provider.id"), "?")
		base := p.Str("provider.base_url")
		wire := p.StrOr("provider.wire_api", "responses")
		desc := p.Str("description")
		tags := strings.Join(p.StringsAt("tags"), ", ")

		a.printf("\n  %s%s● %s%s  %s[%s]%s\n", c.Bld, c.Mag, np.Name, c.Rst, c.Dim, a.KeyStatusShort(p), c.Rst)
		if desc != "" {
			a.printf("      %s%s%s\n", c.Bld, desc, c.Rst)
		}
		a.printf("      %smodel%s %s%s%s    %sprovider%s %s%s%s\n",
			c.Dim, c.Rst+c.Dim, c.Grn, model, c.Rst, c.Dim, c.Rst+c.Dim, c.Rst, provider, c.Rst)
		if base != "" {
			a.printf("      %s%s  wire_api=%s%s\n", c.Dim, base, wire, c.Rst)
		}
		if tags != "" {
			a.printf("      %stags: %s%s\n", c.Dim, tags, c.Rst)
		}
	}
	a.printf("\n  %s用法：cdxp <name> [codex 参数…]    详情：cdxp show <name>%s\n\n", c.Dim, c.Rst)
	return nil
}

// ---------------------------------------------------------------- show

func (a *App) showProfile(args []string) error {
	name := arg0(args)
	if name == "" {
		return dief("用法：cdxp show <name>")
	}
	f, err := a.ResolveProfile(name)
	if err != nil {
		return dief("找不到 profile：%s", name)
	}
	p, err := LoadProfile(f)
	if err != nil {
		return dief("profile JSON 解析失败：%s（用 cdxp check %s 看细节）", f, f)
	}

	c := a.pal
	base := filepath.Base(name)
	a.printf("\n%s%s╭─ %s%s\n", c.Cyn, c.Bld, base, c.Rst)

	meta := []string{"name", "description", "homepage", "author", "tags", "notes"}
	for _, k := range meta {
		var v string
		if k == "tags" {
			v = strings.Join(p.StringsAt(k), ", ")
		} else {
			v = p.Str(k)
		}
		if v != "" {
			a.printf("%s│%s %s %s\n", c.Cyn, c.Rst, padRight(k, 12), v)
		}
	}

	a.printf("\n%s│%s %s\n", c.Cyn, c.Rst, c.Bld+"连接"+c.Rst)
	conn := append([]string{"model"}, modelKeys...)
	for _, k := range conn {
		if v := p.Str(k); v != "" {
			a.printf("%s│%s %s %s\n", c.Cyn, c.Rst, padRight(k, 12), v)
		}
	}
	a.printf("%s│%s %s %s  (%s)\n", c.Cyn, c.Rst, padRight("base_url", 12),
		p.Str("provider.base_url"), firstNonEmpty(p.Str("provider.name"), p.Str("provider.id")))
	a.printf("%s│%s %s %s\n", c.Cyn, c.Rst, padRight("wire_api", 12), p.StrOr("provider.wire_api", "responses"))
	a.printf("%s│%s %s %s\n", c.Cyn, c.Rst, padRight("env_key", 12), p.StrOr("provider.env_key", "CODEXP_API_KEY"))

	a.printf("\n%s│%s %s\n", c.Cyn, c.Rst, c.Bld+"鉴权"+c.Rst)
	a.printf("%s│%s %s %s\n", c.Cyn, c.Rst, padRight("status", 12), a.KeyStatusShort(p))
	for _, k := range []string{"auth.api_key_env", "auth.api_key", "auth.api_key_cmd", "auth.hint"} {
		v := p.Str(k)
		if k == "auth.api_key" && v != "" {
			v = maskKey(v)
		}
		if v != "" {
			a.printf("%s│%s %s %s\n", c.Cyn, c.Rst, padRight(k, 12), v)
		}
	}
	if v := a.ResolveAPIKey(p, ""); v != "" {
		a.printf("%s│%s %s %s\n", c.Cyn, c.Rst, padRight("已解析", 12), maskKey(v))
	}

	if extra := p.ObjectAt("extra_config"); extra.Len() > 0 {
		a.printf("\n%s│%s %s\n", c.Cyn, c.Rst, c.Bld+"extra_config"+c.Rst)
		for _, k := range extra.Keys() {
			a.printf("%s│%s   %s\n", c.Cyn, c.Rst, k+" = "+encodeJSON(extra.At(k)))
		}
	}
	if env := p.ObjectAt("env"); env.Len() > 0 {
		a.printf("\n%s│%s %s\n", c.Cyn, c.Rst, c.Bld+"env"+c.Rst)
		for _, k := range env.Keys() {
			a.printf("%s│%s   %s\n", c.Cyn, c.Rst, k+"="+scalarString(env.At(k)))
		}
	}
	if args := p.StringsAt("codex_args"); len(args) > 0 {
		a.printf("\n%s│%s %s\n%s│%s   %s\n", c.Cyn, c.Rst, c.Bld+"codex_args"+c.Rst,
			c.Cyn, c.Rst, strings.Join(args, " "))
	}
	if images := p.StringsAt("images"); len(images) > 0 {
		a.printf("\n%s│%s %s\n", c.Cyn, c.Rst, c.Bld+"images"+c.Rst)
		for _, img := range images {
			a.printf("%s│%s   %s\n", c.Cyn, c.Rst, img)
		}
	}
	if rest := p.unknownFields(); rest.Len() > 0 {
		a.printf("\n%s│%s %s\n%s│%s   %s\n", c.Cyn, c.Rst, c.Bld+"其它字段"+c.Rst,
			c.Cyn, c.Rst, encodeJSON(rest))
	}

	a.printf("\n%s│%s %s\n", c.Cyn, c.Rst, c.Bld+"文件"+c.Rst)
	a.printf("%s╰─%s  %s\n", c.Cyn, c.Rst, tildePath(f, a.Home))
	a.printf("\n  %s启动：cdxp %s%s\n", c.Dim, base, c.Rst)
	a.printf("  %s预览命令：cdxp %s --dry-run%s\n\n", c.Dim, base, c.Rst)
	return nil
}

// knownFields are the keys show renders individually; anything else is dumped
// as-is under 其它字段.
var knownFields = map[string]bool{
	"name": true, "description": true, "homepage": true, "author": true,
	"tags": true, "notes": true, "model": true,
	"model_reasoning_effort": true, "model_reasoning_summary": true,
	"model_verbosity": true, "model_context_window": true,
	"model_max_output_tokens": true, "review_model": true,
	"images": true, "provider": true, "auth": true, "extra_config": true,
	"env": true, "codex_args": true, "launch": true,
	"api_key": true, "api_key_env": true, "api_key_cmd": true,
}

func (p *Profile) unknownFields() *object {
	rest := newObject()
	for _, k := range p.raw.Keys() {
		if !knownFields[k] {
			rest.keys = append(rest.keys, k)
			rest.vals[k] = p.raw.At(k)
		}
	}
	return rest
}

// ---------------------------------------------------------------- path / env

func (a *App) cmdPath(args []string) error {
	name := arg0(args)
	f, err := a.ResolveProfile(name)
	if err != nil {
		return dief("找不到 profile：%s", name)
	}
	a.printf("%s\n", f)
	return nil
}

func (a *App) cmdEnv(args []string) error {
	name := arg0(args)
	if name == "" {
		return dief(`用法：cdxp env <name>   然后 eval "$(cdxp env NAME)"`)
	}
	f, err := a.ResolveProfile(name)
	if err != nil {
		return dief("找不到 profile：%s", name)
	}
	p, err := LoadProfile(f)
	if err != nil {
		return dief("profile JSON 解析失败：%s（用 cdxp check %s 看细节）", f, f)
	}

	a.printf("export CODEX_PROFILE=%s\n", shellQuote(p.Name))
	a.printf("export CODEX_MODEL=%s\n", shellQuote(p.Str("model")))
	a.printf("export CODEX_BASE_URL=%s\n", shellQuote(p.Str("provider.base_url")))
	if key := a.ResolveAPIKey(p, ""); key != "" {
		a.printf("export %s=%s\n", p.StrOr("provider.env_key", "CODEXP_API_KEY"), shellQuote(key))
	}
	return nil
}

// ---------------------------------------------------------------- new / edit / rm

func (a *App) cmdNew(args []string) error {
	name := arg0(args)
	if name == "" {
		return dief("用法：cdxp new <name>")
	}
	dir := a.ProfilesDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return dief("无法创建目录：%s", dir)
	}
	f := filepath.Join(dir, name+".json")
	if _, err := os.Stat(f); err == nil {
		return dief("已存在：%s（用 cdxp edit %s 修改）", f, name)
	}
	if err := os.WriteFile(f, []byte(profileTemplate(name)), 0o600); err != nil {
		return dief("写入失败：%s", f)
	}
	c := a.pal
	a.printf("%s已创建%s %s\n\n", c.Grn, c.Rst, f)
	a.printf("  下一步：cdxp edit %s  然后  cdxp %s\n\n", name, name)
	return nil
}

// profileTemplate is the starter file `cdxp new` writes.
func profileTemplate(name string) string {
	n := jsonQuote(name)
	return `{
  "name": ` + n + `,
  "description": "一句话说明这个 profile 用来做什么（显示在 cdxp ls 和启动横幅里）",
  "tags": [
    "tag1",
    "tag2"
  ],
  "homepage": "",
  "notes": "更长的备注：注意事项、计费方式、已知坑等",
  "model": "provider-model-id",
  "provider": {
    "id": ` + n + `,
    "name": "Provider Name",
    "base_url": "https://api.example.com/v1",
    "wire_api": "responses",
    "env_key": "CODEXP_API_KEY",
    "extra": {}
  },
  "auth": {
    "api_key_env": "EXAMPLE_API_KEY",
    "api_key_cmd": "",
    "hint": "export EXAMPLE_API_KEY=sk-…  或  cdxp ` + name + ` --key sk-…"
  },
  "extra_config": {},
  "env": {},
  "codex_args": []
}
`
}

func (a *App) cmdEdit(args []string) error {
	name := arg0(args)
	if name == "" {
		return dief("用法：cdxp edit <name>")
	}
	f, err := a.ResolveProfile(name)
	if err != nil {
		return dief("找不到 profile：%s", name)
	}
	editor := firstNonEmpty(a.Getenv("EDITOR"), a.Getenv("VISUAL"), "vim")
	path, err := a.LookPath(editor)
	if err != nil {
		return dief("找不到编辑器：%s（请设置 EDITOR）", editor)
	}
	cmd := exec.Command(path, f)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return dief("编辑器退出异常：%v", err)
	}
	if _, err := LoadProfile(f); err != nil {
		return dief("profile JSON 解析失败：%s（用 cdxp check %s 看细节）", f, f)
	}
	a.printf("%s✓ JSON 合法%s\n", a.pal.Grn, a.pal.Rst)
	return nil
}

func (a *App) cmdRm(args []string) error {
	force := false
	for len(args) > 0 {
		switch args[0] {
		case "-f", "--force":
			force = true
			args = args[1:]
			continue
		}
		break
	}

	name := arg0(args)
	if name == "" {
		return dief("用法：cdxp rm [-f] <name>")
	}
	f, err := a.ResolveProfile(name)
	if err != nil {
		return dief("找不到 profile：%s", name)
	}
	if !force {
		a.printf("删除 %s ？[y/N] ", f)
		line, err := bufio.NewReader(a.Stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		switch strings.TrimSpace(line) {
		case "y", "Y", "yes":
		default:
			a.printf("已取消\n")
			return nil
		}
	}
	if err := os.Remove(f); err != nil {
		return dief("删除失败：%v", err)
	}
	a.printf("已删除 %s\n", f)
	return nil
}

// ---------------------------------------------------------------- check

func (a *App) cmdCheck(args []string) error {
	name := arg0(args)
	if name == "" {
		return dief("用法：cdxp check <name>")
	}
	f, err := a.ResolveProfile(name)
	if err != nil {
		return dief("找不到 profile：%s", name)
	}
	c := a.pal

	a.printf("\n%s│%s %s\n", c.Cyn, c.Rst, c.Bld+"JSON"+c.Rst)
	p, err := LoadProfile(f)
	if err != nil {
		a.printf("  %s✗%s %s\n", c.Red, c.Rst, strings.ReplaceAll(err.Error(), "\n", " "))
		return exitError{1}
	}
	a.printf("  %s✓%s 语法合法\n", c.Grn, c.Rst)

	model := p.Str("model")
	base := p.Str("provider.base_url")
	envKey := p.StrOr("provider.env_key", "CODEXP_API_KEY")
	key := a.ResolveAPIKey(p, "")

	a.printf("\n%s│%s %s\n", c.Cyn, c.Rst, c.Bld+"必填字段"+c.Rst)
	if model != "" {
		a.printf("  %s✓%s model=%s\n", c.Grn, c.Rst, model)
	} else {
		a.printf("  %s✗%s 缺少 model\n", c.Red, c.Rst)
	}
	if base != "" {
		a.printf("  %s✓%s base_url=%s\n", c.Grn, c.Rst, base)
	} else {
		a.printf("  %s✗%s 缺少 provider.base_url\n", c.Red, c.Rst)
	}

	a.printf("\n%s│%s %s\n", c.Cyn, c.Rst, c.Bld+"API key"+c.Rst)
	if key != "" {
		a.printf("  %s✓%s %s → %s（%d 字符）\n", c.Grn, c.Rst, envKey, maskKey(key), utf8.RuneCountInString(key))
	} else {
		a.printf("  %s✗%s 解析不到 key（env %s / api_key / api_key_cmd 均为空）\n", c.Red, c.Rst, envKey)
	}

	a.printf("\n%s│%s %s\n", c.Cyn, c.Rst, c.Bld+"连通性"+c.Rst)
	if base == "" {
		a.printf("  %s-%s 缺少 base_url，跳过\n", c.Dim, c.Rst)
	} else {
		a.probe(base, model, key, c)
	}
	a.printf("\n")
	return nil
}

func (a *App) probe(base, model, key string, c palette) {
	url := strings.TrimSuffix(base, "/") + "/models"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		a.printf("  %s✗%s 无法连接 %s（%v）\n", c.Red, c.Rst, url, err)
		return
	}
	if key == "" {
		req.Header.Set("Authorization", "Bearer none")
	} else {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := a.HTTP.Do(req)
	if err != nil {
		a.printf("  %s✗%s 无法连接 %s（%v）\n", c.Red, c.Rst, url, err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))

	switch {
	case resp.StatusCode == http.StatusOK:
		a.printf("  %s✓%s GET %s → 200\n", c.Grn, c.Rst, url)
		ids := modelIDs(body, 30)
		if len(ids) > 0 {
			a.printf("  %s模型列表：%s\n", c.Dim, c.Rst)
			found := false
			for _, id := range ids {
				if id == model {
					found = true
					a.printf("    %s✓ %s%s  ← profile 指定\n", c.Grn, id, c.Rst)
					continue
				}
				a.printf("    · %s\n", id)
			}
			if !found {
				a.printf("  %s!%s 列表里没有 %s，请确认模型名\n", c.Yel, c.Rst, model)
			}
		}
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		a.printf("  %s✗%s GET %s → %d（key 无效或无权限）\n", c.Red, c.Rst, url, resp.StatusCode)
	case resp.StatusCode == http.StatusNotFound:
		a.printf("  %s!%s GET %s → 404（该服务可能没有 /models 接口，不代表不可用）\n", c.Yel, c.Rst, url)
	default:
		a.printf("  %s!%s GET %s → %d\n", c.Yel, c.Rst, url, resp.StatusCode)
	}
}

func modelIDs(body []byte, limit int) []string {
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}
	out := make([]string, 0, len(payload.Data))
	for _, m := range payload.Data {
		if m.ID == "" {
			continue
		}
		out = append(out, m.ID)
		if len(out) == limit {
			break
		}
	}
	return out
}

// ---------------------------------------------------------------- doctor

func (a *App) cmdDoctor() error {
	c := a.pal
	a.printf("\n%s│%s %s\n", c.Cyn, c.Rst, c.Bld+"环境"+c.Rst)
	a.printf("  cdxp         %s\n", Version)
	a.printf("  go           %s\n", runtime.Version())
	if path, err := exec.LookPath("codex"); err == nil {
		a.printf("  codex        %s\n", path)
	} else {
		a.printf("  codex        %s\n", "✗ 不在 PATH")
	}
	a.printf("  CODEX_HOME   %s\n", a.codexHome())
	a.printf("  profile 目录：\n")
	if dirs := a.ProfileDirs(); len(dirs) > 0 {
		for _, d := range dirs {
			a.printf("    %s\n", d)
		}
	} else {
		a.printf("    （无，cdxp new 会创建 %s）\n", a.DefaultProfilesDir())
	}

	a.printf("\n%s│%s %s\n", c.Cyn, c.Rst, c.Bld+"profile"+c.Rst)
	profiles := a.CollectProfiles()
	for _, np := range profiles {
		p, err := LoadProfile(np.Path)
		if err != nil {
			a.printf("  %s✗%s %s JSON 解析失败\n", c.Red, c.Rst, padRight(np.Name, 18))
			continue
		}
		a.printf("  %s✓%s %s %s\n", c.Grn, c.Rst, padRight(np.Name, 18), a.KeyStatusShort(p))
	}
	if len(profiles) == 0 {
		a.printf("  （空）\n")
	}
	a.printf("\n")
	return nil
}

func tildePath(path, home string) string {
	if home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+"/") {
		return "~" + path[len(home):]
	}
	return path
}
