// 页面脚本：user_list — 从模板内联迁出，配合 CSP script-src 'self'
    HaoVPN.setActiveNav('users');
    var accountCache = {};
    var listOffset = 0;

    function syncCreateModeUI() {
      var mode = document.getElementById('ip_mode').value;
      document.getElementById('vpn_ip_group').style.display = mode === 'fixed' ? '' : 'none';
      document.getElementById('lease_group').style.display = mode === 'dynamic_lease' ? '' : 'none';
      if (mode !== 'fixed') document.getElementById('create_vpn_ip').value = '';
    }
    function syncPolicyModeUI() {
      var mode = document.getElementById('policyIPMode').value;
      var fixed = mode === 'fixed';
      document.getElementById('policyVpnIpGroup').style.display = fixed ? '' : 'none';
      document.getElementById('policyLeaseGroup').style.display = mode === 'dynamic_lease' ? '' : 'none';
      document.getElementById('policyVpnIp').disabled = !fixed;
    }
    document.getElementById('ip_mode').addEventListener('change', syncCreateModeUI);
    document.getElementById('policyIPMode').addEventListener('change', syncPolicyModeUI);

    async function load(off) {
      if (off !== undefined) listOffset = off;
      await HaoVPN.refreshCSRF();
      var limit = parseInt(document.getElementById('fLimit').value, 10) || 50;
      var q = HaoVPN.buildQuery({
        q: document.getElementById('fQ').value.trim(),
        enabled: document.getElementById('fEnabled').value,
        limit: limit,
        offset: listOffset
      });
      var resp = await HaoVPN.api('/api/v1/users' + q);
      var users = resp.items || [];
      var tb = document.getElementById('list');
      tb.innerHTML = '';
      accountCache = {};
      if (!users.length) {
        tb.innerHTML = '<tr><td colspan="8" class="text-muted">无匹配账号</td></tr>';
      }
      users.forEach(function (u) {
        accountCache[u.id] = u;
        var tr = document.createElement('tr');
        var ops = '';
        if (u.has_vpn) {
          ops +=
            '<button type="button" class="btn btn-ghost btn-sm" onclick="exportZip(' + u.id + ',\'' + u.username + '\')">ZIP</button>' +
            '<button type="button" class="btn btn-ghost btn-sm" onclick="exportYaml(' + u.id + ',\'' + u.username + '\')">YAML</button>' +
            '<button class="btn btn-ghost btn-sm" onclick="openPolicy(' + u.id + ')">策略</button>' +
            '<button class="btn btn-ghost btn-sm" onclick="kick(' + u.id + ')">踢线</button>';
        }
        ops +=
          '<button class="btn btn-ghost btn-sm" onclick="openPassword(' + u.id + ')">改密</button>' +
          '<button class="btn btn-ghost btn-sm" onclick="toggle(' + u.id + ',' + (u.enabled ? 'true' : 'false') + ')">' +
          (u.enabled ? '禁用' : '启用') + '</button>' +
          '<button class="btn btn-danger btn-sm" onclick="delUser(' + u.id + ')">删除</button>';
        tr.innerHTML =
          '<td class="text-mono">' + u.id + '</td>' +
          '<td>' + u.username + '</td>' +
          '<td class="text-mono">' + (u.vpn_ip || (u.has_vpn ? '动态' : '—')) + '</td>' +
          '<td class="text-mono">' + (u.ip_mode || '—') + '</td>' +
          '<td class="text-mono">' + (u.policy_ver || '—') + '</td>' +
          '<td>' + (u.online ? HaoVPN.statusBadge(true, '在线', '') : '<span class="badge badge-warn">离线</span>') + '</td>' +
          '<td>' + (u.enabled ? HaoVPN.statusBadge(true, '启用', '') : HaoVPN.statusBadge(false, '', '禁用')) + '</td>' +
          '<td class="actions-cell">' + ops + '</td>';
        tb.appendChild(tr);
      });
      HaoVPN.renderPager(document.getElementById('pager'), resp.total, limit, listOffset, load);
    }

    document.getElementById('fQ').oninput = HaoVPN.debounce(function () { load(0); }, 400);
    document.getElementById('fEnabled').onchange = function () { load(0); };
    document.getElementById('fLimit').onchange = function () { load(0); };

    document.getElementById('create').addEventListener('submit', async function (e) {
      e.preventDefault();
      var mode = e.target.ip_mode.value;
      if (mode !== 'fixed' && !confirm('动态 IP 可能影响现场 PLC 白名单，确认使用 ' + mode + '？')) return;
      try {
        var res = await HaoVPN.api('/api/v1/users', { method: 'POST', body: new FormData(e.target) });
        HaoVPN.toast('账号已创建' + (res.vpn_ip ? ' IP=' + res.vpn_ip : ''), 'success');
        e.target.reset();
        syncCreateModeUI();
        load();
      } catch (ex) { HaoVPN.toast(ex.message, 'error'); }
    });

    function openPolicy(id) {
      var u = accountCache[id];
      if (!u) return;
      document.getElementById('policyUserId').value = id;
      document.getElementById('policyHint').textContent = u.username + ' · 当前 policy_ver=' + (u.policy_ver || 1) +
        ' · fixed 可改 VPN IP；dynamic 由握手分配';
      document.getElementById('policyAllowed').value = (u.allowed_ips || []).join('\n');
      document.getElementById('policyIPMode').value = u.ip_mode || 'fixed';
      document.getElementById('policyVpnIp').value = u.vpn_ip || '';
      document.getElementById('policyLease').value = 86400;
      syncPolicyModeUI();
      document.getElementById('policyModal').style.display = 'flex';
    }
    function closePolicy() {
      document.getElementById('policyModal').style.display = 'none';
    }

    function openPassword(id) {
      var u = accountCache[id];
      if (!u) return;
      document.getElementById('passwordUserId').value = id;
      document.getElementById('passwordHint').textContent = '为账号「' + u.username + '」设置新登录密码（隧道/Web 均使用此密码；保存后踢线）';
      document.getElementById('passwordNew').value = '';
      document.getElementById('passwordConfirm').value = '';
      document.getElementById('passwordModal').style.display = 'flex';
    }
    function closePassword() {
      document.getElementById('passwordModal').style.display = 'none';
    }

    document.getElementById('passwordForm').addEventListener('submit', async function (e) {
      e.preventDefault();
      var id = document.getElementById('passwordUserId').value;
      var p1 = document.getElementById('passwordNew').value;
      var p2 = document.getElementById('passwordConfirm').value;
      if (p1 !== p2) {
        HaoVPN.toast('两次输入的密码不一致', 'error');
        return;
      }
      var fd = new FormData();
      fd.append('new_password', p1);
      try {
        await HaoVPN.refreshCSRF();
        await HaoVPN.api('/api/v1/users/' + id + '/password', { method: 'POST', body: fd });
        HaoVPN.toast('密码已更新，在线会话已踢线', 'success');
        closePassword();
      } catch (ex) { HaoVPN.toast(ex.message, 'error'); }
    });

    document.getElementById('policyForm').addEventListener('submit', async function (e) {
      e.preventDefault();
      var id = document.getElementById('policyUserId').value;
      var mode = document.getElementById('policyIPMode').value;
      var lines = document.getElementById('policyAllowed').value.split(/\n+/).map(function (s) { return s.trim(); }).filter(Boolean);
      var body = {
        allowed_ips: lines,
        ip_mode: mode,
        ip_lease_sec: parseInt(document.getElementById('policyLease').value, 10) || 86400
      };
      if (mode === 'fixed') {
        body.vpn_ip = document.getElementById('policyVpnIp').value.trim();
        if (!body.vpn_ip) {
          HaoVPN.toast('fixed 模式请填写 VPN IP', 'error');
          return;
        }
      } else {
        body.vpn_ip = '';
      }
      try {
        await HaoVPN.refreshCSRF();
        await HaoVPN.api('/api/v1/users/' + id + '/vpn', {
          method: 'PATCH',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body)
        });
        HaoVPN.toast('策略已更新，在线会话已踢线', 'success');
        closePolicy();
        load();
      } catch (ex) { HaoVPN.toast(ex.message, 'error'); }
    });

    async function kick(id) {
      try {
        await HaoVPN.api('/api/v1/users/' + id + '/kick', { method: 'POST' });
        HaoVPN.toast('已踢线', 'success');
        load();
      } catch (ex) { HaoVPN.toast(ex.message, 'error'); }
    }

    async function exportZip(id, username) {
      try {
        await HaoVPN.downloadPost('/api/v1/users/' + id + '/export.zip', 'haovpn-client-' + username + '.zip');
        HaoVPN.toast('ZIP 已开始下载', 'ok');
      } catch (ex) { HaoVPN.toast(ex.message, 'error'); }
    }

    async function exportYaml(id, username) {
      try {
        await HaoVPN.downloadPost('/api/v1/users/' + id + '/export', 'client-' + username + '.yaml');
        HaoVPN.toast('YAML 已开始下载', 'ok');
      } catch (ex) { HaoVPN.toast(ex.message, 'error'); }
    }

    async function toggle(id, en) {
      var fd = new FormData();
      fd.append('action', en ? 'disable' : 'enable');
      try {
        await HaoVPN.api('/api/v1/users/' + id, { method: 'POST', body: fd });
        load();
      } catch (ex) { HaoVPN.toast(ex.message, 'error'); }
    }

    async function delUser(id) {
      if (!confirm('确定删除该账号？隧道密钥与 IP 将一并释放。')) return;
      try {
        await HaoVPN.api('/api/v1/users/' + id, { method: 'DELETE' });
        HaoVPN.toast('已删除', 'success');
        load();
      } catch (ex) { HaoVPN.toast(ex.message, 'error'); }
    }

    syncCreateModeUI();
    load(0);
  
