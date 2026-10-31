# u2b_download

一个面向 YouTube 的命令行下载工具，支持公共视频和会员视频（`yt-dlp` 驱动）。

## 最简安装
```bash
git clone https://git.sr.ht/~sixban/u2b_download
cd u2b_download
chmod +x install.sh
./install.sh
```

Windows（Git Bash）：
```bash
git clone https://git.sr.ht/~sixban/u2b_download
cd u2b_download
bash install.sh
```
`install.sh` 会自动检测系统，并在 Windows 下优先使用 `winget`（其次 `choco` / `scoop`）安装 `yt-dlp`。

## 最简使用

安装后同时提供简写命令 `u2b` 和完整命令 `u2b_download`（无需输入 `download` 子命令，直接支持所有参数）：

1. 直接下载视频（默认参数已内置）：
```bash
u2b "https://www.youtube.com/watch?v=VIDEO_ID"
```

2. 指定下载目录 `-o` 和文件名 `-n`（如 `Star Sky`）：
```bash
u2b -o ./downloads -n "Star Sky" "https://www.youtube.com/watch?v=pICAha0nsb0"
```

3. 只下载 MP3 音频（使用 `-m` 简写，或 `--format mp3`）：
```bash
u2b -m -o ./downloads -n "Star Sky" "https://www.youtube.com/watch?v=pICAha0nsb0"
```

4. 下载播放列表 / 合集（使用 `pl` 或 `series`）：
```bash
u2b pl "https://www.youtube.com/playlist?list=PLAYLIST_ID"
```

## Brave / Chrome 浏览器右键下载插件

运行 `./install.sh` 或执行以下命令会自动生成插件并注册本地通信宿主：
```bash
u2b ext install
```
- 首次启用只需在打开的 `brave://extensions` 页面开启右上角「开发者模式」，将弹出的 Finder 中的 `~/u2b/extension` 文件夹拖入浏览器（或点击「加载已解压的扩展程序」直接选择 `~/u2b/extension`）即可。
- 在 YouTube 任意视频封面、链接或播放页点击**鼠标右键 → `📥 u2b 下载`**：
  - 支持一键下载 1080p 视频或 MP3 音频；
  - 支持直接选择分类子目录（自动探测外置盘或系统媒体库下的分类文件夹，也可在插件弹窗中一键更换扫描根目录或「边下边建」新分类）；
  - 支持 **「✏️ 指定文件名 / 目录下载...」**，页内弹窗改名回车即下；
  - 页面右上角实时显示下载进度、速度，完成后可一键点击 **「📂 在 Finder 中显示」**。

## 高级使用
需要会员视频时，再初始化并修改配置：

```bash
u2b config init
u2b config validate
```

默认参数（与 `config.yaml` 对齐）

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
