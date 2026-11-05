(() => {
  let lastRightClickTarget = { url: "", title: "" };
  const toastElements = new Map();

  // Capture right-click context on YouTube cards, thumbnails, or player
  document.addEventListener(
    "contextmenu",
    (e) => {
      const target = e.target;
      if (!target || !(target instanceof Element)) {
        lastRightClickTarget = getCurrentPageVideoInfo();
        return;
      }

      // 1. Check closest anchor with watch/shorts/playlist href
      const link = target.closest('a[href*="/watch"], a[href*="/shorts/"], a[href*="/playlist"]');
      const card = target.closest(
        "ytd-rich-item-renderer, ytd-video-renderer, ytd-compact-video-renderer, ytd-grid-video-renderer, ytd-playlist-video-renderer, ytd-reel-item-renderer, ytm-shorts-lockup-view-model"
      );

      let url = "";
      let title = "";

      if (link && link.href) {
        url = normalizeYouTubeUrl(link.href);
      } else if (card) {
        const cardLink = card.querySelector('a#thumbnail[href], a#video-title-link[href], a#video-title[href], a[href*="/watch"], a[href*="/shorts/"]');
        if (cardLink && cardLink.href) {
          url = normalizeYouTubeUrl(cardLink.href);
        }
      }

      if (card) {
        const titleEl = card.querySelector("#video-title, #video-title-link, h3 a, span#video-title");
        if (titleEl) {
          title = (titleEl.getAttribute("title") || titleEl.textContent || "").trim();
        }
      }

      if (!url) {
        const pageInfo = getCurrentPageVideoInfo();
        url = pageInfo.url;
        if (!title) title = pageInfo.title;
      } else if (!title && link) {
        title = (link.getAttribute("title") || link.textContent || "").trim();
      }

      lastRightClickTarget = { url, title };
    },
    true
  );

  function normalizeYouTubeUrl(raw) {
    try {
      const u = new URL(raw, window.location.origin);
      if (u.pathname === "/watch" && u.searchParams.get("v")) {
        const clean = new URL("https://www.youtube.com/watch");
        clean.searchParams.set("v", u.searchParams.get("v"));
        if (u.searchParams.get("list")) {
          clean.searchParams.set("list", u.searchParams.get("list"));
        }
        return clean.toString();
      }
      return u.toString();
    } catch (_) {
      return raw;
    }
  }

  function getCurrentPageVideoInfo() {
    const href = window.location.href;
    let title = "";
    const h1 = document.querySelector("ytd-watch-metadata h1 yt-formatted-string, #title h1 yt-formatted-string");
    if (h1 && h1.textContent) {
      title = h1.textContent.trim();
    } else {
      title = document.title.replace(/^\(\d+\)\s*/, "").replace(/\s*-\s*YouTube$/i, "").trim();
    }
    return {
      url: isYouTubeMediaUrl(href) ? normalizeYouTubeUrl(href) : "",
      title
    };
  }

  function isYouTubeMediaUrl(u) {
    return (
      u.includes("youtube.com/watch") ||
      u.includes("youtube.com/shorts/") ||
      u.includes("youtube.com/playlist")
    );
  }

  function ensureToastContainer() {
    let container = document.getElementById("u2b-toast-container");
    if (!container) {
      container = document.createElement("div");
      container.id = "u2b-toast-container";
      document.body.appendChild(container);
    }
    return container;
  }

  function formatBytes(bytes) {
    if (!bytes || bytes <= 0) return "";
    const mb = bytes / (1024 * 1024);
    if (mb >= 1024) {
      return `${(mb / 1024).toFixed(2)} GB`;
    }
    return `${mb.toFixed(1)} MB`;
  }

  function formatQualityLabel(q) {
    switch ((q || "hd1080").toLowerCase()) {
      case "best":
        return "原画最高";
      case "hd2160":
      case "4k":
      case "2160":
        return "原画 4K";
      case "hd1440":
      case "2k":
      case "1440":
        return "2K";
      case "hd720":
      case "720":
        return "720p";
      case "sd480":
      case "480":
        return "480p";
      default:
        return "1080p";
    }
  }

  function renderTaskToast(task) {
    if (!task || !task.id) return;
    const container = ensureToastContainer();
    let el = toastElements.get(task.id);

    if (!el) {
      el = document.createElement("div");
      el.className = "u2b-toast";
      container.appendChild(el);
      toastElements.set(task.id, el);
    }

    const isMP3 = task.format === "mp3";
    const badgeText = isMP3 ? "MP3 音频" : formatQualityLabel(task.quality);
    const folderName = task.outputDir ? task.outputDir.split("/").filter(Boolean).slice(-2).join("/") : "默认目录";
    const pct = Math.min(100, Math.max(0, Number(task.percentage || 0)));

    if (task.status === "starting" || task.status === "downloading") {
      const statusText =
        task.status === "starting" || pct === 0
          ? "正在解析视频与音轨..."
          : `${pct.toFixed(1)}%${task.speed ? " · " + task.speed : ""}${task.eta ? " · 剩余 " + task.eta : ""}`;

      el.className = "u2b-toast u2b-toast-running";
      el.innerHTML = `
        <div class="u2b-toast-header">
          <span class="u2b-badge ${isMP3 ? "u2b-badge-mp3" : "u2b-badge-video"}">${escapeHtml(badgeText)}</span>
          <span class="u2b-folder-tag" title="${escapeHtml(task.outputDir || "")}">📁 ${escapeHtml(folderName)}</span>
          <button class="u2b-close-btn" data-action="cancel" title="取消下载">×</button>
        </div>
        <div class="u2b-toast-title">${escapeHtml(task.filename || task.title || task.url)}</div>
        <div class="u2b-progress-track">
          <div class="u2b-progress-bar ${pct === 0 ? "u2b-indeterminate" : ""}" style="width: ${pct === 0 ? 35 : pct}%"></div>
        </div>
        <div class="u2b-toast-meta">${escapeHtml(statusText)}</div>
      `;
    } else if (task.status === "completed") {
      const sizeStr = formatBytes(task.fileSize);
      el.className = "u2b-toast u2b-toast-success";
      el.innerHTML = `
        <div class="u2b-toast-header">
          <span class="u2b-badge u2b-badge-done">✅ 下载完成 ${sizeStr ? "(" + escapeHtml(sizeStr) + ")" : ""}</span>
          <span class="u2b-folder-tag">📁 ${escapeHtml(folderName)}</span>
          <button class="u2b-close-btn" data-action="dismiss" title="关闭">×</button>
        </div>
        <div class="u2b-toast-title">${escapeHtml(task.filename || task.title || "已完成下载")}</div>
        <div class="u2b-toast-actions">
          <button class="u2b-action-btn" data-action="reveal">📂 在 Finder 中显示</button>
        </div>
      `;
      scheduleAutoDismiss(task.id, el, 12000);
    } else if (task.status === "error") {
      el.className = "u2b-toast u2b-toast-error";
      el.innerHTML = `
        <div class="u2b-toast-header">
          <span class="u2b-badge u2b-badge-err">⚠️ 下载失败</span>
          <button class="u2b-close-btn" data-action="dismiss" title="关闭">×</button>
        </div>
        <div class="u2b-toast-title">${escapeHtml(task.filename || task.title || task.url)}</div>
        <div class="u2b-toast-err-msg">${escapeHtml(task.error || "未知错误")}</div>
      `;
      scheduleAutoDismiss(task.id, el, 10000);
    }

    el.onclick = (e) => {
      const btn = e.target.closest("[data-action]");
      if (!btn) return;
      const action = btn.getAttribute("data-action");
      if (action === "cancel") {
        chrome.runtime.sendMessage({ type: "cancel_download", id: task.id });
        removeToast(task.id);
      } else if (action === "dismiss") {
        removeToast(task.id);
      } else if (action === "reveal") {
        chrome.runtime.sendMessage({
          type: "reveal_in_finder",
          filePath: task.filePath || task.outputDir
        });
      }
    };
  }

  function scheduleAutoDismiss(id, el, delayMs) {
    if (el._dismissTimer) clearTimeout(el._dismissTimer);
    el._dismissTimer = setTimeout(() => {
      if (!el.matches(":hover")) {
        removeToast(id);
      } else {
        scheduleAutoDismiss(id, el, 4000);
      }
    }, delayMs);
  }

  function removeToast(id) {
    const el = toastElements.get(id);
    if (el) {
      el.remove();
      toastElements.delete(id);
    }
  }

  function openCustomDownloadDialog({ url, title, config }) {
    const existing = document.getElementById("u2b-modal-backdrop");
    if (existing) existing.remove();

    const cleanDefaultName = (title || "")
      .replace(/[\/\\:*?"<>|]/g, " ")
      .replace(/\s+/g, " ")
      .trim();

    const backdrop = document.createElement("div");
    backdrop.id = "u2b-modal-backdrop";

    let selectedFormat = "mp4";
    let selectedQuality = (config && config.defaultQuality) || "hd1080";
    if (selectedQuality === "best") selectedQuality = "hd2160";
    const getDirsForFormat = (fmt) =>
      fmt === "mp3" ? (config.audioDirs || []) : (config.videoDirs || []);
    const getDefaultDir = (fmt) =>
      fmt === "mp3" ? config.defaultAudioDir : config.defaultVideoDir;

    backdrop.innerHTML = `
      <div class="u2b-modal" role="dialog" aria-modal="true">
        <div class="u2b-modal-header">
          <div class="u2b-modal-title">📥 u2b 自定义下载</div>
          <button class="u2b-close-btn" id="u2b-modal-close">×</button>
        </div>

        <div class="u2b-form-group">
          <label>下载格式</label>
          <div class="u2b-seg-control">
            <button type="button" class="u2b-seg-btn active" data-fmt="mp4">🎬 视频 (MP4)</button>
            <button type="button" class="u2b-seg-btn" data-fmt="mp3">🎵 仅音频 (MP3)</button>
          </div>
        </div>

        <div class="u2b-form-group" id="u2b-quality-group">
          <label>视频画质（源视频不足所选档位时自动下载最高可用画质）</label>
          <div class="u2b-dir-chips" id="u2b-quality-chips">
            <button type="button" class="u2b-chip" data-q="hd2160">原画 4K</button>
            <button type="button" class="u2b-chip" data-q="hd1080">1080p 高清</button>
            <button type="button" class="u2b-chip" data-q="hd720">720p 均衡</button>
            <button type="button" class="u2b-chip" data-q="sd480">480p 省流</button>
          </div>
        </div>

        <div class="u2b-form-group">
          <label>保存文件名（可选，留空则使用默认模板）</label>
          <input type="text" id="u2b-input-filename" class="u2b-input" value="${escapeHtml(cleanDefaultName)}" placeholder="例如：Star Sky" />
        </div>

        <div class="u2b-form-group">
          <label>快捷分类目录（点击切换，或直接新建分类文件夹）</label>
          <div class="u2b-dir-chips" id="u2b-dir-chips"></div>
          <div class="u2b-new-cat-row" id="u2b-new-cat-row" style="display:none;">
            <input type="text" id="u2b-input-new-cat" class="u2b-input" placeholder="输入新分类名称（如：AI教程 / 刀郎）" />
            <button type="button" class="u2b-browse-btn" id="u2b-btn-create-cat">创建并选中</button>
          </div>
        </div>

        <div class="u2b-form-group">
          <label>保存目录路径</label>
          <div class="u2b-dir-row">
            <input type="text" id="u2b-input-dir" class="u2b-input" value="${escapeHtml(getDefaultDir("mp4"))}" />
            <button type="button" class="u2b-browse-btn" id="u2b-btn-browse">📁 浏览...</button>
          </div>
        </div>

        <div class="u2b-modal-footer">
          <button type="button" class="u2b-btn-secondary" id="u2b-btn-cancel">取消</button>
          <button type="button" class="u2b-btn-primary" id="u2b-btn-submit">立即下载 ↵</button>
        </div>
      </div>
    `;

    document.body.appendChild(backdrop);

    const filenameInput = backdrop.querySelector("#u2b-input-filename");
    const dirInput = backdrop.querySelector("#u2b-input-dir");
    const chipsContainer = backdrop.querySelector("#u2b-dir-chips");
    const newCatRow = backdrop.querySelector("#u2b-new-cat-row");
    const newCatInput = backdrop.querySelector("#u2b-input-new-cat");
    const createCatBtn = backdrop.querySelector("#u2b-btn-create-cat");

    function renderChips() {
      const dirs = getDirsForFormat(selectedFormat);
      const chipsHtml = dirs
        .map(
          (d) =>
            `<button type="button" class="u2b-chip ${dirInput.value === d.path ? "active" : ""}" data-path="${escapeHtml(d.path)}">${escapeHtml(d.label)}</button>`
        )
        .join("");

      chipsContainer.innerHTML =
        chipsHtml +
        `<button type="button" class="u2b-chip u2b-chip-add" id="u2b-chip-new-cat">+ 新建分类</button>`;

      chipsContainer.querySelectorAll(".u2b-chip[data-path]").forEach((chip) => {
        chip.onclick = () => {
          dirInput.value = chip.getAttribute("data-path");
          renderChips();
        };
      });

      const addChip = chipsContainer.querySelector("#u2b-chip-new-cat");
      if (addChip) {
        addChip.onclick = () => {
          const isHidden = newCatRow.style.display === "none";
          newCatRow.style.display = isHidden ? "flex" : "none";
          if (isHidden) {
            newCatInput.value = "";
            newCatInput.focus();
          }
        };
      }
    }

    function createNewCategory() {
      const catName = newCatInput.value.trim();
      if (!catName) return;
      const mediaType = selectedFormat === "mp3" ? "audio" : "video";
      chrome.runtime.sendMessage(
        { type: "create_category", mediaType, name: catName },
        (res) => {
          if (res && res.createdPath) {
            const list = getDirsForFormat(selectedFormat);
            if (!list.some((d) => d.path === res.createdPath)) {
              list.push({ label: res.name || catName, path: res.createdPath });
            }
            dirInput.value = res.createdPath;
            newCatRow.style.display = "none";
            renderChips();
          }
        }
      );
    }

    createCatBtn.onclick = createNewCategory;
    newCatInput.addEventListener("keydown", (e) => {
      if (e.key === "Enter") {
        e.preventDefault();
        e.stopPropagation();
        createNewCategory();
      }
    });

    renderChips();

    const qualityGroup = backdrop.querySelector("#u2b-quality-group");
    function renderQualityChips() {
      backdrop.querySelectorAll("#u2b-quality-chips .u2b-chip").forEach((chip) => {
        chip.classList.toggle("active", chip.getAttribute("data-q") === selectedQuality);
        chip.onclick = () => {
          selectedQuality = chip.getAttribute("data-q");
          renderQualityChips();
        };
      });
    }
    renderQualityChips();

    backdrop.querySelectorAll(".u2b-seg-btn").forEach((btn) => {
      btn.onclick = () => {
        backdrop.querySelectorAll(".u2b-seg-btn").forEach((b) => b.classList.remove("active"));
        btn.classList.add("active");
        selectedFormat = btn.getAttribute("data-fmt");
        qualityGroup.style.display = selectedFormat === "mp3" ? "none" : "";
        dirInput.value = getDefaultDir(selectedFormat);
        renderChips();
      };
    });

    backdrop.querySelector("#u2b-btn-browse").onclick = () => {
      chrome.runtime.sendMessage({ type: "pick_folder", prompt: "选择保存目录" }, (res) => {
        if (res && res.path) {
          dirInput.value = res.path;
          renderChips();
        }
      });
    };

    function closeModal() {
      backdrop.remove();
    }

    function submitDownload() {
      const filename = filenameInput.value.trim();
      const outputDir = dirInput.value.trim();
      chrome.runtime.sendMessage({
        type: "start_download",
        url,
        title: filename || title,
        format: selectedFormat,
        quality: selectedQuality,
        outputDir,
        filename
      });
      closeModal();
    }

    backdrop.querySelector("#u2b-modal-close").onclick = closeModal;
    backdrop.querySelector("#u2b-btn-cancel").onclick = closeModal;
    backdrop.querySelector("#u2b-btn-submit").onclick = submitDownload;
    backdrop.onclick = (e) => {
      if (e.target === backdrop) closeModal();
    };

    backdrop.addEventListener("keydown", (e) => {
      if (e.key === "Escape") {
        e.stopPropagation();
        closeModal();
      } else if (e.key === "Enter") {
        e.preventDefault();
        e.stopPropagation();
        submitDownload();
      }
    });

    setTimeout(() => {
      filenameInput.focus();
      filenameInput.select();
    }, 30);
  }

  function escapeHtml(str) {
    return String(str || "")
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
    if (!msg || !msg.type) return;

    if (msg.type === "get_context_target") {
      if (lastRightClickTarget && lastRightClickTarget.url) {
        sendResponse(lastRightClickTarget);
      } else {
        sendResponse(getCurrentPageVideoInfo());
      }
      return true;
    }

    if (msg.type === "task_update") {
      renderTaskToast(msg.task);
      return;
    }

    if (msg.type === "open_custom_dialog") {
      openCustomDownloadDialog(msg);
      return;
    }

    if (msg.type === "show_alert") {
      renderTaskToast({
        id: "alert-" + Date.now(),
        title: "提示",
        status: "error",
        error: msg.message
      });
    }
  });
})();
