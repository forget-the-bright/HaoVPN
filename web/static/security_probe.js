// 页面脚本：security_probe — 探针封禁与安全事件；禁止模板 onclick=（CSP script-src 'self'）
    HaoVPN.setActiveNav('security');
    var offset = 0;

    // 手动封禁时长上限（10 年，与后端 probedefense.MaxBanDurationSec 一致）
    var MAX_BAN_SEC = 315360000;

    // 预设封禁时长（秒）；月=30 天、年=365 天，与 security-hardening 文档口径一致
    var BAN_PRESET_SEC = {
      '3600': 3600,
      '21600': 21600,
      '86400': 86400,
      '259200': 259200,
      '604800': 604800,
      '1209600': 1209600,
      '2592000': 2592000,
      '7776000': 7776000,
      '15552000': 15552000,
      '31536000': 31536000,
      '63072000': 63072000,
      '157680000': 157680000,
      '0': 0
    };

    function toggleBanCustomFields() {
      var preset = document.getElementById('banDurationPreset').value;
      var wrap = document.getElementById('banCustomWrap');
      if (preset === 'custom') {
        wrap.classList.remove('hidden');
      } else {
        wrap.classList.add('hidden');
      }
    }

    // 从表单解析封禁秒数；返回 { ok, sec, label } 供提交与 Toast 展示
    function resolveBanDurationSec() {
      var preset = document.getElementById('banDurationPreset').value;
      if (preset === 'custom') {
        var n = parseInt(document.getElementById('banCustomValue').value, 10);
        var unit = parseInt(document.getElementById('banCustomUnit').value, 10);
        if (!n || n < 1) {
          return { ok: false, msg: '自定义时长须为正整数' };
        }
        var sec = n * unit;
        if (sec < 60) {
          return { ok: false, msg: '封禁时长须至少 60 秒' };
        }
        if (sec > MAX_BAN_SEC) {
          return { ok: false, msg: '封禁时长不能超过 10 年' };
        }
        return { ok: true, sec: sec, label: n + ' × 单位' };
      }
      if (preset === '0') {
        return { ok: true, sec: 0, label: '永久' };
      }
      var secFixed = BAN_PRESET_SEC[preset];
      if (secFixed === undefined) {
        return { ok: false, msg: '请选择有效的封禁时长' };
      }
      var sel = document.getElementById('banDurationPreset');
      var label = sel.options[sel.selectedIndex] ? sel.options[sel.selectedIndex].text : secFixed + ' 秒';
      return { ok: true, sec: secFixed, label: label };
    }

    function formatBanExpiry(sec) {
      if (sec === 0) {
        return '永久';
      }
      return '约 ' + Math.round(sec / 86400) + ' 天后过期（以服务端写入为准）';
    }

    function scrollToBanForm(ip) {
      if (ip) {
        document.getElementById('banIP').value = ip;
      }
      var el = document.getElementById('banFormSection');
      if (el && el.scrollIntoView) {
        el.scrollIntoView({ behavior: 'smooth', block: 'start' });
      }
      document.getElementById('banIP').focus();
    }

    async function loadBlocks() {
      try {
        await HaoVPN.ensureDisplayTZ();
        await HaoVPN.refreshCSRF();
        var data = await HaoVPN.api('/api/v1/security/blocks?limit=100');
        var rows = data.items || [];
        var tb = document.getElementById('blocks');
        tb.innerHTML = '';
        if (!rows.length) {
          tb.innerHTML = '<tr><td colspan="8" class="text-muted">无生效封禁</td></tr>';
          return;
        }
        rows.forEach(function (b) {
          var tr = document.createElement('tr');
          var sig = b.signature_zh && b.signature
            ? (b.signature_zh + ' (' + b.signature + ')')
            : (b.signature_zh || b.signature || '');
          tr.innerHTML =
            '<td class="text-mono">' + (b.ip || '') + '</td>' +
            '<td>' + (b.source || '') + '</td>' +
            '<td>' + (b.reason || '') + '</td>' +
            '<td>' + sig + '</td>' +
            '<td>' + (b.hits || 0) + '</td>' +
            '<td class="text-mono">' + (b.created_at ? HaoVPN.formatTime(b.created_at) : '—') + '</td>' +
            '<td class="text-mono">' + (b.expires_at ? HaoVPN.formatTime(b.expires_at) : '永久') + '</td>' +
            '<td><button type="button" class="btn btn-ghost btn-sm" data-action="unban-ip" data-ip="' + (b.ip || '') + '">解封</button></td>';
          tb.appendChild(tr);
        });
      } catch (e) {
        HaoVPN.toast(e.message || String(e), 'error');
      }
    }

    async function banIP() {
      var ip = document.getElementById('banIP').value.trim();
      if (!ip) {
        HaoVPN.toast('请填写 IP', 'error');
        return;
      }
      var dur = resolveBanDurationSec();
      if (!dur.ok) {
        HaoVPN.toast(dur.msg, 'error');
        return;
      }
      try {
        await HaoVPN.refreshCSRF();
        await HaoVPN.api('/api/v1/security/blocks', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            ip: ip,
            reason: document.getElementById('banReason').value.trim(),
            duration_sec: dur.sec
          })
        });
        document.getElementById('banIP').value = '';
        HaoVPN.toast('已封禁 ' + ip + '（' + dur.label + '，' + formatBanExpiry(dur.sec) + '）', 'ok');
        loadBlocks();
      } catch (e) {
        HaoVPN.toast(e.message || String(e), 'error');
      }
    }

    async function unban(ip) {
      if (!confirm('解封 ' + ip + '？')) return;
      try {
        await HaoVPN.refreshCSRF();
        await HaoVPN.api('/api/v1/security/blocks/' + encodeURIComponent(ip), { method: 'DELETE' });
        HaoVPN.toast('已解封 ' + ip, 'ok');
        loadBlocks();
      } catch (e) {
        HaoVPN.toast(e.message || String(e), 'error');
      }
    }

    async function loadExempts() {
      try {
        await HaoVPN.ensureDisplayTZ();
        await HaoVPN.refreshCSRF();
        var data = await HaoVPN.api('/api/v1/security/exempts?limit=100');
        var rows = data.items || [];
        var tb = document.getElementById('exempts');
        tb.innerHTML = '';
        if (!rows.length) {
          tb.innerHTML = '<tr><td colspan="5" class="text-muted">无豁免条目</td></tr>';
          return;
        }
        rows.forEach(function (e) {
          var tr = document.createElement('tr');
          tr.innerHTML =
            '<td class="text-mono">' + (e.ip || '') + '</td>' +
            '<td>' + (e.note || '') + '</td>' +
            '<td>' + (e.source || '') + '</td>' +
            '<td class="text-mono">' + (e.created_at ? HaoVPN.formatTime(e.created_at) : '—') + '</td>' +
            '<td><button type="button" class="btn btn-ghost btn-sm" data-action="remove-exempt" data-ip="' + (e.ip || '') + '">移除</button></td>';
          tb.appendChild(tr);
        });
      } catch (e) {
        HaoVPN.toast(e.message || String(e), 'error');
      }
    }

    async function addExempt() {
      var ip = document.getElementById('exemptIP').value.trim();
      if (!ip) {
        HaoVPN.toast('请填写 IP 或 CIDR', 'error');
        return;
      }
      try {
        await HaoVPN.refreshCSRF();
        await HaoVPN.api('/api/v1/security/exempts', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            ip: ip,
            note: document.getElementById('exemptNote').value.trim()
          })
        });
        document.getElementById('exemptIP').value = '';
        HaoVPN.toast('已添加豁免 ' + ip, 'ok');
        loadExempts();
        loadBlocks();
      } catch (e) {
        HaoVPN.toast(e.message || String(e), 'error');
      }
    }

    async function removeExempt(ip) {
      if (!confirm('移除豁免 ' + ip + '？')) return;
      try {
        await HaoVPN.refreshCSRF();
        await HaoVPN.api('/api/v1/security/exempts/' + encodeURIComponent(ip), { method: 'DELETE' });
        HaoVPN.toast('已移除豁免 ' + ip, 'ok');
        loadExempts();
      } catch (e) {
        HaoVPN.toast(e.message || String(e), 'error');
      }
    }

    async function loadEvents(off) {
      if (off !== undefined) offset = off;
      try {
        await HaoVPN.ensureDisplayTZ();
        await HaoVPN.refreshCSRF();
        var q = HaoVPN.buildQuery({
          limit: 50,
          offset: offset,
          ip: document.getElementById('fIP').value.trim(),
          signature: document.getElementById('fSig').value.trim()
        });
        var data = await HaoVPN.api('/api/v1/security/events' + q);
        var rows = data.items || [];
        var tb = document.getElementById('events');
        tb.innerHTML = '';
        if (!rows.length) {
          tb.innerHTML = '<tr><td colspan="7" class="text-muted">无事件</td></tr>';
        }
        rows.forEach(function (e) {
          var tr = document.createElement('tr');
          var phase = e.phase_zh && e.phase ? (e.phase_zh + ' (' + e.phase + ')') : (e.phase_zh || e.phase || '');
          var sig = e.signature_zh && e.signature
            ? (e.signature_zh + ' (' + e.signature + ')')
            : (e.signature_zh || e.signature || '');
          var act = e.action_zh && e.action ? (e.action_zh + ' (' + e.action + ')') : (e.action_zh || e.action || '');
          var clientIP = e.client_ip || '';
          tr.innerHTML =
            '<td class="text-mono">' + HaoVPN.formatTime(e.created_at) + '</td>' +
            '<td class="text-mono">' + clientIP + '</td>' +
            '<td class="text-mono">' + (e.client_port || '') + '</td>' +
            '<td>' + phase + '</td>' +
            '<td><span class="badge badge-ok">' + sig + '</span></td>' +
            '<td>' + act + '</td>' +
            '<td><button type="button" class="btn btn-ghost btn-sm" data-action="ban-event-ip" data-ip="' + clientIP + '">封禁</button></td>';
          tb.appendChild(tr);
        });
        HaoVPN.renderPager(document.getElementById('pager'), data.total, 50, offset, loadEvents);
      } catch (e) {
        HaoVPN.toast(e.message || String(e), 'error');
      }
    }

    // 事件委托：解封 / 从事件行预填 IP（CSP 禁止 innerHTML 内 onclick=）
    document.getElementById('blocks').addEventListener('click', function (ev) {
      var btn = ev.target.closest('[data-action="unban-ip"]');
      if (!btn) return;
      var ip = btn.getAttribute('data-ip');
      if (ip) unban(ip);
    });
    document.getElementById('events').addEventListener('click', function (ev) {
      var btn = ev.target.closest('[data-action="ban-event-ip"]');
      if (!btn) return;
      var ip = btn.getAttribute('data-ip');
      if (ip) scrollToBanForm(ip);
    });

    document.getElementById('exempts').addEventListener('click', function (ev) {
      var btn = ev.target.closest('[data-action="remove-exempt"]');
      if (!btn) return;
      var ip = btn.getAttribute('data-ip');
      if (ip) removeExempt(ip);
    });

    document.getElementById('btnBanIP').addEventListener('click', function () { banIP(); });
    document.getElementById('btnRefreshBlocks').addEventListener('click', function () { loadBlocks(); });
    document.getElementById('btnAddExempt').addEventListener('click', function () { addExempt(); });
    document.getElementById('btnRefreshExempts').addEventListener('click', function () { loadExempts(); });
    document.getElementById('btnQueryEvents').addEventListener('click', function () { loadEvents(0); });
    document.getElementById('banDurationPreset').addEventListener('change', toggleBanCustomFields);

    toggleBanCustomFields();
    loadBlocks();
    loadExempts();
    loadEvents(0);
