# u2b (u2b_download)

面向 YouTube 的现代化极速下载工具，支持**全功能命令行（CLI）**与**浏览器右键极速下载插件（Brave / Chrome / Edge）**，内置高质量 1080p 视频合并与 MP3 无损提取，支持公开视频及会员专属视频（`yt-dlp` 驱动）。

---

## 🚀 极速一键安装（推荐）

通过 GitHub Actions 自动编译与 Releases 发布，**无需本地安装 Go 编译环境**，秒级极速部署：

### macOS / Linux

打开终端执行以下一行命令：

```bash
curl -fsSL https://raw.githubusercontent.com/sixban6/u2b_download/main/install.sh | bash
```
> 或克隆仓库后运行 `./install.sh`。脚本将自动识别系统架构（Apple Silicon M系列 / Intel x64 / Linux arm64 / amd64），优先下载官方 Release 预编译二进制，通过 Homebrew / apt / dnf 补齐 `yt-dlp` 与 `ffmpeg`，并自动部署浏览器扩展。

### Windows 11 / Windows 10

以当前用户打开 PowerShell 执行以下一行命令：

```powershell
irm https://raw.githubusercontent.com/sixban6/u2b_download/main/install.ps1 | iex
```
> 或克隆仓库后运行 `powershell -ExecutionPolicy Bypass -File .\install.ps1`。脚本将自动识别架构（x64 / ARM64），下载对应预编译程序，通过 Win11 自带 `winget` 自动安装 `yt-dlp` 与 `ffmpeg`，加入用户 PATH 并自动配置 Chromium 浏览器原生通信宿主。

---

## ⚡ 命令行最简使用

安装后系统全局支持简写命令 `u2b` 及完整命令 `u2b_download`（无需输入 `download` 子命令，所有参数开箱即用）：

1. **直接下载视频**（默认优选 1080p 高清）：
```bash
u2b "https://www.youtube.com/watch?v=VIDEO_ID"
```

2. **指定下载目录 `-o` 和文件名 `-n`**：
```bash
u2b -o ./downloads -n "Star Sky" "https://www.youtube.com/watch?v=pICAha0nsb0"
```

3. **仅提取 MP3 纯音频**（使用 `-m` 简写，或 `--format mp3`）：
```bash
u2b -m -o ./downloads -n "Star Sky" "https://www.youtube.com/watch?v=pICAha0nsb0"
```

4. **下载播放列表 / 合集**（使用 `pl` 或 `series`）：
```bash
u2b pl "https://www.youtube.com/playlist?list=PLAYLIST_ID"
```

5. **查看视频可用格式与信息**：
```bash
u2b info "https://www.youtube.com/watch?v=VIDEO_ID"
```

---

## 🌐 浏览器右键下载扩展（Brave / Chrome / Edge）

安装脚本执行完毕后，会自动探测系统中的 Chromium 浏览器并注册本地通信（Native Messaging Host）。你也可以随时在终端运行：

```bash
u2b ext install
```

### 极简 2 步挂载流程
1. **开发者模式已自动开启**：`u2b` 会在安装时自动为你的浏览器 Profile 启用「开发者模式 (Developer mode)」，无需在界面翻找开关。
2. **引导页与高亮文件夹已就绪**：
   - 浏览器将自动打开本地精美指引页 `nav.html`，点击按钮一键复制扩展管理页地址（如 `brave://extensions`、`chrome://extensions`、`edge://extensions`）；
   - 系统文件管理器（Finder / 资源管理器）已自动在后台打开并**蓝色高亮选中了 `extension` 文件夹**；
   - 在浏览器新标签页回车打开扩展页，直接将已高亮选中的 `extension` 文件夹拖入即可完成挂载！

### 插件功能特性
- **网页鼠标右键下载**：在 YouTube 任意视频封面、标题链接或播放页点击右键即可调用：
  - `⚡ u2b 一键极速下载 (1080p)`
  - `🎵 u2b 提取纯音频 (MP3)`
  - `✏️ 指定文件名 / 目录下载...`
- **外置盘与分类目录自动探测**：自动识别系统音乐、视频库及外置硬盘中的子文件夹作为下载分类，支持在插件弹窗中直接“边下边建”新目录；
- **原生后台静默执行**：利用 Native Messaging 协议唤醒本地 CLI 进行下载，无常驻后台守护进程，零内存占用。

---

## ⚙️ 高级配置（下载会员视频 / 代理 / Cookies）

如需下载会员专属视频或配置网络代理，初始化本地配置文件：

```bash
u2b config init
u2b config validate
```

配置文件路径：`~/.u2b/config.yaml`（Windows 为 `%USERPROFILE%\.u2b\config.yaml`）。

### 默认配置模板（与 `config.yaml` 对齐）

```yaml
auth:
  method: "none"        # 可设为 "browser" 或 "cookies_file"
  browser: "brave"      # 自动提取 Cookie 的浏览器: brave / chrome / edge
  cookies_file: ""      # 或指定导出的 cookies.txt 路径

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
  url: ""               # 例如: "http://127.0.0.1:7890"

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

---

## 📄 License

MIT License.
