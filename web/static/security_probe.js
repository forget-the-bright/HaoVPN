// 页面脚本：security_probe — 从模板内联迁出，配合 CSP script-src 'self'
    HaoVPN.setActiveNav('security');
    var offset = 0;

    async function loadBlocks() {
      try {
        await HaoVPN.ensureDisplayTZ();
        await HaoVPN.refreshCSRF();
        var data = await HaoVPN.api('/api/v1/security/blocks?limit=100');
        var rows = data.items || [];
        var tb = document.getElementById('blocks');
        tb.innerHTML = '';
        if (!rows.length) {
          tb.innerHTML = '<tr><td colspan="7" class="text-muted">无生效封禁</td></tr>';
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
            '<td class="text-mono">' + (b.expires_at ? HaoVPN.formatTime(b.expires_at) : '永久') + '</td>' +
            '<td><button type="button" class="btn btn-ghost btn-sm" data-ip="' + b.ip + '">解封</button></td>';
          tr.querySelector('button').onclick = function () { unban(b.ip); };
          tb.appendChild(tr);
        });
      } catch (e) {
        alert(e.message || e);
      }
    }

    async function banIP() {
      var ip = document.getElementById('banIP').value.trim();
      if (!ip) { alert('请填写 IP'); return; }
      try {
        await HaoVPN.refreshCSRF();
        await HaoVPN.api('/api/v1/security/blocks', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ ip: ip, reason: document.getElementById('banReason').value.trim() })
        });
        document.getElementById('banIP').value = '';
        loadBlocks();
      } catch (e) {
        alert(e.message || e);
      }
    }

    async function unban(ip) {
      if (!confirm('解封 ' + ip + '？')) return;
      try {
        await HaoVPN.refreshCSRF();
        await HaoVPN.api('/api/v1/security/blocks/' + encodeURIComponent(ip), { method: 'DELETE' });
        loadBlocks();
      } catch (e) {
        alert(e.message || e);
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
          tb.innerHTML = '<tr><td colspan="6" class="text-muted">无事件</td></tr>';
        }
        rows.forEach(function (e) {
          var tr = document.createElement('tr');
          var phase = e.phase_zh && e.phase ? (e.phase_zh + ' (' + e.phase + ')') : (e.phase_zh || e.phase || '');
          var sig = e.signature_zh && e.signature
            ? (e.signature_zh + ' (' + e.signature + ')')
            : (e.signature_zh || e.signature || '');
          var act = e.action_zh && e.action ? (e.action_zh + ' (' + e.action + ')') : (e.action_zh || e.action || '');
          tr.innerHTML =
            '<td class="text-mono">' + HaoVPN.formatTime(e.created_at) + '</td>' +
            '<td class="text-mono">' + (e.client_ip || '') + '</td>' +
            '<td class="text-mono">' + (e.client_port || '') + '</td>' +
            '<td>' + phase + '</td>' +
            '<td><span class="badge badge-ok">' + sig + '</span></td>' +
            '<td>' + act + '</td>';
          tb.appendChild(tr);
        });
        HaoVPN.renderPager(document.getElementById('pager'), data.total, 50, offset, loadEvents);
      } catch (e) {
        alert(e.message || e);
      }
    }

    loadBlocks();
    loadEvents(0);
  
