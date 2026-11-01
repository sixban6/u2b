package cmd

import (
	"fmt"
	"html"
)

// generateNavHTML creates a polished, high-aesthetic onboarding guide HTML
// tailored to the user's detected browser, extension directory, and OS.
func generateNavHTML(browserName, scheme, extDir, baseDir string) string {
	safeBrowser := html.EscapeString(browserName)
	safeScheme := html.EscapeString(scheme)
	safeExtDir := html.EscapeString(extDir)
	safeBaseDir := html.EscapeString(baseDir)

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>u2b 浏览器扩展启用指引</title>
  <style>
    :root {
      --bg: #0b0f19;
      --card-bg: rgba(18, 24, 38, 0.85);
      --card-border: rgba(255, 255, 255, 0.08);
      --primary: #3b82f6;
      --primary-hover: #2563eb;
      --accent: #8b5cf6;
      --success: #10b981;
      --success-bg: rgba(16, 185, 129, 0.12);
      --text-main: #f8fafc;
      --text-muted: #94a3b8;
      --code-bg: rgba(15, 23, 42, 0.9);
    }
    * {
      box-sizing: border-box;
      margin: 0;
      padding: 0;
    }
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif;
      background: radial-gradient(circle at top right, #1e1b4b 0%%, var(--bg) 60%%);
      color: var(--text-main);
      min-height: 100vh;
      display: flex;
      justify-content: center;
      padding: 40px 20px;
      line-height: 1.6;
    }
    .container {
      max-width: 780px;
      width: 100%%;
    }
    .header {
      text-align: center;
      margin-bottom: 36px;
    }
    .logo-badge {
      display: inline-flex;
      align-items: center;
      gap: 10px;
      background: linear-gradient(135deg, rgba(59, 130, 246, 0.2), rgba(139, 92, 246, 0.2));
      border: 1px solid rgba(139, 92, 246, 0.35);
      padding: 8px 18px;
      border-radius: 9999px;
      margin-bottom: 18px;
    }
    .logo-badge span {
      font-weight: 700;
      letter-spacing: 0.05em;
      background: linear-gradient(135deg, #60a5fa, #c084fc);
      -webkit-background-clip: text;
      -webkit-text-fill-color: transparent;
      font-size: 14px;
    }
    h1 {
      font-size: 28px;
      font-weight: 800;
      letter-spacing: -0.02em;
      margin-bottom: 10px;
    }
    .subtitle {
      color: var(--text-muted);
      font-size: 15px;
    }
    .card {
      background: var(--card-bg);
      backdrop-filter: blur(16px);
      border: 1px solid var(--card-border);
      border-radius: 16px;
      padding: 24px 28px;
      margin-bottom: 20px;
      box-shadow: 0 10px 30px rgba(0, 0, 0, 0.35);
      transition: transform 0.2s ease, border-color 0.2s ease;
    }
    .card:hover {
      border-color: rgba(99, 102, 241, 0.3);
    }
    .step-header {
      display: flex;
      align-items: center;
      gap: 14px;
      margin-bottom: 16px;
    }
    .step-num {
      width: 32px;
      height: 32px;
      border-radius: 10px;
      background: linear-gradient(135deg, var(--primary), var(--accent));
      color: #fff;
      display: flex;
      align-items: center;
      justify-content: center;
      font-weight: 700;
      font-size: 15px;
      flex-shrink: 0;
    }
    .step-title {
      font-size: 17px;
      font-weight: 700;
    }
    .badge-auto {
      margin-left: auto;
      font-size: 12px;
      font-weight: 600;
      background: var(--success-bg);
      color: var(--success);
      border: 1px solid rgba(16, 185, 129, 0.3);
      padding: 3px 10px;
      border-radius: 999px;
      display: inline-flex;
      align-items: center;
      gap: 5px;
    }
    .desc {
      color: var(--text-muted);
      font-size: 14px;
      margin-bottom: 16px;
    }
    .action-box {
      background: var(--code-bg);
      border: 1px solid var(--card-border);
      border-radius: 12px;
      padding: 12px 16px;
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
    }
    .action-text {
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
      font-size: 14px;
      color: #38bdf8;
      word-break: break-all;
      font-weight: 600;
    }
    .btn {
      background: var(--primary);
      color: #fff;
      border: none;
      padding: 8px 16px;
      border-radius: 8px;
      font-size: 13px;
      font-weight: 600;
      cursor: pointer;
      display: inline-flex;
      align-items: center;
      gap: 6px;
      white-space: nowrap;
      transition: all 0.2s ease;
    }
    .btn:hover {
      background: var(--primary-hover);
      transform: translateY(-1px);
    }
    .btn-secondary {
      background: rgba(255, 255, 255, 0.08);
      color: var(--text-main);
      border: 1px solid var(--card-border);
    }
    .btn-secondary:hover {
      background: rgba(255, 255, 255, 0.15);
    }
    .hint-callout {
      background: rgba(59, 130, 246, 0.08);
      border-left: 3px solid var(--primary);
      padding: 10px 14px;
      border-radius: 6px;
      font-size: 13px;
      color: #93c5fd;
      margin-top: 12px;
    }
    .toast {
      position: fixed;
      bottom: 24px;
      left: 50%%;
      transform: translateX(-50%%) translateY(100px);
      background: #1e293b;
      color: #fff;
      border: 1px solid var(--primary);
      padding: 10px 22px;
      border-radius: 999px;
      box-shadow: 0 10px 25px rgba(0, 0, 0, 0.5);
      font-size: 14px;
      font-weight: 600;
      transition: transform 0.3s cubic-bezier(0.18, 0.89, 0.32, 1.28);
      pointer-events: none;
      z-index: 999;
    }
    .toast.show {
      transform: translateX(-50%%) translateY(0);
    }
    .feature-grid {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 12px;
      margin-top: 14px;
    }
    .feature-item {
      background: rgba(255, 255, 255, 0.03);
      border: 1px solid rgba(255, 255, 255, 0.05);
      padding: 12px 14px;
      border-radius: 10px;
      font-size: 13px;
    }
    .feature-item strong {
      display: block;
      color: #e2e8f0;
      margin-bottom: 2px;
      font-size: 14px;
    }
  </style>
</head>
<body>
  <div class="container">
    <div class="header">
      <div class="logo-badge">
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" style="color: #60a5fa;"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path><polyline points="7 10 12 15 17 10"></polyline><line x1="12" y1="15" x2="12" y2="3"></line></svg>
        <span>u2b YouTube Downloader</span>
      </div>
      <h1>浏览器扩展 10 秒极速挂载指引</h1>
      <p class="subtitle">已为您自动配置好环境，跟着下方 2 个小步骤即可立即启用</p>
    </div>

    <!-- Step 1 -->
    <div class="card">
      <div class="step-header">
        <div class="step-num">1</div>
        <div class="step-title">打开扩展管理页（%s）</div>
      </div>
      <p class="desc">点击下方按钮复制地址，然后在浏览器新建标签页中粘贴并按回车打开：</p>
      <div class="action-box">
        <span class="action-text">%s</span>
        <button class="btn" onclick="copyText('%s', '扩展管理页地址已复制！请新开标签页粘贴打开')">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path></svg>
          一键复制地址
        </button>
      </div>
      <div class="hint-callout">
        💡 <strong>小提示</strong>：出于浏览器底层安全防护，Chromium 禁止外部程序直接静默跳入设置页，请复制后在地址栏回车一次即可。
      </div>
    </div>

    <!-- Step 2 -->
    <div class="card">
      <div class="step-header">
        <div class="step-num">2</div>
        <div class="step-title">右上角开发者模式</div>
        <div class="badge-auto">
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3"><polyline points="20 6 9 17 4 12"></polyline></svg>
          已由 u2b 自动为您启用
        </div>
      </div>
      <p class="desc">u2b 在安装时已自动为您在浏览器偏好中激活了<strong>「开发者模式 (Developer mode)」</strong>。进入扩展页面后，右上角的开关已呈打开状态，左上方的「加载已解压的扩展程序」按钮已就绪。</p>
    </div>

    <!-- Step 3 -->
    <div class="card">
      <div class="step-header">
        <div class="step-num">3</div>
        <div class="step-title">将高亮的 extension 文件夹拖入即可</div>
      </div>
      <p class="desc">此时系统文件管理器（访达/资源管理器）已自动在后台为您打开了 <code>%s</code> 目录，并且已经为您<strong>蓝色高亮选中了 <code>extension</code> 文件夹</strong>！</p>
      <div class="action-box" style="margin-bottom: 12px;">
        <span class="action-text" style="color: #cbd5e1; font-size: 13px;">📁 %s</span>
        <button class="btn btn-secondary" onclick="copyText('%s', '文件夹路径已复制到剪贴板！')">
          复制绝对路径
        </button>
      </div>
      <p class="desc" style="font-size: 13px; margin-bottom: 0;">
        👉 <strong>操作方法</strong>：直接按住已高亮选中的 <code>extension</code> 文件夹，拖到第 1 步打开的扩展页面中释放！<br>
        （也可以点击扩展页面的「加载已解压的扩展程序」按钮，粘贴上面的路径回车）
      </p>
    </div>

    <!-- Usage Guide -->
    <div class="card" style="border-color: rgba(59, 130, 246, 0.3);">
      <div class="step-header">
        <div class="step-num" style="background: linear-gradient(135deg, #10b981, #059669);">🎉</div>
        <div class="step-title">挂载完成后如何使用？</div>
      </div>
      <div class="feature-grid">
        <div class="feature-item">
          <strong>🖱️ 网页右键快捷下载</strong>
          在任何 YouTube 视频或播放列表页面，右键点击视频或页面任意空白处，即可直接选择下载 1080p 视频或提取纯音频 MP3。
        </div>
        <div class="feature-item">
          <strong>⚙️ 浏览器右上角控制台</strong>
          点击浏览器工具栏的 u2b 扩展图标，即可一键查看下载预设、自定义音视频偏好或刷新配置。
        </div>
      </div>
    </div>
  </div>

  <div id="toast" class="toast">已复制</div>

  <script>
    function copyText(text, msg) {
      navigator.clipboard.writeText(text).then(() => {
        showToast(msg || '已复制到剪贴板');
      }).catch(() => {
        const input = document.createElement('input');
        input.value = text;
        document.body.appendChild(input);
        input.select();
        document.execCommand('copy');
        document.body.removeChild(input);
        showToast(msg || '已复制到剪贴板');
      });
    }

    function showToast(msg) {
      const t = document.getElementById('toast');
      t.innerText = msg;
      t.classList.add('show');
      setTimeout(() => {
        t.classList.remove('show');
      }, 2500);
    }
  </script>
</body>
</html>
`, safeBrowser, safeScheme, safeScheme, safeBaseDir, safeExtDir, safeExtDir)
}
