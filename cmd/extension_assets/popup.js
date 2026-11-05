let currentConfig = null;
let currentTasks = [];

const tabTasks = document.getElementById("tab-tasks");
const tabDirs = document.getElementById("tab-dirs");
const panelTasks = document.getElementById("panel-tasks");
const panelDirs = document.getElementById("panel-dirs");

tabTasks.onclick = () => {
  tabTasks.classList.add("active");
  tabDirs.classList.remove("active");
  panelTasks.classList.remove("hidden");
  panelDirs.classList.add("hidden");
};

tabDirs.onclick = () => {
  tabDirs.classList.add("active");
  tabTasks.classList.remove("active");
  panelDirs.classList.remove("hidden");
  panelTasks.classList.add("hidden");
};

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

function renderQualityBar() {
  const activeQ = (currentConfig && currentConfig.defaultQuality) || "hd1080";
  const normalizedBtnQ = activeQ === "best" ? "hd2160" : activeQ;
  document.querySelectorAll("#popup-quality-pills .q-pill").forEach((btn) => {
    btn.classList.toggle("active", btn.dataset.q === normalizedBtnQ);
  });
  const dlVideoBtn = document.getElementById("btn-dl-video");
  if (dlVideoBtn) {
    dlVideoBtn.textContent = `⚡ 下载当前页视频 (${formatQualityLabel(activeQ)})`;
  }
}

document.querySelectorAll("#popup-quality-pills .q-pill").forEach((btn) => {
  btn.onclick = () => {
    const q = btn.dataset.q;
    if (!currentConfig) currentConfig = {};
    currentConfig.defaultQuality = q;
    renderQualityBar();
    chrome.runtime.sendMessage({ type: "set_default_quality", quality: q });
  };
});

function renderTasks() {
  const container = document.getElementById("tasks-container");
  if (!currentTasks || currentTasks.length === 0) {
    container.innerHTML = `<div class="empty">暂无下载记录<br/>在 YouTube 视频上点击鼠标右键即可一键下载</div>`;
    return;
  }

  container.innerHTML = currentTasks
    .map((t) => {
      const pct = Math.min(100, Math.max(0, Number(t.percentage || 0)));
      let statusLabel = "下载中";
      if (t.status === "completed") statusLabel = "✅ 已完成";
      else if (t.status === "error") statusLabel = "⚠️ 失败";
      else if (pct > 0) statusLabel = `${pct.toFixed(1)}% · ${t.speed || ""}`;

      const typeTag = t.format === "mp3" ? "🎵 MP3" : `🎬 视频 ${formatQualityLabel(t.quality)}`;
      return `
        <div class="task-card">
          <div class="task-top">
            <span>${escapeHtml(typeTag)} · ${escapeHtml(statusLabel)}</span>
            ${
              t.status === "completed"
                ? `<button class="link-btn" data-reveal="${escapeHtml(t.filePath || t.outputDir)}">📂 Finder</button>`
                : ""
            }
          </div>
          <div class="task-name">${escapeHtml(t.filename || t.title || t.url)}</div>
          ${
            t.status === "starting" || t.status === "downloading"
              ? `<div class="bar-bg"><div class="bar-fill" style="width:${pct || 25}%"></div></div>`
              : ""
          }
          ${t.status === "error" ? `<div style="color:#fca5a5;font-size:11px;">${escapeHtml(t.error)}</div>` : ""}
        </div>
      `;
    })
    .join("");

  container.querySelectorAll("[data-reveal]").forEach((btn) => {
    btn.onclick = () => {
      chrome.runtime.sendMessage({
        type: "reveal_in_finder",
        filePath: btn.getAttribute("data-reveal")
      });
    };
  });
}

function renderDirs() {
  if (!currentConfig) return;

  const vRootEl = document.getElementById("video-root-path");
  const aRootEl = document.getElementById("audio-root-path");
  vRootEl.textContent = currentConfig.videoRootDir || currentConfig.defaultVideoDir || "";
  vRootEl.title = vRootEl.textContent;
  aRootEl.textContent = currentConfig.audioRootDir || currentConfig.defaultAudioDir || "";
  aRootEl.title = aRootEl.textContent;

  const vList = document.getElementById("video-dirs-list");
  const aList = document.getElementById("audio-dirs-list");

  vList.innerHTML = (currentConfig.videoDirs || [])
    .map(
      (d) => `
      <div class="dir-item">
        <div class="dir-info">
          <div class="dir-label">${escapeHtml(d.label)}</div>
          <div class="dir-path" title="${escapeHtml(d.path)}">${escapeHtml(d.path)}</div>
        </div>
        <button class="link-btn" data-open="${escapeHtml(d.path)}">打开</button>
        <button class="link-btn" data-remove-path="${escapeHtml(d.path)}" title="从右键菜单隐藏">×</button>
      </div>
    `
    )
    .join("");

  aList.innerHTML = (currentConfig.audioDirs || [])
    .map(
      (d) => `
      <div class="dir-item">
        <div class="dir-info">
          <div class="dir-label">${escapeHtml(d.label)}</div>
          <div class="dir-path" title="${escapeHtml(d.path)}">${escapeHtml(d.path)}</div>
        </div>
        <button class="link-btn" data-open="${escapeHtml(d.path)}">打开</button>
        <button class="link-btn" data-remove-path="${escapeHtml(d.path)}" title="从右键菜单隐藏">×</button>
      </div>
    `
    )
    .join("");

  document.querySelectorAll("[data-open]").forEach((btn) => {
    btn.onclick = () => {
      chrome.runtime.sendMessage({
        type: "reveal_in_finder",
        filePath: btn.getAttribute("data-open")
      });
    };
  });

  document.querySelectorAll("[data-remove-path]").forEach((btn) => {
    btn.onclick = () => {
      chrome.runtime.sendMessage({
        type: "remove_dir",
        path: btn.getAttribute("data-remove-path")
      });
    };
  });
}

