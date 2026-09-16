package cdxp

import (
	"os"
	"strings"
)

// launch parses cdxp's own flags, resolves the profile, and hands the process
// over to codex, passing every other argument through untouched.
func (a *App) launch(args []string) error {
	var (
		dryRun      bool
		quiet       bool
		keyOverride string
		restAll     []string
	)

scan:
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--dry-run":
			dryRun = true
		case arg == "--quiet", arg == "-q", arg == "--no-banner":
			quiet = true
		case arg == "--key":
			if i+1 < len(args) {
				keyOverride = args[i+1]
				i++
			} else {
				keyOverride = ""
			}
		case strings.HasPrefix(arg, "--key="):
			keyOverride = strings.TrimPrefix(arg, "--key=")
		case arg == "--":
			restAll = append(restAll, args[i+1:]...)
			break scan
		default:
			restAll = append(restAll, arg)
		}
	}

	if len(restAll) == 0 {
		a.printUsage()
		return exitError{1}
	}
	name, rest := restAll[0], restAll[1:]

	f, err := a.ResolveProfile(name)
	if err != nil {
		a.warn("找不到 profile：%s", name)
		if s := a.SuggestNames(name); s != "" {
			a.eprintf("你是不是想要：\n%s\n", s)
		} else {
			a.eprintf("可用 profile（%s）：\n", a.ProfilesDir())
			for _, n := range a.KnownProfiles() {
				a.eprintf("  %s\n", n)
			}
		}
		return exitError{1}
	}
	if !isFile(f) {
		return dief("profile 文件不存在：%s", f)
	}
	p, err := LoadProfile(f)
	if err != nil {
		return dief("profile JSON 解析失败：%s（用 cdxp check %s 看细节）", f, f)
	}

	key := a.ResolveAPIKey(p, keyOverride)
	spec, err := a.BuildCodexArgs(p)
	if err != nil {
		return err
	}
	spec.EnvPairs = a.BuildEnvPairs(p, spec, key)

	if !quiet {
		a.banner(spec, p, key)
		if key == "" && spec.EnvKey != "" {
			c := a.pal
			a.eprintf("%s! 该 profile 需要鉴权 key，但 auth.api_key_env / api_key / api_key_cmd 都没取到值%s\n", c.Yel, c.Rst)
			a.eprintf("%s  export %s=sk-…     或      cdxp %s --key sk-…%s\n\n", c.Yel, spec.EnvKey, spec.ProfileName, c.Rst)
		}
	}

	if dryRun {
		a.PrintCommand(spec, key)
		return nil
	}

	codexPath, err := a.LookPath("codex")
	if err != nil {
		return dief("PATH 中找不到 codex")
	}

	env := mergeEnv(os.Environ(), append([]string{
		"CODEX_PROFILE=" + spec.ProfileName,
		"CODEX_PROFILE_FILE=" + f,
	}, spec.EnvPairs...))

	argv := append([]string{"codex"}, spec.Args...)
	argv = append(argv, rest...)
	return a.Exec(codexPath, argv, env)
}
