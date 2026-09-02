// 页面脚本：index — 总览；在线账号分页
    HaoVPN.setActiveNav('dashboard');
    var onlineOffset = 0;

    async function loadOnline(off) {
      if (off !== undefined) onlineOffset = off;
      var limit = parseInt(document.getElementById('onlineLimit').value, 10) || 50;
      var mon = await HaoVPN.api('/api/v1/monitor/online' + HaoVPN.buildQuery({
        limit: limit, offset: onlineOffset
      }));
      var tbody = document.getElementById('accounts');
      tbody.innerHTML = '';
      var items = mon.items || [];
      if (!items.length) {
        tbody.innerHTML = '<tr><td colspan="7" class="text-muted">暂无在线账号</td></tr>';
      } else {
        items.forEach(function (p) {
          var tr = document.createElement('tr');
          tr.innerHTML =
            '<td class="text-mono">' + (p.user_id || '') + '</td>' +
            '<td class="cell-nowrap">' + (p.username || '—') + '</td>' +
            '<td class="text-mono">' + (p.vpn_ip || '') + '</td>' +
            '<td class="text-mono">' + (p.reconnect_count != null ? p.reconnect_count : '—') + '</td>' +
            '<td class="text-mono cell-ellipsis" title="' + escAttr((p.allowed_ips || []).join(', ')) + '">' +
              ((p.allowed_ips || []).join(', ') || '—') + '</td>' +
            '<td class="text-mono">' + HaoVPN.formatBytes(p.rx_bytes) + ' / ' + HaoVPN.formatBytes(p.tx_bytes) + '</td>' +
            '<td class="text-mono">' + (p.remote_addr || '—') + '</td>';
          tbody.appendChild(tr);
        });
      }
      HaoVPN.renderPager(document.getElementById('onlinePager'), mon.total != null ? mon.total : items.length, limit, onlineOffset, loadOnline);
    }

    function escAttr(s) {
      return String(s == null ? '' : s)
        .replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;');
    }

    async function loadDashboard() {
      try {
        await HaoVPN.refreshCSRF();
        var h = await HaoVPN.api('/api/v1/dashboard');
        document.getElementById('online').textContent = h.online_accounts ?? h.online_peers ?? '0';
        document.getElementById('uptime').textContent = HaoVPN.formatUptime(h.uptime_sec);
        document.getElementById('db').innerHTML = HaoVPN.statusBadge(h.db_ok);
        document.getElementById('tun').innerHTML = HaoVPN.statusBadge(h.tun_ok);
        document.getElementById('nat').innerHTML = HaoVPN.statusBadge(h.nat_ok);

        var errs = h.recent_errors || [];
        document.getElementById('errors').textContent = errs.length ? errs.slice(-12).join('\n') : '(无)';

        await loadOnline(onlineOffset);
      } catch (ex) {
        HaoVPN.toast(ex.message, 'error');
      }
    }

    document.getElementById('onlineLimit').onchange = function () { loadOnline(0); };
    document.getElementById('btnRefreshOnline').addEventListener('click', function () { loadOnline(onlineOffset); });

    loadDashboard();
    setInterval(loadDashboard, 8000);
