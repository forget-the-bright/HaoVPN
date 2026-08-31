/**
 * HaoVPN WebUI 共用脚本：CSRF、API、Toast、格式化、分页。
 * 各 templates/*.html 通过 window.HaoVPN 调用；无构建步骤，修改后须重编服务端 embed。
 */
(function () {
  'use strict';

  /** 当前页 CSRF token；由 refreshCSRF 或模板注入 setCSRF 设置 */
  let csrfToken = '';

  /** 右下角 Toast 提示；type 为 info|ok|error 等，对应 CSS class */
  function toast(msg, type) {
    let box = document.getElementById('toastContainer');
    if (!box) {
      box = document.createElement('div');
      box.id = 'toastContainer';
      box.className = 'toast-container';
      document.body.appendChild(box);
    }
    const el = document.createElement('div');
    el.className = 'toast toast-' + (type || 'info');
    el.textContent = msg;
    box.appendChild(el);
    setTimeout(function () { el.remove(); }, 4500);
  }

  /** 从 /api/v1/csrf 刷新 token；失败返回空串不抛错 */
  async function refreshCSRF() {
    try {
      const r = await fetch('/api/v1/csrf');
      if (!r.ok) return '';
      const j = await r.json();
      csrfToken = j.csrf_token || '';
      return csrfToken;
    } catch (_) {
      return '';
    }
  }

  /** 带 CSRF 的 fetch 封装；JSON 响应且 r.ok=false 时抛 Error(j.error) */
  async function api(path, opts) {
    opts = opts || {};
    opts.headers = opts.headers || {};
    if (csrfToken) opts.headers['X-CSRF-Token'] = csrfToken;
    const r = await fetch(path, opts);
    const ct = r.headers.get('content-type') || '';
    if (ct.includes('application/json')) {
      const j = await r.json();
      if (!r.ok && j.error) throw new Error(j.error);
      return j;
    }
    if (!r.ok) throw new Error('请求失败 ' + r.status);
    return r;
  }

  /** POST /api/v1/logout 后跳转登录页 */
  async function logout() {
    try {
      await api('/api/v1/logout', { method: 'POST' });
    } catch (_) { /* ignore */ }
    location.href = '/login';
  }

  /** 字节数人类可读（B/KB/MB/GB） */
  function formatBytes(n) {
    n = Number(n) || 0;
    if (n < 1024) return n + ' B';
    if (n < 1048576) return (n / 1024).toFixed(1) + ' KB';
    if (n < 1073741824) return (n / 1048576).toFixed(1) + ' MB';
    return (n / 1073741824).toFixed(2) + ' GB';
  }

  /** 秒数转 uptime 字符串（如 1h 2m） */
  function formatUptime(sec) {
    sec = Number(sec) || 0;
    const h = Math.floor(sec / 3600);
    const m = Math.floor((sec % 3600) / 60);
    const s = sec % 60;
    if (h > 0) return h + 'h ' + m + 'm';
    if (m > 0) return m + 'm ' + s + 's';
    return s + 's';
  }

  /** 展示时区相对 UTC 的分钟偏移（东为正）；由 /api/v1/system/info 填充 */
  let displayOffsetMin = 0;
  let displayTZName = 'UTC';
  const displayTZReady = (async function initDisplayTZ() {
    try {
      const r = await fetch('/api/v1/system/info');
      if (!r.ok) return;
      const j = await r.json();
      displayTZName = j.display_timezone || 'UTC';
      displayOffsetMin = parseOffsetMinutes(j.display_timezone_offset || 'Z');
    } catch (_) { /* 保持 UTC */ }
  })();

  /** 解析 +08:00 / Z 为分钟偏移 */
  function parseOffsetMinutes(off) {
    if (!off || off === 'Z' || off === 'z') return 0;
    const m = /^([+-])(\d{2}):(\d{2})$/.exec(String(off).trim());
    if (!m) return 0;
    const sign = m[1] === '-' ? -1 : 1;
    return sign * (parseInt(m[2], 10) * 60 + parseInt(m[3], 10));
  }

  function formatOffsetLabel(mins) {
    if (!mins) return 'Z';
    const sign = mins < 0 ? '-' : '+';
    const abs = Math.abs(mins);
    const h = Math.floor(abs / 60);
    const m = abs % 60;
    return sign + (h < 10 ? '0' : '') + h + ':' + (m < 10 ? '0' : '') + m;
  }

  /**
   * 将 API 返回的 UTC 时间串格式化为 api.display_timezone 展示串。
   * 支持 RFC3339（含 Z）与无后缀的「YYYY-MM-DD HH:MM:SS」（按 UTC 解析）。
   */
  function formatTime(s) {
    if (s == null || s === '') return '—';
    if (typeof s !== 'string') s = String(s);
    var ms = Date.parse(s);
    if (isNaN(ms) && s.indexOf('T') === -1 && s.length >= 19) {
      ms = Date.parse(s.replace(' ', 'T') + 'Z');
    }
    if (isNaN(ms)) return s;
    var shifted = new Date(ms + displayOffsetMin * 60000);
    function pad(n) { return n < 10 ? '0' + n : String(n); }
    var out =
      shifted.getUTCFullYear() + '-' +
      pad(shifted.getUTCMonth() + 1) + '-' +
      pad(shifted.getUTCDate()) + ' ' +
      pad(shifted.getUTCHours()) + ':' +
      pad(shifted.getUTCMinutes()) + ':' +
      pad(shifted.getUTCSeconds()) + ' ' +
      formatOffsetLabel(displayOffsetMin);
    return out;
  }

  /** 等待展示时区初始化（各页 load 前 await） */
  function ensureDisplayTZ() {
    return displayTZReady;
  }

  /** 生成 OK/异常 状态徽章 HTML 片段 */
  function statusBadge(ok, okText, failText) {
    return ok
      ? '<span class="badge badge-ok">' + (okText || '正常') + '</span>'
      : '<span class="badge badge-error">' + (failText || '异常') + '</span>';
  }

  /** 侧栏导航高亮：data-page 与当前 page 匹配项加 active */
  function setActiveNav(page) {
    document.querySelectorAll('.nav-item[data-page]').forEach(function (el) {
      el.classList.toggle('active', el.getAttribute('data-page') === page);
    });
  }

  /** 带 CSRF 的 POST 下载（备份/导出）；触发浏览器另存为 */
  async function downloadPost(path, filename) {
    await refreshCSRF();
    const headers = {};
    if (csrfToken) headers['X-CSRF-Token'] = csrfToken;
    const r = await fetch(path, { method: 'POST', credentials: 'same-origin', headers: headers });
    if (!r.ok) {
      const ct = r.headers.get('content-type') || '';
      if (ct.includes('application/json')) {
        const j = await r.json();
        throw new Error(j.error || ('下载失败 ' + r.status));
      }
      throw new Error('下载失败 ' + r.status);
    }
    const blob = await r.blob();
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = filename || 'download';
    a.click();
    URL.revokeObjectURL(a.href);
  }

  /**
   * 绑定 data-action 声明式控件（禁止 HTML onclick=：CSP script-src 'self' 会拦截内联事件）。
   * 当前支持：data-action="logout"。
   */
  function bindDataActions() {
    document.querySelectorAll('[data-action="logout"]').forEach(function (el) {
      el.addEventListener('click', function (e) {
        e.preventDefault();
        logout();
      });
    });
  }

  window.HaoVPN = {
    api: api,
    toast: toast,
    logout: logout,
    refreshCSRF: refreshCSRF,
    downloadPost: downloadPost,
    formatBytes: formatBytes,
    formatUptime: formatUptime,
    formatTime: formatTime,
    ensureDisplayTZ: ensureDisplayTZ,
    statusBadge: statusBadge,
    setActiveNav: setActiveNav,
    /** 读取当前内存中的 CSRF token（模板注入或 refreshCSRF 设置） */
    getCSRF: function () { return csrfToken; },
    /** 由服务端模板或登录响应注入 CSRF token */
    setCSRF: function (t) { csrfToken = t; },
    /** 将键值对象编码为 URL 查询串（跳过空值） */
    buildQuery: function (params) {
      var parts = [];
      Object.keys(params || {}).forEach(function (k) {
        var v = params[k];
        if (v === undefined || v === null || v === '') return;
        parts.push(encodeURIComponent(k) + '=' + encodeURIComponent(String(v)));
      });
      return parts.length ? '?' + parts.join('&') : '';
    },
    /** 渲染分页控件：总条数、上一页/下一页按钮，onChange(offset) 翻页回调 */
    renderPager: function (el, total, limit, offset, onChange) {
      if (!el) return;
      el.innerHTML = '';
      var pages = Math.max(1, Math.ceil((total || 0) / (limit || 50)));
      var cur = Math.floor((offset || 0) / (limit || 50)) + 1;
      var info = document.createElement('span');
      info.textContent = '共 ' + (total || 0) + ' 条 · 第 ' + cur + '/' + pages + ' 页';
      el.appendChild(info);
      function btn(label, disabled, nextOffset) {
        var b = document.createElement('button');
        b.type = 'button';
        b.className = 'btn btn-ghost btn-sm';
        b.textContent = label;
        b.disabled = !!disabled;
        // 用属性赋值而非 HTML onclick=，兼容 CSP
        b.onclick = function () { onChange(nextOffset); };
        el.appendChild(b);
      }
      btn('上一页', cur <= 1, Math.max(0, offset - limit));
      btn('下一页', cur >= pages, offset + limit);
    },
    /** 防抖包装：ms 毫秒内重复调用只执行最后一次 */
    debounce: function (fn, ms) {
      var t;
      return function () {
        var args = arguments;
        var self = this;
        clearTimeout(t);
        t = setTimeout(function () { fn.apply(self, args); }, ms || 300);
      };
    }
  };

  // app.js 置于 </body> 前加载，此时侧栏等 DOM 已就绪
  bindDataActions();
})();
