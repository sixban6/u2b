const HOST_NAME = "com.sixban.u2b";

let nativePort = null;
let appConfig = {
  videoRootDir: "",
  audioRootDir: "",
  defaultVideoDir: "~/Movies/u2b",
  defaultAudioDir: "~/Music/u2b",
  defaultQuality: "hd1080",
  videoDirs: [
    { label: "视频默认 (u2b)", path: "~/Movies/u2b" }
  ],
  audioDirs: [
    { label: "音乐默认 (u2b)", path: "~/Music/u2b" }
  ]
};

function formatQualityLabel(q) {
  switch ((q || "hd1080").toLowerCase()) {
    case "best":
      return "原画最高";
    case "hd2160":
    case "4k":
    case "2160":
    case "2160p":
      return "原画 4K";
    case "hd1440":
    case "2k":
    case "1440":
    case "1440p":
      return "2K";
    case "hd720":
    case "720":
    case "720p":
      return "720p";
    case "sd480":
    case "480":
    case "480p":
      return "480p";
    default:
      return "1080p";
  }
}

// Map of taskId -> task state
const activeTasks = new Map();
const pendingCallbacks = new Map();

function ensureNativePort() {
  if (nativePort) {
    return nativePort;
  }

  try {
    nativePort = chrome.runtime.connectNative(HOST_NAME);
  } catch (err) {
    console.error("Failed to connect to native host:", err);
    nativePort = null;
    return null;
  }

  nativePort.onMessage.addListener((msg) => {
    handleNativeMessage(msg);
  });

  nativePort.onDisconnect.addListener(() => {
    const err = chrome.runtime.lastError;
    if (err) {
      console.warn("Native host disconnected:", err.message);
    }
    nativePort = null;
    updateBadge();
  });

  return nativePort;
}

function sendNative(payload) {
  const port = ensureNativePort();
  if (!port) {
    return false;
  }
  try {
    port.postMessage(payload);
    return true;
  } catch (err) {
    console.error("Error posting to native host:", err);
    nativePort = null;
    return false;
  }
}

function handleNativeMessage(msg) {
  if (!msg || !msg.type) return;

  if (msg.type === "config") {
    if (msg.videoRootDir) appConfig.videoRootDir = msg.videoRootDir;
    if (msg.audioRootDir) appConfig.audioRootDir = msg.audioRootDir;
    if (msg.defaultVideoDir) appConfig.defaultVideoDir = msg.defaultVideoDir;
    if (msg.defaultAudioDir) appConfig.defaultAudioDir = msg.defaultAudioDir;
    if (msg.defaultQuality) appConfig.defaultQuality = msg.defaultQuality;
    if (Array.isArray(msg.videoDirs)) {
      appConfig.videoDirs = msg.videoDirs;
    }
    if (Array.isArray(msg.audioDirs)) {
      appConfig.audioDirs = msg.audioDirs;
    }
    chrome.storage.local.set({ u2bConfig: appConfig });
    rebuildContextMenus();
    broadcastToExtension({ type: "config_updated", config: appConfig });
    return;
  }

  if (msg.type === "category_created") {
    const cb = pendingCallbacks.get(msg.requestId);
    if (cb) {
      pendingCallbacks.delete(msg.requestId);
      cb(msg);
    }
    return;
  }

  if (msg.type === "folder_picked") {
    const cb = pendingCallbacks.get(msg.requestId);
    if (cb) {
      pendingCallbacks.delete(msg.requestId);
      cb(msg);
    }
    return;
  }

  if (msg.type === "progress" || msg.type === "started") {
    const existing = activeTasks.get(msg.id) || {};
    const updated = {
      ...existing,
      id: msg.id,
      url: msg.url || existing.url,
      title: msg.title || existing.title || "正在获取视频信息...",
      format: msg.format || existing.format || "mp4",
      outputDir: msg.outputDir || existing.outputDir || "",
      status: msg.status || "downloading",
      percentage: typeof msg.percentage === "number" ? msg.percentage : (existing.percentage || 0),
      speed: msg.speed || existing.speed || "",
      eta: msg.eta || existing.eta || "",
      tabId: existing.tabId,
      updatedAt: Date.now()
    };
    activeTasks.set(msg.id, updated);
    persistTasks();
    updateBadge();
    notifyTab(updated.tabId, { type: "task_update", task: updated });
    broadcastToExtension({ type: "task_update", task: updated });
    return;
  }

  if (msg.type === "complete") {
    const existing = activeTasks.get(msg.id) || {};
    const updated = {
      ...existing,
      id: msg.id,
      title: msg.title || existing.title || "下载完成",
      status: "completed",
      percentage: 100,
      filePath: msg.filePath || "",
      fileSize: msg.fileSize || 0,
      outputDir: msg.outputDir || existing.outputDir || "",
      updatedAt: Date.now()
    };
    activeTasks.set(msg.id, updated);
    persistTasks();
    updateBadge();
    notifyTab(updated.tabId, { type: "task_update", task: updated });
    broadcastToExtension({ type: "task_update", task: updated });
    return;
  }

  if (msg.type === "error") {
    const existing = activeTasks.get(msg.id) || {};
    const updated = {
      ...existing,
      id: msg.id,
      status: "error",
      error: msg.error || "下载失败",
      updatedAt: Date.now()
    };
    activeTasks.set(msg.id, updated);
    persistTasks();
    updateBadge();
    notifyTab(updated.tabId, { type: "task_update", task: updated });
    broadcastToExtension({ type: "task_update", task: updated });
    return;
  }
}

