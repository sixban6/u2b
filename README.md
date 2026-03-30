# u2b_download

一个面向 YouTube 的命令行下载工具，支持公共视频和会员视频（`yt-dlp` 驱动）。

## 最简安装

1) 安装 `yt-dlp`

```bash
# macOS
brew install yt-dlp
```

2) 安装 `u2b_download`

```bash
go install git.sr.ht/~sixban/u2b_download@latest
```

3) 验证

```bash
u2b_download --version
```

## 最简使用

无需先写配置，直接下载（默认参数已内置）：

```bash
u2b_download "https://www.youtube.com/watch?v=VIDEO_ID"
```

指定清晰度：

```bash
u2b_download download -q hd720 "https://www.youtube.com/watch?v=VIDEO_ID"
```

指定下载目录和文件名（如 `Star Sky`）：

```bash
u2b_download download -o /Volumes/lexar/音乐/高昂 -n "Star Sky" "https://www.youtube.com/watch?v=pICAha0nsb0"
```

需要会员视频时，再初始化并修改配置：

```bash
u2b_download config init
u2b_download config validate
```

## 默认参数（与 `config.yaml` 对齐）

```yaml
auth:
  method: "none"
  browser: "brave"
  cookies_file: ""

download:
  output_dir: "./downloads"
  output_template: "%(uploader)s - %(title)s [%(id)s].%(ext)s"
  concurrent_downloads: 3
  keep_temp_files: false

quality:
  preferred: "hd1080"
  fallback: "hd720"
  audio_quality: "best"

proxy:
  enabled: false
  url: ""

advanced:
  user_agent: ""
  retry_count: 3
  timeout: "30m"
  verbose: false
  ytdlp_path: ""
  extra_args: []

youtube:
  client: "web_creator"
  skip_premium_only: true
  extract_chapters: true
```

## TDD 与测试

- 所有测试文件均放在 `test/` 目录。
- 开发流程：先写测试，再实现，再回归。

```bash
go test ./test/...
go test ./...
```
