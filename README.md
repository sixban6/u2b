# u2b_download 安装和使用指南

## 安装步骤

### 1. 安装前置依赖

#### yt-dlp (必需)
```bash
# macOS (使用 Homebrew)
brew install yt-dlp

# 验证安装
yt-dlp --version
```

### 2. 构建项目

```bash
# 克隆项目
git clone https://git.sr.ht/~sixban/u2b_download
cd u2b_download

# 下载依赖
go mod download

# 构建项目
go build -o u2b_download

# 验证构建
./u2b_download --version
```

### 3. 安装到系统路径 (可选)

```bash
# 复制到系统路径
sudo cp u2b_download /usr/local/bin/

# 验证安装
u2b_download --version
```

## 配置设置

### 1. 初始化配置文件

```bash
u2b_download config init
```

这将创建 `config.yaml` 文件。

### 2. 针对 Mac + Brave 浏览器的配置

编辑 `config.yaml`:

```yaml
auth:
  method: "browser"
  browser: "brave"

download:
  output_dir: "./downloads"
  output_template: "%(uploader)s - %(title)s [%(id)s].%(ext)s"
  concurrent_downloads: 3

quality:
  preferred: "hd1080"
  fallback: "hd720"
  audio_quality: "best"

advanced:
  retry_count: 3
  timeout: "30m"
  verbose: false

youtube:
  client: "web_creator"
  skip_premium_only: false
  extract_chapters: true
```

### 3. Brave 浏览器 Cookie 位置

在 macOS 上，Brave 浏览器的 cookie 存储在：
```
~/Library/Application Support/BraveSoftware/Brave-Browser/Default/
```

确保你已经在 Brave 浏览器中登录 YouTube 账户并拥有相关频道的会员资格。

## 使用示例

### 1. 基本使用

```bash
# 下载 1080p 视频
u2b_download download -q hd1080 "https://www.youtube.com/watch?v=VIDEO_ID"

# 使用详细输出
u2b_download download -v -q hd1080 "https://www.youtube.com/watch?v=VIDEO_ID"
```
### 2. 指定下载目录

```bash
u2b_download download -q hd1080 -o "./my_downloads" "VIDEO_URL"
```

### 3. 验证认证设置

```bash
u2b_download config validate
```

## 故障排除


### 1. 认证失败

```bash
# 验证认证设置
u2b_download config validate

# 检查浏览器是否已登录 YouTube
# 确保在 Brave 中已登录并有会员权限
```

### 2. 网络问题

```bash
# 使用代理 (如果需要)
u2b_download download --proxy "http://proxy:port" -q hd1080 "VIDEO_URL"
```

## 支持和反馈

如有问题，请：
1. 查看日志输出 (使用 `-v` 参数)
2. 检查配置文件是否正确
3. 确保 yt-dlp 为最新版本
4. 在 GitHub 项目页面提交 Issue