function persistTasks() {
  const list = Array.from(activeTasks.values())
    .sort((a, b) => (b.updatedAt || 0) - (a.updatedAt || 0))
    .slice(0, 30);
  chrome.storage.local.set({ u2bTasks: list });
}

function updateBadge() {
  const running = Array.from(activeTasks.values()).filter(
    (t) => t.status === "starting" || t.status === "downloading"
  );
  if (running.length === 0) {
    chrome.action.setBadgeText({ text: "" });
    return;
  }
  if (running.length === 1 && running[0].percentage > 0) {
    chrome.action.setBadgeBackgroundColor({ color: "#2563eb" });
    chrome.action.setBadgeText({ text: `${Math.round(running[0].percentage)}%` });
  } else {
    chrome.action.setBadgeBackgroundColor({ color: "#2563eb" });
    chrome.action.setBadgeText({ text: `${running.length}` });
  }
}

const SUPPORTED_DOC_PATTERNS = [
  "*://*.youtube.com/*",
  "*://youtu.be/*",
  "*://*.bilibili.com/*",
  "*://*.douyin.com/*",
  "*://*.kuaishou.com/*",
  "*://*.xiaohongshu.com/*",
  "*://*.weibo.com/*",
  "*://*.ixigua.com/*",
  "*://*.x.com/*",
  "*://*.twitter.com/*",
  "*://*.tiktok.com/*",
  "*://*.instagram.com/*",
  "*://*.twitch.tv/*",
  "*://*.vimeo.com/*"
];

async function ensureContentScriptInTab(tabId) {
  if (!tabId || !chrome.scripting) return false;
  try {
    await chrome.scripting.insertCSS({ target: { tabId }, files: ["content.css"] });
  } catch (_) {}
  try {
    await chrome.scripting.executeScript({ target: { tabId }, files: ["content.js"] });
    return true;
  } catch (_) {
    return false;
  }
}

async function sendToTabWithAutoInject(tabId, payload) {
  if (!tabId) return null;
  try {
    return await chrome.tabs.sendMessage(tabId, payload);
  } catch (_) {
    const injected = await ensureContentScriptInTab(tabId);
    if (!injected) return null;
    try {
      return await chrome.tabs.sendMessage(tabId, payload);
    } catch (_) {
      return null;
    }
  }
}

async function notifyTab(tabId, payload) {
  if (tabId) {
    const res = await sendToTabWithAutoInject(tabId, payload);
    if (res !== null) return;
  }
  // Fallback: if tabId was missing (e.g. Service Worker woke from sleep), notify current active supported tab
  try {
    const [activeTab] = await chrome.tabs.query({ active: true, currentWindow: true });
    if (activeTab && activeTab.id && activeTab.id !== tabId && isSupportedMediaPage(activeTab.url)) {
      await sendToTabWithAutoInject(activeTab.id, payload);
    }
  } catch (_) {}
}

async function injectContentIntoOpenTabs() {
  if (!chrome.scripting || !chrome.tabs) return;
  try {
    const tabs = await chrome.tabs.query({ url: SUPPORTED_DOC_PATTERNS });
    for (const t of tabs) {
      if (t && t.id) {
        ensureContentScriptInTab(t.id);
      }
    }
  } catch (_) {}
}

