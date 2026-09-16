package cdxp

import "strings"

// banner is the startup card printed before codex takes over the terminal.
func (a *App) banner(spec *launchSpec, p *Profile, key string) {
	c := a.pal
	desc := p.Str("description")
	tags := strings.Join(p.StringsAt("tags"), ", ")
	home := p.Str("homepage")
	notes := p.Str("notes")

	a.printf("\n%s%s╭─ Codex profile %s%s%s%s\n", c.Cyn, c.Bld, c.Mag, spec.ProfileName, c.Rst, c.Cyn)
	if desc != "" {
		a.printf("│ %s%s%s\n", c.Bld, desc, c.Rst+c.Cyn)
	}
	a.printf("│ %smodel%s      %s%s%s\n", c.Dim, c.Rst+c.Cyn, c.Grn, spec.Model, c.Rst+c.Cyn)
	a.printf("│ %sprovider%s   %s @ %s  (wire_api=%s)\n", c.Dim, c.Rst+c.Cyn,
		spec.ProviderName, spec.Base, spec.Wire)
	if key != "" {
		a.printf("│ %sauth%s       %s = %s\n", c.Dim, c.Rst+c.Cyn, spec.EnvKey, maskKey(key))
	} else {
		a.printf("│ %sauth%s       %s✗ 未找到 key%s\n", c.Dim, c.Rst+c.Cyn, c.Yel, c.Rst+c.Cyn)
	}
	if tags != "" {
		a.printf("│ %stags%s       %s\n", c.Dim, c.Rst+c.Cyn, tags)
	}
	if home != "" {
		a.printf("│ %shomepage%s   %s\n", c.Dim, c.Rst+c.Cyn, home)
	}
	if notes != "" {
		for _, line := range strings.Split(strings.TrimRight(notes, "\n"), "\n") {
			a.printf("│ %s%s%s\n", c.Dim, line, c.Rst+c.Cyn)
		}
	}
	a.printf("%s╰─ %s%s%s\n\n", c.Cyn, c.Dim, tildePath(p.Path, a.Home), c.Rst)
}

func (a *App) printUsage() {
	rep := strings.NewReplacer(
		"@@BLD@@", a.pal.Bld,
		"@@RST@@", a.pal.Rst,
		"@@DIM@@", a.pal.Dim,
		"@@VER@@", Version,
		"@@DIR@@", a.DefaultProfilesDir(),
	)
	a.printf("%s", rep.Replace(usageTemplate))
}

const usageTemplate = `@@BLD@@cdxp@@RST@@ @@VER@@ — Codex profile 启动器

@@BLD@@用法@@RST@@
  cdxp                          列出所有 profile 及其描述
  cdxp <name> [codex 参数…]     读 codex-profiles/<name>.json 并启动 codex
  cdxp <name> --dry-run         只打印将要执行的命令（key 打码）

@@BLD@@选项@@RST@@（写在 profile 名之前）
  -q, --quiet      不打印启动横幅
      --dry-run    不启动，只显示解析结果
      --key <key>  临时覆盖该 profile 的 API key

@@BLD@@管理@@RST@@
  cdxp ls [-1]          列出 profile（-1 只输出名字）
  cdxp show <name>      查看完整 metadata + 连接配置
  cdxp check <name>     JSON / 必填字段 / key / 连通性体检
  cdxp path <name>      打印 profile 文件路径
  cdxp env <name>       输出可 eval 的 export 语句
  cdxp new <name>       新建 profile 模板
  cdxp edit <name>      用 $EDITOR 打开
  cdxp rm [-f] <name>   删除
  cdxp doctor           环境自检
  cdxp schema           字段说明

@@BLD@@profile 目录@@RST@@（按优先级）
  ${CDXP_PROFILES_DIR}  @@DIR@@  ~/codex-profiles

@@BLD@@例子@@RST@@
  cdxp deepseek                 用 DeepSeek 启动
  cdxp deepseek --search        附加 codex 参数
  cdxp deepseek exec "hi"       非交互跑一次
  eval "$(cdxp env deepseek)"   在当前 shell 导出配置，之后直接用 codex
`

func (a *App) printSchema() {
	a.printf("%s", schemaText)
}

const schemaText = `Codex profile —— JSON 字段说明
==============================

放置目录（按优先级）：
  $CDXP_PROFILES_DIR、$CODEX_HOME/codex-profiles、~/.codex/codex-profiles、~/codex-profiles
文件名即 profile 名：deepseek.json → cdxp deepseek

除 model、provider.base_url 外都可省略：

  name             string   显示名，缺省用文件名
  description      string   一句话描述，cdxp ls 与启动横幅都会显示    ← metadata
  tags             array    标签
  homepage         string   平台/文档地址
  author           string   维护者
  notes            string   多行备注（启动横幅用暗色打印）            ← metadata

  model            string   模型 id（必填）→ -c model=…
  provider.id      string   提供方 id，默认取文件名 → model_provider
  provider.name    string   显示名
  provider.base_url string  必填，如 https://api.deepseek.com/v1（不要带 /chat/completions）
  provider.wire_api string  "chat" 或 "responses"（默认 responses）
                            仅兼容 OpenAI /chat/completions 的服务必须用 "chat"
  provider.env_key string   把 key 传给 codex 用的环境变量名（默认 CODEXP_API_KEY）
  provider.requires_openai_auth boolean  可选
  provider.extra    object  原样映射到 model_providers.<id>.<key>，例如：
                           {"http_headers": {"X-Foo": "bar"},
                            "query_params": {"api-version": "2025-04-01-preview"},
                            "env_http_headers": {"X-Key": "MY_KEY_ENV"},
                            "request_max_retries": 3, "stream_max_retries": 5,
                            "stream_idle_timeout_ms": 300000,
                            "experimental_bearer_token": "…"}

  auth.api_key      string  直接内联 key（不推荐，文件会是明文）
  auth.api_key_env  string  从该环境变量读取（推荐）
  auth.api_key_cmd  string  从命令输出读取（取第一行，可从密钥管理器取值）
  auth.hint         string  拿不到 key 时显示的提示
  优先级：--key 参数 > api_key_env > api_key > api_key_cmd

  model_reasoning_effort    string   minimal | low | medium | high …
  model_reasoning_summary   string
  model_verbosity           string
  model_context_window      number
  model_max_output_tokens   number
  review_model              string

  images           array   每次启动默认附带的图片（多模态），映射到 --image=<path>
                           支持 ~ 展开；TUI 里也可以直接 Ctrl+V 粘贴图片

  extra_config     object  任意 -c 覆盖：key 是点号路径，value 直接转 TOML
                           例 {"tools.web_search": true, "model_context_window": 128000}
  env              object  启动时注入的额外环境变量
                           （auth.api_key_env 指定的变量若在 shell 里没设置，
                             也会从这里取值，所以 profile 可以自包含）
  codex_args       array   每次启动默认追加的 codex 参数，如 ["--search"]
  launch.sandbox   string  → -s（read-only | workspace-write | danger-full-access）
  launch.approval  string  → -a（on-request | never）
  launch.cd        string  → -C，指定工作目录

命令：
  cdxp [--quiet] [--dry-run] [--key sk-…] <name> [codex 参数…]
  cdxp ls | show <name> | check <name> | path <name> | env <name>
  cdxp new <name> | edit <name> | rm [-f] <name>
  cdxp doctor | schema | version
`
