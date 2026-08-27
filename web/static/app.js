/**
 * HaoVPN WebUI 共用脚本：CSRF、API、Toast、格式化
 */
(function () {
  'use strict';

  let csrfToken = '';

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

  async function logout() {
    try {
      await api('/api/v1/logout', { method: 'POST' });
    } catch (_) { /* ignore */ }
    location.href = '/login';
  }

  function formatBytes(n) {
    n = Number(n) || 0;
    if (n < 1024) return n + ' B';
    if (n < 1048576) return (n / 1024).toFixed(1) + ' KB';
    if (n < 1073741824) return (n / 1048576).toFixed(1) + ' MB';
    return (n / 1073741824).toFixed(2) + ' GB';
  }

  function formatUptime(sec) {
    sec = Number(sec) || 0;
    const h = Math.floor(sec / 3600);
    const m = Math.floor((sec % 3600) / 60);
    const s = sec % 60;
    if (h > 0) return h + 'h ' + m + 'm';
    if (m > 0) return m + 'm ' + s + 's';
    return s + 's';
  }

  function statusBadge(ok, okText, failText) {
    return ok
      ? '<span class="badge badge-ok">' + (okText || '正常') + '</span>'
      : '<span class="badge badge-error">' + (failText || '异常') + '</span>';
  }

  function setActiveNav(page) {
    document.querySelectorAll('.nav-item[data-page]').forEach(function (el) {
      el.classList.toggle('active', el.getAttribute('data-page') === page);
    });
  }

  window.HaoVPN = {
    api: api,
    toast: toast,
    logout: logout,
    refreshCSRF: refreshCSRF,
    formatBytes: formatBytes,
    formatUptime: formatUptime,
    statusBadge: statusBadge,
    setActiveNav: setActiveNav,
    getCSRF: function () { return csrfToken; },
    setCSRF: function (t) { csrfToken = t; },
    buildQuery: function (params) {
      var parts = [];
      Object.keys(params || {}).forEach(function (k) {
        var v = params[k];
        if (v === undefined || v === null || v === '') return;
        parts.push(encodeURIComponent(k) + '=' + encodeURIComponent(String(v)));
      });
      return parts.length ? '?' + parts.join('&') : '';
    },
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
        b.onclick = function () { onChange(nextOffset); };
        el.appendChild(b);
      }
      btn('上一页', cur <= 1, Math.max(0, offset - limit));
      btn('下一页', cur >= pages, offset + limit);
    },
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
})();