function broadcastToExtension(payload) {
  chrome.runtime.sendMessage(payload).catch(() => {});
}

let rebuildTimer = null;
function rebuildContextMenus() {
  if (rebuildTimer) clearTimeout(rebuildTimer);
  rebuildTimer = setTimeout(() => {
    chrome.contextMenus.removeAll(() => {
      const contexts = ["page", "link", "video", "image"];
      const docPatterns = SUPPORTED_DOC_PATTERNS;

      chrome.contextMenus.create({
        id: "u2b-root",
        title: "📥 u2b 下载",
        contexts,
        documentUrlPatterns: docPatterns
      });

      chrome.contextMenus.create({
        id: "u2b-quick-video",
        parentId: "u2b-root",
        title: `⚡ 快速下载视频 (${formatQualityLabel(appConfig.defaultQuality)} → ${shortenPath(appConfig.defaultVideoDir)})`,
        contexts,
        documentUrlPatterns: docPatterns
      });

      chrome.contextMenus.create({
        id: "u2b-quick-mp3",
        parentId: "u2b-root",
        title: `🎵 快速下载 MP3 (最佳音质 → ${shortenPath(appConfig.defaultAudioDir)})`,
        contexts,
        documentUrlPatterns: docPatterns
      });

      chrome.contextMenus.create({
        id: "u2b-custom-dialog",
        parentId: "u2b-root",
        title: "✏️ 指定文件名 / 目录下载...",
        contexts,
        documentUrlPatterns: docPatterns
      });

      chrome.contextMenus.create({
        id: "u2b-sep-1",
        parentId: "u2b-root",
        type: "separator",
        contexts,
        documentUrlPatterns: docPatterns
      });

      // Video directory submenu
      chrome.contextMenus.create({
        id: "u2b-video-dirs",
        parentId: "u2b-root",
        title: "📂 下载视频到分类目录",
        contexts,
        documentUrlPatterns: docPatterns
      });

      (appConfig.videoDirs || []).forEach((dir, idx) => {
        chrome.contextMenus.create({
          id: `u2b-vdir-${idx}`,
          parentId: "u2b-video-dirs",
          title: `📁 ${dir.label} (${shortenPath(dir.path)})`,
          contexts,
          documentUrlPatterns: docPatterns
        });
      });

      chrome.contextMenus.create({
        id: "u2b-vdir-browse",
        parentId: "u2b-video-dirs",
        title: "🔍 浏览选择其它文件夹...",
        contexts,
        documentUrlPatterns: docPatterns
      });

      // MP3 directory submenu
      chrome.contextMenus.create({
        id: "u2b-audio-dirs",
        parentId: "u2b-root",
        title: "🎧 下载 MP3 到分类目录",
        contexts,
        documentUrlPatterns: docPatterns
      });

      (appConfig.audioDirs || []).forEach((dir, idx) => {
        chrome.contextMenus.create({
          id: `u2b-adir-${idx}`,
          parentId: "u2b-audio-dirs",
          title: `🎵 ${dir.label} (${shortenPath(dir.path)})`,
          contexts,
          documentUrlPatterns: docPatterns
        });
      });

      chrome.contextMenus.create({
        id: "u2b-adir-browse",
        parentId: "u2b-audio-dirs",
        title: "🔍 浏览选择其它文件夹...",
        contexts,
        documentUrlPatterns: docPatterns
      });

      chrome.contextMenus.create({
        id: "u2b-sep-2",
        parentId: "u2b-root",
        type: "separator",
        contexts,
        documentUrlPatterns: docPatterns
      });

      chrome.contextMenus.create({
        id: "u2b-playlist-video",
        parentId: "u2b-root",
        title: "📚 下载整个播放列表 / 合集 (视频)",
        contexts,
        documentUrlPatterns: docPatterns
      });

      chrome.contextMenus.create({
        id: "u2b-playlist-mp3",
        parentId: "u2b-root",
        title: "📚 下载整个播放列表 / 合集 (MP3)",
        contexts,
        documentUrlPatterns: docPatterns
      });
    });
  }, 50);
}

function shortenPath(p) {
  if (!p) return "";
  const parts = p.split("/").filter(Boolean);
  if (parts.length <= 2) return p;
  return ".../" + parts.slice(-2).join("/");
}

