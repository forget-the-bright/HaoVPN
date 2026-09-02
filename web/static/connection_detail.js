// 页面脚本：connection_detail — 账号会话 / 流量明细 / 连接事件（均分页+可配条数）
    HaoVPN.setActiveNav('connections');
    var accOffset = 0;
    var evOffset = 0;
    var flowOffset = 0;
    var flowsExpanded = localStorage.getItem('haovpn_flows_expanded') === '1';
    var flowPollTimer = null;
    // 多列排序：[{key, dir}]，dir=asc|desc；空则服务端默认 last_seen desc
    var flowSortKeys = [];

    function syncFlowsUI() {
      var body = document.getElementById('flowsBody');
      var btn = document.getElementById('btnToggleFlows');
      if (flowsExpanded) {
        body.classList.remove('hidden');
        btn.textContent = '折叠';
        if (!flowPollTimer) {
          flowPollTimer = setInterval(function () { loadFlows(flowOffset); }, 10000);
        }
        loadFlows(flowOffset);
      } else {
        body.classList.add('hidden');
        btn.textContent = '展开';
        if (flowPollTimer) {
          clearInterval(flowPollTimer);
          flowPollTimer = null;
        }
      }
      localStorage.setItem('haovpn_flows_expanded', flowsExpanded ? '1' : '0');
    }

    // 表头三态：无 → 升 → 降 → 无；多列追加，角标显示优先级
    function syncFlowSortHeaders() {
      document.querySelectorAll('#flowsBody th.th-sortable').forEach(function (th) {
        var key = th.getAttribute('data-sort');
        th.classList.remove('sort-asc', 'sort-desc');
        var ind = th.querySelector('.sort-ind');
        var pri = th.querySelector('.sort-pri');
        if (pri) pri.remove();
        var idx = -1;
        for (var i = 0; i < flowSortKeys.length; i++) {
          if (flowSortKeys[i].key === key) { idx = i; break; }
        }
        if (idx < 0) {
          if (ind) ind.textContent = '↕';
          return;
        }
        var dir = flowSortKeys[idx].dir;
        th.classList.add(dir === 'asc' ? 'sort-asc' : 'sort-desc');
        if (ind) ind.textContent = dir === 'asc' ? '↑' : '↓';
        var badge = document.createElement('span');
        badge.className = 'sort-pri';
        badge.textContent = String(idx + 1);
        th.appendChild(badge);
      });
    }

    function toggleFlowSort(key) {
      var idx = -1;
      for (var i = 0; i < flowSortKeys.length; i++) {
        if (flowSortKeys[i].key === key) { idx = i; break; }
      }
      if (idx < 0) {
        flowSortKeys.push({ key: key, dir: 'asc' });
      } else if (flowSortKeys[idx].dir === 'asc') {
        flowSortKeys[idx].dir = 'desc';
      } else {
        flowSortKeys.splice(idx, 1);
      }
      syncFlowSortHeaders();
      loadFlows(0);
    }

    function flowSortQuery() {
      if (!flowSortKeys.length) return '';
      return flowSortKeys.map(function (s) { return s.key + ':' + s.dir; }).join(',');
    }

    async function loadAccounts(off) {
      if (off !== undefined) accOffset = off;
      await HaoVPN.ensureDisplayTZ();
      await HaoVPN.refreshCSRF();
      var limit = parseInt(document.getElementById('accLimit').value, 10) || 50;
      var q = HaoVPN.buildQuery({
        q: document.getElementById('fQ').value.trim(),
        online: document.getElementById('fOnline').value,
        limit: limit,
        offset: accOffset
      });
      var all = await HaoVPN.api('/api/v1/monitor/accounts' + q);
      var tb = document.getElementById('list');
      tb.innerHTML = '';
      var items = all.items || [];
      if (!items.length) {
        tb.innerHTML = '<tr><td colspan="10" class="text-muted">无匹配账号</td></tr>';
      } else {
        items.forEach(function (p) {
          var tr = document.createElement('tr');
          var prefixes = (p.allowed_ips || []).join(', ') || '—';
          tr.innerHTML =
            '<td class="cell-nowrap">' + (p.username || p.user_id) + '</td>' +
            '<td class="text-mono">' + (p.vpn_ip || '') + '</td>' +
            '<td>' + (p.online ? HaoVPN.statusBadge(true, '在线', '') : '<span class="badge badge-warn">离线</span>') + '</td>' +
            '<td class="text-mono">' + (p.reconnect_count != null ? p.reconnect_count : '—') + '</td>' +
            '<td class="text-mono cell-ellipsis" title="' + escAttr(prefixes) + '">' + prefixes + '</td>' +
            '<td class="text-mono">' + HaoVPN.formatTime(p.connected_at) + '</td>' +
            '<td class="text-mono">' + HaoVPN.formatTime(p.last_heartbeat) + '</td>' +
            '<td class="text-mono">' + HaoVPN.formatBytes(p.rx_bytes) + ' / ' + HaoVPN.formatBytes(p.tx_bytes) + '</td>' +
            '<td class="text-mono">' + (p.remote_addr || '') + '</td>' +
            '<td><button type="button" class="btn btn-ghost btn-sm">看流量</button></td>';
          tr.querySelector('button').onclick = function () {
            document.getElementById('fFlowUser').value = String(p.user_id || '');
            flowsExpanded = true;
            syncFlowsUI();
          };
          tb.appendChild(tr);
        });
      }
      HaoVPN.renderPager(document.getElementById('accPager'), all.total, limit, accOffset, loadAccounts);
    }

    async function loadFlows(off) {
      if (!flowsExpanded) return;
      if (off !== undefined) flowOffset = off;
      await HaoVPN.ensureDisplayTZ();
      await HaoVPN.refreshCSRF();
      var limit = parseInt(document.getElementById('flowLimit').value, 10) || 50;
      var uid = document.getElementById('fFlowUser').value.trim();
      var q = HaoVPN.buildQuery({
        limit: limit,
        offset: flowOffset,
        user_id: uid,
        proto: document.getElementById('fFlowProto').value,
        src_ip: document.getElementById('fFlowSrc').value.trim(),
        dst_ip: document.getElementById('fFlowDst').value.trim(),
        sort: flowSortQuery()
      });
      var data = await HaoVPN.api('/api/v1/monitor/flows' + q);
      var tb = document.getElementById('flowRows');
      tb.innerHTML = '';
      var items = data.items || [];
      if (!items.length) {
        tb.innerHTML = '<tr><td colspan="10" class="text-muted">暂无流（有流量后出现）</td></tr>';
      } else {
        items.forEach(function (f) {
          var tr = document.createElement('tr');
          tr.innerHTML =
            '<td class="cell-nowrap">' + (f.username || f.user_id) + '</td>' +
            '<td class="text-mono">' + (f.src_ip || '') + '</td>' +
            '<td class="text-mono">' + (f.dst_ip || '') + '</td>' +
            '<td>' + (f.proto_name || f.proto) + '</td>' +
            '<td class="text-mono">' + (f.sport || 0) + ' → ' + (f.dport || 0) + '</td>' +
            '<td class="text-mono">' + HaoVPN.formatBytes(f.bytes_in) + '</td>' +
            '<td class="text-mono">' + HaoVPN.formatBytes(f.bytes_out) + '</td>' +
            '<td class="text-mono">' + (f.packets_in || 0) + '</td>' +
            '<td class="text-mono">' + (f.packets_out || 0) + '</td>' +
            '<td class="text-mono">' + HaoVPN.formatTime(f.last_seen) + '</td>';
          tb.appendChild(tr);
        });
      }
      HaoVPN.renderPager(document.getElementById('flowPager'), data.total, limit, flowOffset, loadFlows);
    }

    async function loadEvents(off) {
      if (off !== undefined) evOffset = off;
      await HaoVPN.ensureDisplayTZ();
      await HaoVPN.refreshCSRF();
      var limit = parseInt(document.getElementById('evLimit').value, 10) || 50;
      var q = HaoVPN.buildQuery({
        limit: limit,
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
          '<td class="cell-nowrap">' + (e.username || e.user_id) + '</td>' +
          '<td><span class="badge badge-ok">' + (e.event_type || '') + '</span></td>' +
          '<td class="text-mono">' + (e.remote_addr || '') + '</td>';
        tb.appendChild(tr);
      });
      if (!(ev.items || []).length) {
        tb.innerHTML = '<tr><td colspan="4" class="text-muted">无事件</td></tr>';
      }
      HaoVPN.renderPager(document.getElementById('evPager'), ev.total, limit, evOffset, loadEvents);
    }

    function escAttr(s) {
      return String(s == null ? '' : s)
        .replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;');
    }

    document.getElementById('btnToggleFlows').addEventListener('click', function () {
      flowsExpanded = !flowsExpanded;
      syncFlowsUI();
    });
    document.getElementById('fQ').oninput = HaoVPN.debounce(function () { loadAccounts(0); }, 400);
    document.getElementById('fOnline').onchange = function () { loadAccounts(0); };
    document.getElementById('accLimit').onchange = function () { loadAccounts(0); };
    document.getElementById('fEvent').oninput = HaoVPN.debounce(function () { loadEvents(0); }, 400);
    document.getElementById('evLimit').onchange = function () { loadEvents(0); };
    document.getElementById('fFlowUser').oninput = HaoVPN.debounce(function () { loadFlows(0); }, 400);
    document.getElementById('fFlowSrc').oninput = HaoVPN.debounce(function () { loadFlows(0); }, 400);
    document.getElementById('fFlowDst').oninput = HaoVPN.debounce(function () { loadFlows(0); }, 400);
    document.getElementById('fFlowProto').onchange = function () { loadFlows(0); };
    document.getElementById('flowLimit').onchange = function () { loadFlows(0); };
    document.getElementById('btnRefreshAccounts').addEventListener('click', function () { loadAccounts(accOffset); });
    document.getElementById('btnRefreshConnEvents').addEventListener('click', function () { loadEvents(0); });
    document.getElementById('btnRefreshFlows').addEventListener('click', function () { loadFlows(flowOffset); });

    document.querySelectorAll('#flowsBody th.th-sortable').forEach(function (th) {
      th.addEventListener('click', function () {
        toggleFlowSort(th.getAttribute('data-sort'));
      });
    });
    syncFlowSortHeaders();

    loadAccounts(0);
    loadEvents(0);
    syncFlowsUI();
    setInterval(function () { loadAccounts(accOffset); }, 10000);
    setInterval(function () { loadEvents(evOffset); }, 30000);
