// 页面脚本：peer_routes — 从模板内联迁出，配合 CSP script-src 'self'
    HaoVPN.setActiveNav('peers');
    var users = [];
    var dirty = false;
    var memberEditRoute = null; // 正在编辑访问方的路由行

    document.getElementById('routeScope').addEventListener('change', function() {
      document.getElementById('accessorGroup').style.display =
        this.value === 'user' ? '' : 'none';
    });

    function setDirty(v) {
      dirty = !!v;
      document.getElementById('pendingBanner').style.display = dirty ? '' : 'none';
    }

    function fillUserSelects() {
      var ids = ['viaUser', 'accessorUser', 'accUser', 'peerUser'];
      ids.forEach(function(id) {
        var sel = document.getElementById(id);
        sel.innerHTML = '';
        users.forEach(function(u) {
          if (!u.has_vpn && !u.vpn_ip) return;
          var opt = document.createElement('option');
          opt.value = u.id;
          opt.textContent = u.username + (u.vpn_ip ? ' (' + u.vpn_ip + ')' : '');
          sel.appendChild(opt);
        });
      });
    }

    async function loadUsers() {
      var data = await HaoVPN.api('/api/v1/users?limit=500');
      users = data.items || data.users || [];
      if (!users.length && Array.isArray(data)) users = data;
      fillUserSelects();
    }

    async function loadPending() {
      try {
        var data = await HaoVPN.api('/api/v1/peers/apply');
        setDirty(!!data.pending_apply);
      } catch (e) {
        /* ignore */
      }
    }

    async function loadPeersPolicy() {
      var data = await HaoVPN.api('/api/v1/security/vpn-peers');
      document.getElementById('allowAllPeers').checked = !!data.allow_all_vpn_peers;
    }

    async function savePeersPolicy() {
      await HaoVPN.refreshCSRF();
      await HaoVPN.api('/api/v1/security/vpn-peers', {
        method: 'PUT',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({allow_all_vpn_peers: document.getElementById('allowAllPeers').checked})
      });
      alert('全局互访开关已保存（即时生效）');
    }

    // 应用生效：全部 .btn-apply 同步 loading（保留 .btn-label 结构）
    function setApplyLoading(on) {
      document.querySelectorAll('.btn-apply').forEach(function(btn) {
        btn.disabled = !!on;
        var label = btn.querySelector('.btn-label');
        var spin = btn.querySelector('.btn-spinner');
        if (on) {
          if (label) label.textContent = '应用中…';
          if (!spin) {
            spin = document.createElement('span');
            spin.className = 'btn-spinner';
            spin.setAttribute('aria-hidden', 'true');
            btn.insertBefore(spin, btn.firstChild);
          }
        } else {
          if (spin) spin.remove();
          if (label) label.textContent = '应用生效';
          else btn.textContent = '应用生效';
        }
      });
    }

    async function applyPeers() {
      setApplyLoading(true);
      try {
        await HaoVPN.refreshCSRF();
        var data = await HaoVPN.api('/api/v1/peers/apply', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({})
        });
        setDirty(false);
        alert(data.message || ('已踢线 ' + (data.kicked || 0) + ' 个账号'));
      } catch (e) {
        alert('应用失败: ' + (e.message || e));
      } finally {
        setApplyLoading(false);
      }
    }

    async function loadRegistry() {
      await HaoVPN.ensureDisplayTZ();
      var data = await HaoVPN.api('/api/v1/lan-registry');
      var rows = data.items || [];
      var tb = document.getElementById('registryRows');
      tb.innerHTML = '';
      if (!rows.length) {
        tb.innerHTML = '<tr><td colspan="6" class="text-muted">暂无注册（客户端须配置 local_lans 并在线）</td></tr>';
        return;
      }
      rows.forEach(function(r) {
        var tr = document.createElement('tr');
        tr.innerHTML =
          '<td>' + esc(r.username || r.user_id) + '</td>' +
          '<td>' + esc(r.vpn_ip) + '</td>' +
          '<td><code>' + esc(r.dest_cidr) + '</code></td>' +
          '<td>' + esc(r.host_id || '') + '</td>' +
          '<td>' + esc(HaoVPN.formatTime(r.updated_at)) + '</td>' +
          '<td><button type="button" class="btn btn-primary btn-sm">创建托管路由</button></td>';
        tr.querySelector('button').onclick = function() {
          document.getElementById('destCidr').value = r.dest_cidr;
          document.getElementById('viaUser').value = String(r.user_id);
          document.getElementById('routeScope').value = 'all';
          document.getElementById('accessorGroup').style.display = 'none';
          window.scrollTo(0, document.getElementById('destCidr').offsetTop);
        };
        tb.appendChild(tr);
      });
    }

    async function loadRoutes() {
      var data = await HaoVPN.api('/api/v1/peer-routes');
      var rows = data.items || [];
      var tb = document.getElementById('routeRows');
      tb.innerHTML = '';
      if (!rows.length) {
        tb.innerHTML = '<tr><td colspan="6" class="text-muted">暂无托管路由</td></tr>';
        return;
      }
      rows.forEach(function(r) {
        var tr = document.createElement('tr');
        var scope = r.member_names || (r.scope === 'all' ? '全部账号' : '指定');
        var via = r.via_username + (r.via_vpn_ip ? ' / ' + r.via_vpn_ip : ' (离线)');
        var st = r.stale ? '<span style="color:#c9a227">失效</span>' : '<span style="color:#3a7">有效</span>';
        tr.innerHTML =
          '<td><code>' + esc(r.display) + '</code></td>' +
          '<td>' + esc(r.dest_cidr) + '</td>' +
          '<td>' + esc(via) + '</td>' +
          '<td>' + esc(scope) + '</td>' +
          '<td>' + st + '</td>' +
          '<td>' +
            '<button type="button" class="btn btn-ghost btn-sm btn-members">访问方</button> ' +
            '<button type="button" class="btn btn-ghost btn-sm btn-del">删除</button>' +
          '</td>';
        tr.querySelector('.btn-del').onclick = function() { delRoute(r.id); };
        tr.querySelector('.btn-members').onclick = function() { editMembers(r); };
        tb.appendChild(tr);
      });
    }

    function vpnUsers() {
      return users.filter(function(u) { return u.has_vpn || u.vpn_ip; });
    }

    function closeMemberModal() {
      memberEditRoute = null;
      document.getElementById('memberModal').classList.add('hidden');
    }

    // 打开访问方多选框：第一项「全部账号」，其余为具体用户（via 本人不可选）
    function openMemberModal(r) {
      memberEditRoute = r;
      var viaID = r.via_user_id;
      var isAll = r.scope === 'all' || (r.member_user_ids || []).indexOf(0) >= 0;
      var selected = {};
      (r.member_user_ids || []).forEach(function(id) {
        if (id > 0) selected[id] = true;
      });

      document.getElementById('memberModalTitle').textContent =
        '选择访问方 · ' + (r.dest_cidr || '');
      document.getElementById('memberModalHint').textContent =
        'via：' + (r.via_username || viaID) +
        (r.via_vpn_ip ? ' / ' + r.via_vpn_ip : '') +
        '。勾选「全部账号」则任意客户端可经此 via 访问目标网段；或只勾选指定账号。';

      var list = document.getElementById('memberPickList');
      list.innerHTML = '';

      var allRow = document.createElement('label');
      allRow.className = 'member-pick-row all-row';
      allRow.innerHTML =
        '<input type="checkbox" id="memberPickAll">' +
        '<span>全部账号</span>';
      list.appendChild(allRow);
      var allCb = allRow.querySelector('input');
      allCb.checked = isAll;

      vpnUsers().forEach(function(u) {
        if (u.id === viaID) return; // via 不能是访问方自己
        var row = document.createElement('label');
        row.className = 'member-pick-row member-user-row';
        row.innerHTML =
          '<input type="checkbox" class="member-user-cb" value="' + u.id + '">' +
          '<span>' + esc(u.username) + '</span>' +
          '<span class="member-pick-meta">' + esc(u.vpn_ip || '未分配') + '</span>';
        var cb = row.querySelector('input');
        cb.checked = !isAll && !!selected[u.id];
        cb.disabled = isAll;
        list.appendChild(row);
      });

      function syncUserDisabled() {
        var on = allCb.checked;
        list.querySelectorAll('.member-user-cb').forEach(function(cb) {
          cb.disabled = on;
          if (on) cb.checked = false;
        });
      }
      allCb.onchange = syncUserDisabled;
      syncUserDisabled();

      document.getElementById('memberModal').classList.remove('hidden');
    }

    async function saveMemberModal() {
      if (!memberEditRoute) return;
      var allCb = document.getElementById('memberPickAll');
      var body = {};
      if (allCb && allCb.checked) {
        body.apply_all = true;
      } else {
        var ids = [];
        document.querySelectorAll('.member-user-cb:checked').forEach(function(cb) {
          var n = parseInt(cb.value, 10);
          if (n > 0) ids.push(n);
        });
        if (!ids.length) {
          alert('请勾选「全部账号」或至少一个访问方');
          return;
        }
        body.member_user_ids = ids;
      }
      var okBtn = document.getElementById('memberModalOk');
      okBtn.disabled = true;
      var okLabel = okBtn.textContent;
      okBtn.innerHTML = '<span class="btn-spinner" aria-hidden="true"></span> 保存中…';
      try {
        await HaoVPN.refreshCSRF();
        await HaoVPN.api('/api/v1/peer-routes/' + memberEditRoute.id + '/members', {
          method: 'PUT',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify(body)
        });
        closeMemberModal();
        setDirty(true);
        await loadRoutes();
      } catch (e) {
        alert('保存访问方失败: ' + (e.message || e));
      } finally {
        okBtn.disabled = false;
        okBtn.textContent = okLabel || '确定';
      }
    }

    function editMembers(r) {
      openMemberModal(r);
    }

    document.getElementById('memberModalClose').onclick = closeMemberModal;
    document.getElementById('memberModalCancel').onclick = closeMemberModal;
    document.getElementById('memberModalOk').onclick = function() { saveMemberModal(); };
    document.getElementById('memberModal').addEventListener('click', function(ev) {
      if (ev.target === this) closeMemberModal();
    });
    document.addEventListener('keydown', function(ev) {
      if (ev.key === 'Escape' && !document.getElementById('memberModal').classList.contains('hidden')) {
        closeMemberModal();
      }
    });

    async function addRoute() {
      var btn = document.getElementById('btnAddRoute');
      btn.disabled = true;
      try {
        await HaoVPN.refreshCSRF();
        var body = {
          dest_cidr: document.getElementById('destCidr').value.trim(),
          via_user_id: parseInt(document.getElementById('viaUser').value, 10),
          apply_all: document.getElementById('routeScope').value === 'all'
        };
        if (!body.apply_all) {
          body.user_id = parseInt(document.getElementById('accessorUser').value, 10);
        }
        await HaoVPN.api('/api/v1/peer-routes', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify(body)
        });
        document.getElementById('destCidr').value = '';
        setDirty(true);
        await loadRoutes();
      } catch (e) {
        alert('新增失败: ' + (e.message || e));
      } finally {
        btn.disabled = false;
      }
    }

    async function delRoute(id) {
      if (!confirm('删除该托管路由？删除后请点「应用生效」刷新相关客户端。')) return;
      await HaoVPN.refreshCSRF();
      await HaoVPN.api('/api/v1/peer-routes/' + id, {method: 'DELETE'});
      setDirty(true);
      await loadRoutes();
    }

    // 将双向 A→B / B→A 合并为一行展示
    function collapseAccessPairs(rows) {
      var seen = {};
      var out = [];
      rows.forEach(function(a) {
        var lo = Math.min(a.user_id, a.peer_user_id);
        var hi = Math.max(a.user_id, a.peer_user_id);
        var key = lo + ':' + hi;
        if (seen[key]) return;
        seen[key] = true;
        var left = a.user_id === lo ? a : null;
        var rightName = a.peer_username, rightIP = a.peer_vpn_ip, leftName = a.username, leftIP = '';
        rows.forEach(function(b) {
          if (b.user_id === lo && b.peer_user_id === hi) {
            leftName = b.username;
          }
          if (b.user_id === hi && b.peer_user_id === lo) {
            rightName = b.username;
            rightIP = b.peer_vpn_ip || rightIP;
          }
          if (b.user_id === lo) leftIP = ''; // 本侧 IP 从 users 补
        });
        var uLo = users.find(function(u) { return u.id === lo; });
        var uHi = users.find(function(u) { return u.id === hi; });
        out.push({
          user_id: lo,
          peer_user_id: hi,
          label: (leftName || (uLo && uLo.username) || lo) + ' ↔ ' + (rightName || (uHi && uHi.username) || hi),
          ips: ((uLo && uLo.vpn_ip) || '—') + ' / ' + ((uHi && uHi.vpn_ip) || rightIP || '—')
        });
      });
      return out;
    }

    async function loadAccess() {
      var data = await HaoVPN.api('/api/v1/peer-access');
      var rows = collapseAccessPairs(data.items || []);
      var tb = document.getElementById('accessRows');
      tb.innerHTML = '';
      if (!rows.length) {
        tb.innerHTML = '<tr><td colspan="3" class="text-muted">暂无白名单</td></tr>';
        return;
      }
      rows.forEach(function(a) {
        var tr = document.createElement('tr');
        tr.innerHTML =
          '<td>' + esc(a.label) + '</td>' +
          '<td>' + esc(a.ips) + '</td>' +
          '<td><button type="button" class="btn btn-ghost btn-sm">删除</button></td>';
        tr.querySelector('button').onclick = function() {
          delAccess(a.user_id, a.peer_user_id);
        };
        tb.appendChild(tr);
      });
    }

    async function addAccess() {
      var btn = document.getElementById('btnAddAccess');
      btn.disabled = true;
      try {
        await HaoVPN.refreshCSRF();
        await HaoVPN.api('/api/v1/peer-access', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({
            user_id: parseInt(document.getElementById('accUser').value, 10),
            peer_user_id: parseInt(document.getElementById('peerUser').value, 10)
          })
        });
        setDirty(true);
        await loadAccess();
      } catch (e) {
        alert('添加失败: ' + (e.message || e));
      } finally {
        btn.disabled = false;
      }
    }

    async function delAccess(uid, pid) {
      if (!confirm('删除该双向互访？')) return;
      await HaoVPN.refreshCSRF();
      await HaoVPN.api('/api/v1/peer-access?user_id=' + uid + '&peer_user_id=' + pid, {method: 'DELETE'});
      setDirty(true);
      await loadAccess();
    }

    function esc(s) {
      return String(s == null ? '' : s)
        .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    (async function() {
      try {
        await HaoVPN.refreshCSRF();
        await loadUsers();
        await loadPeersPolicy();
        await loadRegistry();
        await loadRoutes();
        await loadAccess();
        await loadPending();
      } catch (e) {
        console.error(e);
        alert('加载失败: ' + (e.message || e));
      }
    })();
  
