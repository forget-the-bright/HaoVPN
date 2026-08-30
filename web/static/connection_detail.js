// 页面脚本：connection_detail — 从模板内联迁出，配合 CSP script-src 'self'
    HaoVPN.setActiveNav('connections');
    var evOffset = 0;
    var rowCache = {};

    function renderAccountRow(p) {
      var key = String(p.user_id);
      var html =
        '<td>' + (p.username || p.user_id) + '</td>' +
        '<td class="text-mono">' + (p.vpn_ip || '') + '</td>' +
        '<td>' + (p.online ? HaoVPN.statusBadge(true, '在线', '') : '<span class="badge badge-warn">离线</span>') + '</td>' +
        '<td class="text-mono">' + (p.reconnect_count != null ? p.reconnect_count : '—') + '</td>' +
        '<td class="text-mono">' + ((p.allowed_ips || []).join(', ') || '—') + '</td>' +
        '<td class="text-mono">' + HaoVPN.formatTime(p.connected_at) + '</td>' +
        '<td class="text-mono">' + HaoVPN.formatTime(p.last_heartbeat) + '</td>' +
        '<td class="text-mono">' + HaoVPN.formatBytes(p.rx_bytes) + ' / ' + HaoVPN.formatBytes(p.tx_bytes) + '</td>' +
        '<td class="text-mono">' + (p.remote_addr || '') + '</td>';
      var tr = rowCache[key];
      if (!tr) {
        tr = document.createElement('tr');
        tr.dataset.key = key;
        rowCache[key] = tr;
      }
      if (tr.innerHTML !== html) tr.innerHTML = html;
      return tr;
    }

    async function loadAccounts() {
      await HaoVPN.ensureDisplayTZ();
      await HaoVPN.refreshCSRF();
      var q = HaoVPN.buildQuery({
        q: document.getElementById('fQ').value.trim(),
        online: document.getElementById('fOnline').value
      });
      var all = await HaoVPN.api('/api/v1/monitor/accounts' + q);
      var tb = document.getElementById('list');
      var items = all.items || [];
      var seen = {};
      items.forEach(function (p) {
        seen[String(p.user_id)] = true;
        var tr = renderAccountRow(p);
        if (tr.parentNode !== tb) tb.appendChild(tr);
      });
      Array.from(tb.children).forEach(function (tr) {
        if (!seen[tr.dataset.key]) tr.remove();
      });
      if (!items.length) {
        tb.innerHTML = '<tr><td colspan="9" class="text-muted">无匹配账号</td></tr>';
        rowCache = {};
      }
    }

    async function loadEvents(off) {
      if (off !== undefined) evOffset = off;
      await HaoVPN.ensureDisplayTZ();
      await HaoVPN.refreshCSRF();
      var q = HaoVPN.buildQuery({
        limit: 50,
        offset: evOffset,
        event_type: document.getElementById('fEvent').value.trim()
      });
      var ev = await HaoVPN.api('/api/v1/monitor/events' + q);
      var tb = document.getElementById('events');
      tb.innerHTML = '';
      (ev.items || []).forEach(function (e) {
        var tr = document.createElement('tr');
        tr.innerHTML =
          '<td class="text-mono">' + HaoVPN.formatTime(e.created_at) + '</td>' +
          '<td>' + (e.username || e.user_id) + '</td>' +
          '<td><span class="badge badge-ok">' + (e.event_type || '') + '</span></td>' +
          '<td class="text-mono">' + (e.remote_addr || '') + '</td>';
        tb.appendChild(tr);
      });
      if (!(ev.items || []).length) {
        tb.innerHTML = '<tr><td colspan="4" class="text-muted">无事件</td></tr>';
      }
      HaoVPN.renderPager(document.getElementById('evPager'), ev.total, 50, evOffset, loadEvents);
    }

    async function load() {
      try {
        await loadAccounts();
        await loadEvents(evOffset);
      } catch (ex) {
        HaoVPN.toast(ex.message, 'error');
      }
    }

    document.getElementById('fQ').oninput = HaoVPN.debounce(loadAccounts, 400);
    document.getElementById('fOnline').onchange = loadAccounts;
    document.getElementById('fEvent').oninput = HaoVPN.debounce(function () { loadEvents(0); }, 400);

    load();
    setInterval(loadAccounts, 10000);
    setInterval(function () { loadEvents(evOffset); }, 30000);
  
