# cdxp — Codex Profile 启动器

给 Codex CLI 套一层「具名 profile」：每个 profile 是一个 JSON 文件，装着一整套连接参数
（`model` / `base_url` / `wire_api` / API key …）外加展示用的 metadata。
`cdxp <name>` 把它翻译成 `codex -c …` 然后启动，于是「切供应商」变成切一个名字：

```bash
cdxp deepseek          # 用 DeepSeek 起一个会话
cdxp ks exec "跑一下测试"   # 非交互跑一次
```

一个静态二进制，没有运行时依赖（不需要 jq），Linux / macOS / Termux / Windows 都一样，
本仓库不依赖任何 Termux 专有特性。

## 安装

```bash
curl -fsSL https://raw.githubusercontent.com/aaaa0ggMC/cdxp-termux/main/scripts/install-cdxp.sh | bash
```

装到别处：

```bash
curl -fsSL https://raw.githubusercontent.com/aaaa0ggMC/cdxp-termux/main/scripts/install-cdxp.sh | INSTALL_DIR="$HOME/.local/bin" bash
```

各平台（macOS / Linux / Windows，amd64 与 arm64）的二进制也发布在
[Releases](https://github.com/aaaa0ggMC/cdxp-termux/releases)。

### Termux / Android

Termux 里 `PREFIX` 会被自动识别，上面的一行命令直接装到 `$PREFIX/bin`，无需 `sudo`。
也可以就地编译：

```bash
pkg install golang
git clone https://github.com/aaaa0ggMC/cdxp-termux.git
cd cdxp-termux && go build -o "$PREFIX/bin/cdxp" ./cmd/cdxp
```

### 从源码构建

```bash
go build -o cdxp ./cmd/cdxp
go test ./...
```

## 快速开始

需要先有 `codex` 在 `PATH` 里（`codex login` 过一次）。

```bash
cdxp new deepseek                     # 生成 ~/.codex/codex-profiles/deepseek.json
cdxp edit deepseek                    # 填 model / base_url
export DEEPSEEK_API_KEY=sk-…          # key 走环境变量，不落盘
cdxp deepseek                         # 启动
```

`cdxp doctor` 会自检环境、列出 profile 目录和每个 profile 的 key 状态；
`cdxp check <name>` 更进一步，做 JSON / 必填字段 / key / 连通性体检；
`cdxp <name> --dry-run` 只打印将要执行的命令（key 打码）。

## profile 长什么样

最小可用：

```json
{
  "description": "给 cdxp ls 看的一句话",
  "model": "deepseek-chat",
  "provider": {
    "base_url": "https://api.deepseek.com/v1",
    "wire_api": "chat"
  },
  "auth": { "api_key_env": "DEEPSEEK_API_KEY" }
}
```

除 `model` 和 `provider.base_url` 外都可以省略。完整字段说明：

```bash
cdxp schema
```

几个值得知道的点：

- `provider.wire_api` 默认 `responses`；只兼容 OpenAI `/chat/completions` 的服务（DeepSeek、大多数中转站）必须写 `chat`。
- `provider.extra` 里的键会原样映射到 `model_providers.<id>.<key>`，比如 `http_headers`、`query_params`、`request_max_retries`。
- `extra_config` 是任意 `-c` 覆盖，键写成点号路径，例如 `{"tools.web_search": true}`。
- `images` 里列出的图片每次启动自动带上（`--image=<path>`，支持 `~`）；TUI 里也可以直接 Ctrl+V 粘贴。
- `launch.sandbox` / `launch.approval` / `launch.cd` 分别对应 `-s` / `-a` / `-C`。

profile 按 `$CDXP_PROFILES_DIR` → `$CODEX_HOME/codex-profiles` → `~/.codex/codex-profiles`
→ `~/codex-profiles` 的顺序查找，同名以靠前的目录为准。

## 命令

| 命令 | 作用 |
| --- | --- |
| `cdxp` / `cdxp ls [-1]` | 列出 profile 及其描述、key 状态 |
| `cdxp <name> [codex 参数…]` | 用该 profile 启动 codex，参数原样透传 |
| `cdxp show <name>` | 完整 metadata + 连接配置 |
| `cdxp check <name>` | JSON / 必填字段 / key / 连通性体检 |
| `cdxp path <name>` | 打印 profile 文件路径 |
| `cdxp env <name>` | 输出可 `eval` 的 export 语句 |
| `cdxp new <name>` / `edit <name>` / `rm [-f] <name>` | 新建 / 编辑 / 删除 |
| `cdxp doctor` | 环境自检 |
| `cdxp schema` | 字段说明 |

写在 profile 名之前的选项：`-q` / `--quiet`（不打印横幅）、`--dry-run`（只预览）、
`--key <key>`（临时覆盖该 profile 的 key）。

## API key 的取法

优先级：`--key` 参数 > `auth.api_key_env` > `auth.api_key` > `auth.api_key_cmd`。

`api_key_env` 指定的变量先看当前 shell，再看 profile 自己的 `env` 对象，所以 profile 可以自包含。
推荐用环境变量，别把 key 写进 JSON。

## 相关项目

[aaaa0ggMC/codex-proxy](https://github.com/aaaa0ggMC/codex-proxy) 是一个独立项目
（fork 自 [Max-Leopold/codex-proxy](https://github.com/Max-Leopold/codex-proxy)）：
把 Codex 的 ChatGPT 登录包装成本地 OpenAI 兼容接口。它和 cdxp 没有代码依赖——
cdxp 只负责拼 `codex` 的命令行，可以指向任何服务，包括 codex-proxy 起的本地端口。

## License

MIT，见 [LICENSE](LICENSE)。