async function resolveTargetVideo(info, tab) {
  let url = "";
  let title = "";

  if (info.linkUrl && isSupportedMediaUrl(info.linkUrl)) {
    url = info.linkUrl;
  }

  if (tab && tab.id) {
    const ctx = await sendToTabWithAutoInject(tab.id, { type: "get_context_target" });
    if (ctx) {
      if (!url && ctx.url && isSupportedMediaUrl(ctx.url)) {
        url = ctx.url;
      }
      if (ctx.title) {
        title = ctx.title;
      }
    }
  }

  if (!url && info.pageUrl && isSupportedMediaUrl(info.pageUrl)) {
    url = info.pageUrl;
  }
  if (!url && tab && tab.url && isSupportedMediaUrl(tab.url)) {
    url = tab.url;
  }
  if (!title && tab && tab.title) {
    title = tab.title
      .replace(/\s*-\s*YouTube$/i, "")
      .replace(/_哔哩哔哩_bilibili$/i, "")
      .replace(/\s*-\s*抖音$/i, "")
      .trim();
  }

  return { url, title };
}

function isSupportedMediaPage(u) {
  if (!u || !/^https?:\/\//i.test(u)) return false;
  try {
    const host = new URL(u).hostname.toLowerCase();
    const roots = [
      "youtube.com", "youtu.be",
      "bilibili.com", "b23.tv",
      "douyin.com", "kuaishou.com", "xiaohongshu.com", "weibo.com", "ixigua.com",
      "x.com", "twitter.com", "tiktok.com", "instagram.com", "twitch.tv", "vimeo.com"
    ];
    return roots.some((d) => host === d || host.endsWith("." + d));
  } catch (_) {
    return false;
  }
}

function isSupportedMediaUrl(u) {
  if (!isSupportedMediaPage(u)) return false;
  if (u.includes("youtube.com/") || u.includes("youtu.be/")) {
    return (
      u.includes("youtube.com/watch") ||
      u.includes("youtube.com/shorts/") ||
      u.includes("youtube.com/playlist") ||
      u.includes("youtu.be/")
    );
  }
  return true;
}

async function getYouTubeCookiesNetscape() {
  if (!chrome.cookies || !chrome.cookies.getAll) return "";
  try {
    const cookies = await chrome.cookies.getAll({ domain: "youtube.com" });
    if (!cookies || !cookies.length) return "";
    const lines = ["# Netscape HTTP Cookie File", "# Exported automatically by u2b extension"];
    for (const c of cookies) {
      let domain = c.domain || ".youtube.com";
      const flag = domain.startsWith(".") ? "TRUE" : "FALSE";
      const path = c.path || "/";
      const secure = c.secure ? "TRUE" : "FALSE";
      const expiration = c.expirationDate ? Math.round(c.expirationDate) : Math.round(Date.now() / 1000) + 86400 * 30;
      lines.push(`${domain}\t${flag}\t${path}\t${secure}\t${expiration}\t${c.name}\t${c.value}`);
    }
    return lines.join("\n");
  } catch (err) {
    console.warn("Could not export youtube cookies from extension:", err);
    return "";
  }
}

function startDownloadTask({ url, title, format, quality, outputDir, filename, isPlaylist, tabId }) {
  const id = "task-" + Date.now() + "-" + Math.random().toString(36).slice(2, 6);
  const task = {
    id,
    url,
    title: title || url,
    format: format || "mp4",
    quality: quality || appConfig.defaultQuality || "hd1080",
    outputDir: outputDir || (format === "mp3" ? appConfig.defaultAudioDir : appConfig.defaultVideoDir),
    filename: filename || "",
    isPlaylist: Boolean(isPlaylist),
    status: "starting",
    percentage: 0,
    speed: "",
    eta: "",
    tabId,
    updatedAt: Date.now()
  };

  activeTasks.set(id, task);
  persistTasks();
  updateBadge();
  notifyTab(tabId, { type: "task_update", task });
  broadcastToExtension({ type: "task_update", task });

  getYouTubeCookiesNetscape().then((cookiesStr) => {
    const ok = sendNative({
      type: "download",
      id: task.id,
      url: task.url,
      title: task.title,
      format: task.format,
      quality: task.quality,
      outputDir: task.outputDir,
      filename: task.filename,
      isPlaylist: task.isPlaylist,
      cookies: cookiesStr || ""
    });

    if (!ok) {
      task.status = "error";
      task.error = "无法连接本地 u2b 服务，请确认已运行 u2b ext install";
      activeTasks.set(id, task);
      persistTasks();
      updateBadge();
      notifyTab(tabId, { type: "task_update", task });
    }
  });

  return task;
}

function requestNativeFolderPicker(promptText) {
  return new Promise((resolve) => {
    const requestId = "pick-" + Date.now();
    pendingCallbacks.set(requestId, (res) => {
      resolve(res && res.path ? res.path : "");
    });
    const ok = sendNative({
      type: "pick_folder",
      requestId,
      prompt: promptText || "选择 u2b 下载保存目录"
    });
    if (!ok) {
      pendingCallbacks.delete(requestId);
      resolve("");
    }
  });
}

chrome.contextMenus.onClicked.addListener(async (info, tab) => {
  const target = await resolveTargetVideo(info, tab);
  const tabId = tab ? tab.id : null;

  if (!target.url) {
    if (tabId) {
      notifyTab(tabId, {
        type: "show_alert",
        message: "未检测到可下载的 YouTube 视频链接，请在视频封面或视频播放页上右键。"
      });
    }
    return;
  }

  const menuId = info.menuItemId;

  const defaultQuality = appConfig.defaultQuality || "hd1080";

  if (menuId === "u2b-quick-video") {
    startDownloadTask({
      url: target.url,
      title: target.title,
      format: "mp4",
      quality: defaultQuality,
      outputDir: appConfig.defaultVideoDir,
      tabId
    });
    return;
  }

  if (menuId === "u2b-quick-mp3") {
    startDownloadTask({
      url: target.url,
      title: target.title,
      format: "mp3",
      outputDir: appConfig.defaultAudioDir,
      tabId
    });
    return;
  }

  if (menuId === "u2b-custom-dialog") {
    if (tabId) {
      notifyTab(tabId, {
        type: "open_custom_dialog",
        url: target.url,
        title: target.title,
        config: appConfig
      });
    }
    return;
  }

  if (menuId.startsWith("u2b-vdir-")) {
    if (menuId === "u2b-vdir-browse") {
      const picked = await requestNativeFolderPicker("选择视频保存目录");
      if (picked) {
        startDownloadTask({
          url: target.url,
          title: target.title,
          format: "mp4",
          quality: defaultQuality,
          outputDir: picked,
          tabId
        });
      }
      return;
    }
    const idx = parseInt(menuId.replace("u2b-vdir-", ""), 10);
    const dir = (appConfig.videoDirs || [])[idx];
    if (dir) {
      startDownloadTask({
        url: target.url,
        title: target.title,
        format: "mp4",
        quality: defaultQuality,
        outputDir: dir.path,
        tabId
      });
    }
    return;
  }

  if (menuId.startsWith("u2b-adir-")) {
    if (menuId === "u2b-adir-browse") {
      const picked = await requestNativeFolderPicker("选择 MP3 音频保存目录");
      if (picked) {
        startDownloadTask({
          url: target.url,
          title: target.title,
          format: "mp3",
          outputDir: picked,
          tabId
        });
      }
      return;
    }
    const idx = parseInt(menuId.replace("u2b-adir-", ""), 10);
    const dir = (appConfig.audioDirs || [])[idx];
    if (dir) {
      startDownloadTask({
        url: target.url,
        title: target.title,
        format: "mp3",
        outputDir: dir.path,
        tabId
      });
    }
    return;
  }

  if (menuId === "u2b-playlist-video") {
    startDownloadTask({
      url: target.url,
      title: target.title || "播放列表下载",
      format: "mp4",
      quality: defaultQuality,
      outputDir: appConfig.defaultVideoDir,
      isPlaylist: true,
      tabId
    });
    return;
  }

  if (menuId === "u2b-playlist-mp3") {
    startDownloadTask({
      url: target.url,
      title: target.title || "播放列表 MP3 下载",
      format: "mp3",
      outputDir: appConfig.defaultAudioDir,
      isPlaylist: true,
      tabId
    });
  }
});

chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  if (!msg || !msg.type) return;

  if (msg.type === "get_state") {
    getYouTubeCookiesNetscape().then((cookies) => {
      sendNative({ type: "get_config", cookies: cookies || "" });
    });
    sendResponse({
      config: appConfig,
      tasks: Array.from(activeTasks.values()).sort((a, b) => (b.updatedAt || 0) - (a.updatedAt || 0))
    });
    return true;
  }

  if (msg.type === "start_download") {
    const tabId = sender && sender.tab ? sender.tab.id : msg.tabId;
    const task = startDownloadTask({
      url: msg.url,
      title: msg.title,
      format: msg.format,
      quality: msg.quality,
      outputDir: msg.outputDir,
      filename: msg.filename,
      isPlaylist: msg.isPlaylist,
      tabId
    });
    sendResponse({ ok: true, task });
    return true;
  }

  if (msg.type === "cancel_download") {
    sendNative({ type: "cancel", id: msg.id });
    const existing = activeTasks.get(msg.id);
    if (existing) {
      existing.status = "error";
      existing.error = "已取消下载";
      activeTasks.set(msg.id, existing);
      persistTasks();
      updateBadge();
      notifyTab(existing.tabId, { type: "task_update", task: existing });
    }
    sendResponse({ ok: true });
    return true;
  }

  if (msg.type === "reveal_in_finder") {
    sendNative({ type: "reveal_in_finder", filePath: msg.filePath });
    sendResponse({ ok: true });
    return true;
  }

  if (msg.type === "pick_folder") {
    requestNativeFolderPicker(msg.prompt).then((path) => {
      sendResponse({ path });
    });
    return true;
  }

  if (msg.type === "set_default_quality") {
    appConfig.defaultQuality = msg.quality || "hd1080";
    chrome.storage.local.set({ u2bConfig: appConfig });
    rebuildContextMenus();
    sendNative({ type: "set_default_quality", quality: appConfig.defaultQuality });
    broadcastToExtension({ type: "config_updated", config: appConfig });
    sendResponse({ ok: true, config: appConfig });
    return true;
  }

  if (msg.type === "save_presets") {
    if (msg.config) {
      appConfig = { ...appConfig, ...msg.config };
      chrome.storage.local.set({ u2bConfig: appConfig });
      rebuildContextMenus();
      sendNative({ type: "save_presets", config: appConfig });
    }
    sendResponse({ ok: true, config: appConfig });
    return true;
  }

  if (msg.type === "set_root_dir") {
    sendNative({
      type: "set_root_dir",
      mediaType: msg.mediaType,
      path: msg.path || ""
    });
    sendResponse({ ok: true });
    return true;
  }

  if (msg.type === "create_category") {
    const requestId = "cat-" + Date.now();
    pendingCallbacks.set(requestId, (res) => {
      sendResponse(res || {});
    });
    const ok = sendNative({
      type: "create_category",
      requestId,
      mediaType: msg.mediaType,
      name: msg.name
    });
    if (!ok) {
      pendingCallbacks.delete(requestId);
      sendResponse({ error: "无法连接本地 u2b" });
    }
    return true;
  }

  if (msg.type === "add_custom_dir") {
    sendNative({
      type: "add_custom_dir",
      mediaType: msg.mediaType,
      path: msg.path || "",
      name: msg.name || ""
    });
    sendResponse({ ok: true });
    return true;
  }

  if (msg.type === "remove_dir") {
    sendNative({
      type: "remove_dir",
      path: msg.path || ""
    });
    sendResponse({ ok: true });
    return true;
  }
});

// Initialize on startup / install
chrome.runtime.onInstalled.addListener(() => {
  chrome.storage.local.get(["u2bConfig", "u2bTasks"], (res) => {
    if (res.u2bConfig) {
      appConfig = { ...appConfig, ...res.u2bConfig };
    }
    if (Array.isArray(res.u2bTasks)) {
      res.u2bTasks.forEach((t) => activeTasks.set(t.id, t));
    }
    rebuildContextMenus();
    injectContentIntoOpenTabs();
    sendNative({ type: "get_config" });
  });
});

chrome.storage.local.get(["u2bConfig", "u2bTasks"], (res) => {
  if (res.u2bConfig) {
    appConfig = { ...appConfig, ...res.u2bConfig };
  }
  if (Array.isArray(res.u2bTasks)) {
    res.u2bTasks.forEach((t) => activeTasks.set(t.id, t));
  }
  rebuildContextMenus();
  getYouTubeCookiesNetscape().then((cookies) => {
    sendNative({ type: "get_config", cookies: cookies || "" });
  });
});
