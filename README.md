# u2b

面向 YouTube 的现代化跨端极速下载工具，集成了 **全功能命令行（CLI）**、**桌面浏览器右键下载插件（Brave / Chrome / Edge）** 与 **iPhone / 移动端 Brave 远端及本地下载（iOS 快捷指令 + 手机 Web 面板）**，内置高质量 1080p/4K 视频合并与 MP3 提取，原生支持公开视频及会员专属视频（`yt-dlp` 驱动）。

---

## 🚀 极速一键安装（推荐）

通过 GitHub Actions 自动编译与 Releases 发布，**无需本地安装 Go 编译环境**，秒级部署：

### macOS / Linux

打开终端执行以下一行命令：

```bash
curl -fsSL https://raw.githubusercontent.com/sixban6/u2b/main/install.sh | bash
```
> 或克隆仓库后运行 `./install.sh`。脚本将自动识别系统架构（Apple Silicon M系列 / Intel x64 / Linux arm64 / amd64），优先下载官方 Release 预编译二进制，通过 Homebrew / apt / dnf 补齐 `yt-dlp` 与 `ffmpeg`，并自动部署浏览器扩展。

### Windows 11 / Windows 10

以当前用户打开 PowerShell 执行以下一行命令：

```powershell
irm https://raw.githubusercontent.com/sixban6/u2b/main/install.ps1 | iex
```
> 或克隆仓库后运行 `powershell -ExecutionPolicy Bypass -File .\install.ps1`。脚本将自动识别架构（x64 / ARM64），通过 `winget` 自动安装 `yt-dlp` 与 `ffmpeg`，加入用户 PATH 并自动配置 Chromium 浏览器原生通信宿主。

---

## 📱 iPhone 手机端 Brave 下载支持（快捷指令 + 手机 Web 面板）

针对 iOS 端 Brave 无法安装扩展的限制，`u2b` 内置了轻量跨端服务（`u2b serve`），融合了 **iOS 系统分享快捷指令（一键投递/存回手机）** 与 **内置移动端 Web 面板（PWA）**：

### 1. 在 Mac / 服务端一键启动（支持开机静默常驻）

```bash
# 方式 A：安装为系统后台常驻服务（推荐，开机自启、无终端黑框、自动弹出配置页）
u2b serve install

# 方式 B：临时在前台启动（终端直接打印扫码二维码）
u2b serve
```
> 随时可通过 `u2b serve status` 查看运行状态，或 `u2b serve uninstall` 移除后台服务。  
> 若需要在外网（4G/5G）随时调用，推荐在 Mac 与 iPhone 同时安装 **Tailscale**，`u2b serve` 会自动识别 Tailscale 地址并支持 `--token <口令>` 鉴权。

### 2. iPhone 扫码 10 秒完成配置
1. 用 iPhone 相机扫描终端（或 `http://<Mac主机名>.local:8989/setup` 网页）上的二维码，在手机 **Brave** 中打开引导页。
2. **方案 A · iOS 分享菜单快捷指令（日常主力）**：
   - 点击页面上的 **「📥 一键获取 u2b 下载快捷指令」**（macOS 服务端已自动调用 Apple 官方签名，也可直接在 Mac 点击导入并通过 iCloud 自动同步到 iPhone）；
   - 此后在 iPhone Brave 播放页或首页**长按任意视频封面 → 点「分享」→ 点「u2b 下载」**，即可选择：
     - `⚡ 快速下载视频 (存电脑 1080p)` / `🎵 快速提取 MP3 (存电脑)`（点完即走，顶部原生通知反馈）
     - `📲 下载视频 / MP3 到 iPhone 本地`（解析完成后直接弹出系统保存菜单存入 iPhone 文件 App）
     - `📂 选分类目录 / 打开 Web 面板`
3. **方案 B · 手机端 Web 控制台（PWA）**：
   - 在 iPhone Brave 点击「分享 → 添加到主屏幕」，即可像原生 App 一样查看 **SSE 实时下载进度条、网速、新建分类文件夹（边下边建）**，并支持一键将已下载的视频/MP3 **取回手机本地**或加入 **Brave Playlist** 离线播放。
4. **会员专属视频无感共享**：只要桌面端 Brave 安装过 `u2b` 扩展，YouTube 会员 Cookie 会自动同步缓存供手机端下载使用，全程不弹 macOS 钥匙串授权框。

---

## ⚡ 命令行最简使用

全局命令精简为 `u2b`（无需输入 `download` 子命令，所有参数开箱即用）：

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

## 🌐 桌面浏览器右键下载扩展（Brave / Chrome / Edge）

在终端运行以下命令自动注册本地通信（Native Messaging Host）并开启浏览器开发者模式：

```bash
u2b ext install
```

- **网页鼠标右键下载**：在 YouTube 任意视频封面、标题链接或播放页点击右键即可一键下载 1080p 视频、提取 MP3、选择分类目录或下载整个播放列表；
- **外置盘与分类目录自动探测**：自动识别系统音乐、视频库及外置硬盘中的子文件夹，与手机端 Web 面板实时共享同一套目录配置。

---

## ⚙️ 高级配置（代理 / 端口 / 鉴权口令）

初始化或验证本地配置文件：

```bash
u2b config init
u2b config validate
```

默认配置模板（`config.yaml` / `~/.u2b/config.yaml`）：

```yaml
auth:
  method: "none"        # 可设为 "none" / "browser" / "cookies_file"
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

server:
  listen: "0.0.0.0:8989"
  token: ""             # 可选：远程访问口令
```

---

## 📄 License

MIT License.
