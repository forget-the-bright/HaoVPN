// 页面脚本：index — 从模板内联迁出，配合 CSP script-src 'self'
    HaoVPN.setActiveNav('dashboard');

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

        var mon = await HaoVPN.api('/api/v1/monitor/online');
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
              '<td>' + (p.username || '—') + '</td>' +
              '<td class="text-mono">' + (p.vpn_ip || '') + '</td>' +
              '<td class="text-mono">' + (p.reconnect_count != null ? p.reconnect_count : '—') + '</td>' +
              '<td class="text-mono">' + ((p.allowed_ips || []).join(', ') || '—') + '</td>' +
              '<td class="text-mono">' + HaoVPN.formatBytes(p.rx_bytes) + ' / ' + HaoVPN.formatBytes(p.tx_bytes) + '</td>' +
              '<td class="text-mono">' + (p.remote_addr || '—') + '</td>';
            tbody.appendChild(tr);
          });
        }
      } catch (ex) {
        HaoVPN.toast(ex.message, 'error');
      }
    }

    loadDashboard();
    setInterval(loadDashboard, 8000);
  
