# u2b

**面向主流视频平台的跨端极速下载工具**（原生适配 **YouTube、B站、抖音、小红书、快手、TikTok、X** 等主流平台）—— 无论是在电脑网页上点一下右键、在 iPhone 上随手分享，还是在终端敲一行命令，都能把高清视频（1080p / 4K）或纯享 MP3 秒速存下来。

- **零门槛上手**：一行命令自动装齐所需组件，无需配置复杂的开发环境
- **全场景打通**：电脑浏览器右键直下 · iPhone 扫码/快捷指令远程下 · 命令行极简下
- **会员视频直通**：自动复用桌面浏览器登录态，公开视频与频道会员专属视频同样顺畅
- **分类井井有条**：自动识别电脑影音目录与外置硬盘，支持新建分类文件夹归档

---

## 1. 一键安装

打开电脑终端，复制对应系统的一行命令回车即可（会自动补齐 `yt-dlp`、`ffmpeg` 依赖并弹出浏览器插件引导）：

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/sixban6/u2b/main/install.sh | bash
```

### Windows 11 / 10（打开 PowerShell）

```powershell
irm https://raw.githubusercontent.com/sixban6/u2b/main/install.ps1 | iex
```

---

## 2. 三种使用方式（任选其一）

### 💻 方式 A：电脑网页鼠标右键下载（日常最推荐）

安装完成后会自动打开引导页与插件文件夹（支持 **Brave / Chrome / Edge**）：
1. 按页面提示把已高亮的 `extension` 文件夹拖入浏览器扩展页，即可完成首次启用；
2. 之后打开 YouTube，在**任意视频封面、标题或播放页直接点击鼠标右键**：
   - 一键下载高清视频（默认 1080p，可在右上角插件面板一键切换为 **原画 4K**）或仅提取音频（MP3）
   - 直接选择保存的分类文件夹、单次自定义文件名与画质，或一键下载整个播放列表

> 如果后续新装了浏览器，随时在终端运行 `u2b ext` 即可重新打开挂载引导。

---

### 📱 方式 B：iPhone 手机端下载（存电脑 / 存手机本地）

想在床上刷 iPhone Brave 时把视频存进电脑分类库，或直接保存到手机本地：

**第一步：在电脑开启后台服务（仅需执行一次，支持开机静默自启）**
```bash
u2b serve install
```
执行后电脑会自动弹出配置页，接下来在手机上有两种用法：

- **方式一 · 手机扫码**：用 iPhone 相机扫描屏幕二维码，直接在手机浏览器进入 **「下载中心」**，粘贴链接即可投递到电脑下载或下完存回手机。
- **方式二 · 安装快捷指令**：在配置页点击导入 **「u2b 下载」快捷指令**。此后在 iPhone Brave 看到喜欢的视频，直接点 **「分享 → u2b 下载」** 即可一键下载，点完即走。

---

### ⌨️ 方式 C：终端命令行极简下载

喜欢用命令行的用户，直接输入 `u2b + 链接` 即可开箱使用：

```bash
# 1. 直接下载视频（默认 1080p 高清；需要 4K 原画可加 -q 4k）
u2b "https://www.youtube.com/watch?v=VIDEO_ID"
u2b -q 4k "https://www.youtube.com/watch?v=VIDEO_ID"

# 2. 仅提取 MP3 音频（加 -m）
u2b -m "https://www.youtube.com/watch?v=VIDEO_ID"

# 3. 指定保存目录 (-o) 和自定义文件名 (-n)
u2b -m -o ./downloads -n "Star Sky" "https://www.youtube.com/watch?v=pICAha0nsb0"

# 4. 下载整个播放列表 / 合集（加 pl）
u2b pl "https://www.youtube.com/playlist?list=PLAYLIST_ID"
```

---

## 3. 一键更新

无需重新运行安装脚本，任何时候想升级到最新版本，只需在终端输入：

```bash
u2b update
```

> 常用附加参数：`u2b update --check`（仅检查新版本）、`u2b update --deps`（同时升级 `yt-dlp`）。

---

## 💡 常见问题与进阶设置

<details>
<summary><strong>1. 频道会员专属视频 / 年龄限制视频怎么下？</strong></summary>
<br/>

无需手动导出繁琐的 `cookies.txt`：只要电脑上的 Brave / Chrome 安装了 `u2b` 右键插件并正常登录过 YouTube，登录态就会自动同步，电脑右键和 iPhone 手机端都能直接下载会员视频，且不会反复弹出系统钥匙串授权框。
</details>

<details>
<summary><strong>2. 出门在外（4G/5G 流量）怎么用手机连家里的电脑下载？</strong></summary>
<br/>

在电脑和 iPhone 上同时登录免费的 [Tailscale](https://tailscale.com/) 即可。`u2b serve` 会自动识别 Tailscale 专网地址，出门在外也能像在家里 Wi-Fi 一样随时投递任务。如需额外设置访问口令，可在配置文件中填写 `server.token` 或使用 `u2b serve --token <口令>`。
</details>

<details>
<summary><strong>3. 如何配置网络代理或修改默认画质 / 保存路径？</strong></summary>
<br/>

单次命令行下载可直接加 `--proxy http://127.0.0.1:7890`；如需长期生效，运行以下命令生成配置文件（默认位于 `~/.u2b/config.yaml`）：

```bash
u2b config init
```

按需修改其中的代理或默认目录即可：

```yaml
download:
  output_dir: "./downloads"

quality:
  preferred: "hd1080"   # 可选: hd2160 (4K) / hd1080 / hd720 / best

proxy:
  enabled: false
  url: ""               # 例如: "http://127.0.0.1:7890"

server:
  listen: "0.0.0.0:8989"
  token: ""             # 可选：手机远程访问口令
```
</details>

<details>
<summary><strong>4. 如何查看或关闭电脑后台常驻服务？</strong></summary>
<br/>

```bash
u2b serve status      # 查看后台服务运行状态与手机连接地址
u2b serve uninstall   # 停止并移除开机自启后台服务
```
</details>

---

## 🛡️ 安全说明

- **构建与更新可追溯**：所有 macOS / Windows / Linux 预编译程序均由本仓库 **GitHub Actions 官方流水线**自动构建，一键安装与 `u2b update` 全程强制校验 **SHA-256 哈希 + SLSA 构建溯源证明 + macOS 代码签名**；
- **服务端零信任防护**：`u2b serve` 默认自动生成高强度随机安全口令（扫码即自动携带，零操作负担），内置跨站请求（CSRF）拦截、主流视频域名白名单（阻断内网 SSRF）与影音目录写入沙箱；
- **macOS 内核级沙盒隔离**：在 macOS 上自动启用系统级 **Seatbelt 内核沙盒**包裹底层 `yt-dlp` 与 `ffmpeg` 解析进程，全盘默认禁写（仅放行当前下载目录）并内核级隔离 `~/.ssh` 等敏感密钥目录。

## 📄 License

MIT License.
