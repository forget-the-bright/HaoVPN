// 页面脚本：audit_log — 从模板内联迁出，配合 CSP script-src 'self'
    HaoVPN.setActiveNav('audit');
    var offset = 0;

    async function load(off) {
      if (off !== undefined) offset = off;
      try {
        await HaoVPN.ensureDisplayTZ();
        await HaoVPN.refreshCSRF();
        var limit = parseInt(document.getElementById('fLimit').value, 10) || 50;
        var q = HaoVPN.buildQuery({
          limit: limit,
          offset: offset,
          action: document.getElementById('fAction').value.trim()
        });
        var data = await HaoVPN.api('/api/v1/audit' + q);
        var rows = data.items || [];
        var tb = document.getElementById('list');
        tb.innerHTML = '';
        if (!rows.length) {
          tb.innerHTML = '<tr><td colspan="4" class="text-muted">无记录</td></tr>';
        }
        rows.forEach(function (e) {
          var tr = document.createElement('tr');
          var actionLabel = e.action || '';
          if (e.action_zh) {
            actionLabel = actionLabel + '（' + e.action_zh + '）';
          }
          var target = formatAuditTarget(e);
          var ts = HaoVPN.formatTime(e.created_at);
          tr.innerHTML =
            '<td class="text-mono">' + ts + '</td>' +
            '<td><span class="badge badge-ok">' + actionLabel + '</span></td>' +
            '<td>' + target + '</td>' +
            '<td class="text-mono">' + (e.client_ip || '') + '</td>';
          tb.appendChild(tr);
        });
        HaoVPN.renderPager(document.getElementById('pager'), data.total, limit, offset, load);
      } catch (ex) {
        HaoVPN.toast(ex.message, 'error');
      }
    }

    /** 目标列：用户 →「用户名 (#id)」；其它 →「中文类型」[+ #id] */
    function formatAuditTarget(e) {
      var typeZH = e.target_type_zh || e.target_type || '';
      var id = e.target_id;
      if (e.target_type === 'user') {
        if (e.target_username && id != null) {
          return e.target_username + ' (#' + id + ')';
        }
        if (e.target_username) {
          return e.target_username;
        }
        if (id != null) {
          return (typeZH || '用户') + ' (#' + id + ')';
        }
        return typeZH || '用户';
      }
      if (id != null && id !== '') {
        return (typeZH || e.target_type || '') + ' #' + id;
      }
      return typeZH || e.target_type || '';
    }

    document.getElementById('fAction').oninput = HaoVPN.debounce(function () { load(0); }, 400);
    load(0);
  