document.getElementById("change-video-root").onclick = () => {
  chrome.runtime.sendMessage({ type: "set_root_dir", mediaType: "video" });
};

document.getElementById("change-audio-root").onclick = () => {
  chrome.runtime.sendMessage({ type: "set_root_dir", mediaType: "audio" });
};

document.getElementById("add-video-dir").onclick = () => {
  chrome.runtime.sendMessage({ type: "add_custom_dir", mediaType: "video" });
};

document.getElementById("add-audio-dir").onclick = () => {
  chrome.runtime.sendMessage({ type: "add_custom_dir", mediaType: "audio" });
};

const videoCatRow = document.getElementById("video-cat-row");
const videoCatInput = document.getElementById("video-cat-input");
document.getElementById("new-video-cat").onclick = () => {
  videoCatRow.classList.toggle("show");
  if (videoCatRow.classList.contains("show")) {
    videoCatInput.value = "";
    videoCatInput.focus();
  }
};

function submitNewCategory(mediaType, inputEl, rowEl) {
  const name = inputEl.value.trim();
  if (!name) return;
  chrome.runtime.sendMessage({ type: "create_category", mediaType, name }, () => {
    rowEl.classList.remove("show");
    inputEl.value = "";
  });
}

document.getElementById("video-cat-submit").onclick = () =>
  submitNewCategory("video", videoCatInput, videoCatRow);
videoCatInput.addEventListener("keydown", (e) => {
  if (e.key === "Enter") submitNewCategory("video", videoCatInput, videoCatRow);
});

const audioCatRow = document.getElementById("audio-cat-row");
const audioCatInput = document.getElementById("audio-cat-input");
document.getElementById("new-audio-cat").onclick = () => {
  audioCatRow.classList.toggle("show");
  if (audioCatRow.classList.contains("show")) {
    audioCatInput.value = "";
    audioCatInput.focus();
  }
};

document.getElementById("audio-cat-submit").onclick = () =>
  submitNewCategory("audio", audioCatInput, audioCatRow);
audioCatInput.addEventListener("keydown", (e) => {
  if (e.key === "Enter") submitNewCategory("audio", audioCatInput, audioCatRow);
});

async function triggerActiveTabDownload(format) {
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  if (!tab || !tab.url) return;
  const outputDir =
    format === "mp3"
      ? (currentConfig && currentConfig.defaultAudioDir) || ""
      : (currentConfig && currentConfig.defaultVideoDir) || "";
  const quality = (currentConfig && currentConfig.defaultQuality) || "hd1080";
  chrome.runtime.sendMessage({
    type: "start_download",
    url: tab.url,
    title: (tab.title || "").replace(/\s*-\s*YouTube$/i, "").trim(),
    format,
    quality,
    outputDir,
    tabId: tab.id
  });
}

document.getElementById("btn-dl-video").onclick = () => triggerActiveTabDownload("mp4");
document.getElementById("btn-dl-mp3").onclick = () => triggerActiveTabDownload("mp3");

function escapeHtml(str) {
  return String(str || "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

chrome.runtime.sendMessage({ type: "get_state" }, (res) => {
  if (res) {
    currentConfig = res.config;
    currentTasks = res.tasks || [];
    renderQualityBar();
    renderTasks();
    renderDirs();
  }
});

chrome.runtime.onMessage.addListener((msg) => {
  if (!msg) return;
  if (msg.type === "task_update" && msg.task) {
    const idx = currentTasks.findIndex((t) => t.id === msg.task.id);
    if (idx >= 0) currentTasks[idx] = msg.task;
    else currentTasks.unshift(msg.task);
    renderTasks();
  } else if (msg.type === "config_updated" && msg.config) {
    currentConfig = msg.config;
    renderQualityBar();
    renderDirs();
  }
});
