// 页面脚本：peer_routes — 从模板内联迁出，配合 CSP script-src 'self'
    HaoVPN.setActiveNav('peers');
    var users = [];
    var dirty = false;
    var memberEditRoute = null; // 正在编辑访问方的路由行

    document.getElementById('routeScope').addEventListener('change', function() {
      HaoVPN.setVisible('accessorGroup', this.value === 'user');
    });
    var dnsScopeEl = document.getElementById('dnsScope');
    if (dnsScopeEl) {
      dnsScopeEl.addEventListener('change', function() {
        HaoVPN.setVisible('dnsMemberGroup', this.value === 'user');
      });
    }

    var dnsOffset = 0;
    var dnsExcludeEdit = null; // 正在编辑排除名单的 DNS 行
    var dnsMemberEdit = null; // 正在编辑包含范围的 DNS 行（复用成员模态）
    var dnsCreateMemberIDs = []; // 新增 DNS 时指定账号多选结果
    var dnsCreatePickMode = false; // true=新增选人；false=编辑已有行范围
    var regOffset = 0;
    var routeOffset = 0;
    var accessOffset = 0;

    function setDirty(v) {
      dirty = !!v;
      HaoVPN.setVisible('pendingBanner', dirty);
    }

    function updateDNSCreateMemberSummary() {
      var el = document.getElementById('dnsCreateMemberSummary');
      if (!el) return;
      if (!dnsCreateMemberIDs.length) {
        el.textContent = '未选择';
        return;
      }
      var names = [];
      dnsCreateMemberIDs.forEach(function(id) {
        var u = users.find(function(x) { return x.id === id; });
        names.push(u ? u.username : String(id));
      });
      el.textContent = '已选 ' + dnsCreateMemberIDs.length + ' 人：' + names.join(', ');
    }

    function fillUserSelects() {
      var ids = ['viaUser', 'accessorUser', 'accUser', 'peerUser'];
      ids.forEach(function(id) {
        var sel = document.getElementById(id);
        if (!sel) return;
        sel.innerHTML = '';
        users.forEach(function(u) {
          if (!u.has_vpn && !u.vpn_ip) return;
          var opt = document.createElement('option');
          opt.value = u.id;
          opt.textContent = u.username + (u.vpn_ip ? ' (' + u.vpn_ip + ')' : '');
          sel.appendChild(opt);
        });
      });
      updateDNSCreateMemberSummary();
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

    async function loadRegistry(off) {
      if (off !== undefined) regOffset = off;
      await HaoVPN.ensureDisplayTZ();
      var limit = parseInt(document.getElementById('regLimit').value, 10) || 50;
      var data = await HaoVPN.api('/api/v1/lan-registry' + HaoVPN.buildQuery({limit: limit, offset: regOffset}));
      var rows = data.items || [];
      var tb = document.getElementById('registryRows');
      tb.innerHTML = '';
      if (!rows.length) {
        tb.innerHTML = '<tr><td colspan="6" class="text-muted">暂无注册（客户端须配置 local_lans 并在线）</td></tr>';
      } else {
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
            HaoVPN.setVisible('accessorGroup', false);
            window.scrollTo(0, document.getElementById('destCidr').offsetTop);
          };
          tb.appendChild(tr);
        });
      }
      HaoVPN.renderPager(document.getElementById('regPager'), data.total, limit, regOffset, loadRegistry);
    }

    async function loadRoutes(off) {
      if (off !== undefined) routeOffset = off;
      var limit = parseInt(document.getElementById('routeLimit').value, 10) || 50;
      var data = await HaoVPN.api('/api/v1/peer-routes' + HaoVPN.buildQuery({limit: limit, offset: routeOffset}));
      var rows = data.items || [];
      var tb = document.getElementById('routeRows');
      tb.innerHTML = '';
      if (!rows.length) {
        tb.innerHTML = '<tr><td colspan="6" class="text-muted">暂无托管路由</td></tr>';
      } else {
        rows.forEach(function(r) {
          var tr = document.createElement('tr');
          var scope = r.member_names || (r.scope === 'all' ? '全部账号' : '指定');
          var via = r.via_username + (r.via_vpn_ip ? ' / ' + r.via_vpn_ip : ' (离线)');
          var st = r.stale ? '<span class="status-stale">失效</span>' : '<span class="status-ok">有效</span>';
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
      HaoVPN.renderPager(document.getElementById('routePager'), data.total, limit, routeOffset, loadRoutes);
    }

    function vpnUsers() {
      return users.filter(function(u) { return u.has_vpn || u.vpn_ip; });
    }

    // 关闭成员/排除共用模态：必须还原 OK 回调，否则 DNS 排除编辑取消后
    // 再点托管路由「访问方」确定会误调排除保存并可能清空排除名单。
    function closeMemberModal() {
      memberEditRoute = null;
      dnsExcludeEdit = null;
      dnsMemberEdit = null;
      dnsCreatePickMode = false;
      document.getElementById('memberModalOk').onclick = function() { saveMemberModal(); };
      document.getElementById('memberModal').classList.add('hidden');
    }

    // 打开访问方多选框：第一项「全部账号」，其余为具体用户（via 本人不可选）
    function openMemberModal(r) {
      memberEditRoute = r;
      dnsExcludeEdit = null;
      dnsMemberEdit = null;
      document.getElementById('memberModalOk').onclick = function() { saveMemberModal(); };
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

    async function loadAccess(off) {
      if (off !== undefined) accessOffset = off;
      var limit = parseInt(document.getElementById('accessLimit').value, 10) || 50;
      // 双向对须全量折叠后再分页：循环拉齐所有页（每页 API max=200），避免固定 500 截断。
      var pageSize = 200;
      var raw = [];
      var offset = 0;
      for (;;) {
        var data = await HaoVPN.api('/api/v1/peer-access' + HaoVPN.buildQuery({limit: pageSize, offset: offset}));
        var chunk = data.items || [];
        raw = raw.concat(chunk);
        if (chunk.length < pageSize) break;
        offset += pageSize;
        if (typeof data.total === 'number' && offset >= data.total) break;
      }
      var all = collapseAccessPairs(raw);
      var total = all.length;
      var rows = all.slice(accessOffset, accessOffset + limit);
      var tb = document.getElementById('accessRows');
      tb.innerHTML = '';
      if (!rows.length) {
        tb.innerHTML = '<tr><td colspan="3" class="text-muted">暂无白名单</td></tr>';
      } else {
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
      HaoVPN.renderPager(document.getElementById('accessPager'), total, limit, accessOffset, loadAccess);
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

    async function loadDNS(off) {
      if (off !== undefined) dnsOffset = off;
      var limit = parseInt(document.getElementById('dnsLimit').value, 10) || 50;
      var data = await HaoVPN.api('/api/v1/dns-servers' + HaoVPN.buildQuery({limit: limit, offset: dnsOffset}));
      var tb = document.getElementById('dnsRows');
      tb.innerHTML = '';
      var rows = data.items || [];
      if (!rows.length) {
        tb.innerHTML = '<tr><td colspan="6" class="text-muted">暂无托管 DNS（可从 YAML seed 或下方新增）</td></tr>';
      } else {
        rows.forEach(function(d) {
          var src = d.source === 'config' ? '配置' : '手工';
          var tr = document.createElement('tr');
          tr.innerHTML =
            '<td><code>' + esc(d.dns_ip) + '</code></td>' +
            '<td>' + esc(d.remark || '') + '</td>' +
            '<td>' + src + '</td>' +
            '<td>' + esc(d.member_names || d.scope) + '</td>' +
            '<td>' + esc(d.exclude_names || '—') + '</td>' +
            '<td></td>';
          var ops = tr.querySelector('td:last-child');
          var btnRemark = document.createElement('button');
          btnRemark.type = 'button';
          btnRemark.className = 'btn btn-ghost btn-sm';
          btnRemark.textContent = '备注';
          btnRemark.onclick = function() { editDNSRemark(d); };
          ops.appendChild(btnRemark);
          ops.appendChild(document.createTextNode(' '));
          if (d.can_edit_excludes) {
            var btnEx = document.createElement('button');
            btnEx.type = 'button';
            btnEx.className = 'btn btn-ghost btn-sm';
            btnEx.textContent = '排除';
            btnEx.onclick = function() { openDNSExcludeModal(d); };
            ops.appendChild(btnEx);
            ops.appendChild(document.createTextNode(' '));
          }
          if (!d.readonly_ip) {
            var btnMem = document.createElement('button');
            btnMem.type = 'button';
            btnMem.className = 'btn btn-ghost btn-sm';
            btnMem.textContent = '范围';
            btnMem.onclick = function() { editDNSMembers(d); };
            ops.appendChild(btnMem);
            ops.appendChild(document.createTextNode(' '));
            var btnDel = document.createElement('button');
            btnDel.type = 'button';
            btnDel.className = 'btn btn-ghost btn-sm';
            btnDel.textContent = '删除';
            btnDel.onclick = function() { delDNS(d.id); };
            ops.appendChild(btnDel);
          }
          tb.appendChild(tr);
        });
      }
      HaoVPN.renderPager(document.getElementById('dnsPager'), data.total, limit, dnsOffset, loadDNS);
    }

    async function addDNS() {
      var btn = document.getElementById('btnAddDNS');
      btn.disabled = true;
      try {
        await HaoVPN.refreshCSRF();
        var body = {
          dns_ip: document.getElementById('dnsIp').value.trim(),
          remark: document.getElementById('dnsRemark').value.trim(),
          apply_all: document.getElementById('dnsScope').value === 'all'
        };
        if (!body.apply_all) {
          if (!dnsCreateMemberIDs.length) {
            alert('请先点「选择账号」勾选至少一个 VPN 账号');
            return;
          }
          body.member_user_ids = dnsCreateMemberIDs.slice();
        }
        await HaoVPN.api('/api/v1/dns-servers', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify(body)
        });
        document.getElementById('dnsIp').value = '';
        document.getElementById('dnsRemark').value = '';
        dnsCreateMemberIDs = [];
        updateDNSCreateMemberSummary();
        setDirty(true);
        await loadDNS(0);
      } catch (e) {
        alert('新增 DNS 失败: ' + (e.message || e));
      } finally {
        btn.disabled = false;
      }
    }

    // 新增 DNS：打开多选账号模态（无「全部」项；全部走 dnsScope）
    function openDNSCreateMemberModal() {
      dnsCreatePickMode = true;
      dnsMemberEdit = null;
      dnsExcludeEdit = null;
      memberEditRoute = null;
      var selected = {};
      dnsCreateMemberIDs.forEach(function(id) { selected[id] = true; });
      document.getElementById('memberModalTitle').textContent = '选择 DNS 绑定账号';
      document.getElementById('memberModalHint').textContent =
        '勾选一个或多个 VPN 账号；若要对全部账号生效请改上方「适用范围」为全部账号。';
      var list = document.getElementById('memberPickList');
      list.innerHTML = '';
      vpnUsers().forEach(function(u) {
        var row = document.createElement('label');
        row.className = 'member-pick-row member-user-row';
        row.innerHTML =
          '<input type="checkbox" class="member-user-cb" value="' + u.id + '">' +
          '<span>' + esc(u.username) + '</span>' +
          '<span class="member-pick-meta">' + esc(u.vpn_ip || '未分配') + '</span>';
        row.querySelector('input').checked = !!selected[u.id];
        list.appendChild(row);
      });
      document.getElementById('memberModalOk').onclick = saveDNSCreateMemberModal;
      document.getElementById('memberModal').classList.remove('hidden');
    }

    function saveDNSCreateMemberModal() {
      var ids = [];
      document.querySelectorAll('.member-user-cb:checked').forEach(function(cb) {
        var n = parseInt(cb.value, 10);
        if (n > 0) ids.push(n);
      });
      if (!ids.length) {
        alert('请至少勾选一个账号');
        return;
      }
      dnsCreateMemberIDs = ids;
      dnsCreatePickMode = false;
      updateDNSCreateMemberSummary();
      closeMemberModal();
    }

    async function editDNSRemark(d) {
      var remark = prompt('备注', d.remark || '');
      if (remark === null) return;
      await HaoVPN.refreshCSRF();
      await HaoVPN.api('/api/v1/dns-servers/' + d.id + '/remark', {
        method: 'PUT',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({remark: remark})
      });
      // 备注即时生效，不置 pending / 不踢线
      await loadDNS(dnsOffset);
    }

    // 打开 DNS 包含范围多选（与托管路由访问方同一模态；无 via 限制）
    function openDNSMemberModal(d) {
      dnsMemberEdit = d;
      dnsExcludeEdit = null;
      memberEditRoute = null;
      var isAll = d.scope === 'all' || (d.member_user_ids || []).indexOf(0) >= 0;
      var selected = {};
      (d.member_user_ids || []).forEach(function(id) {
        if (id > 0) selected[id] = true;
      });
      document.getElementById('memberModalTitle').textContent = 'DNS 适用范围 · ' + (d.dns_ip || '');
      document.getElementById('memberModalHint').textContent =
        '勾选「全部账号」则所有 VPN 账号下发该 DNS（仍可另配排除）；或只勾选指定账号。';
      var list = document.getElementById('memberPickList');
      list.innerHTML = '';
      var allRow = document.createElement('label');
      allRow.className = 'member-pick-row all-row';
      allRow.innerHTML = '<input type="checkbox" id="memberPickAll"><span>全部账号</span>';
      list.appendChild(allRow);
      var allCb = allRow.querySelector('input');
      allCb.checked = isAll;
      vpnUsers().forEach(function(u) {
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
      document.getElementById('memberModalOk').onclick = saveDNSMemberModal;
      document.getElementById('memberModal').classList.remove('hidden');
    }

    async function saveDNSMemberModal() {
      if (!dnsMemberEdit) return;
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
          alert('请勾选「全部账号」或至少一个账号');
          return;
        }
        body.member_user_ids = ids;
      }
      var okBtn = document.getElementById('memberModalOk');
      okBtn.disabled = true;
      try {
        await HaoVPN.refreshCSRF();
        await HaoVPN.api('/api/v1/dns-servers/' + dnsMemberEdit.id + '/members', {
          method: 'PUT',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify(body)
        });
        closeMemberModal();
        setDirty(true);
        await loadDNS(dnsOffset);
      } catch (e) {
        alert('保存适用范围失败: ' + (e.message || e));
      } finally {
        okBtn.disabled = false;
        okBtn.textContent = '确定';
      }
    }

    function editDNSMembers(d) {
      openDNSMemberModal(d);
    }

    function openDNSExcludeModal(d) {
      dnsExcludeEdit = d;
      dnsMemberEdit = null;
      memberEditRoute = null;
      var selected = {};
      (d.exclude_user_ids || []).forEach(function(id) { selected[id] = true; });
      document.getElementById('memberModalTitle').textContent = '排除账号 · ' + (d.dns_ip || '');
      document.getElementById('memberModalHint').textContent =
        '勾选后这些账号不会下发该 DNS（YAML/全部生效时的反向规则）。';
      var list = document.getElementById('memberPickList');
      list.innerHTML = '';
      vpnUsers().forEach(function(u) {
        var row = document.createElement('label');
        row.className = 'member-pick-row member-user-row';
        row.innerHTML =
          '<input type="checkbox" class="dns-exclude-cb" value="' + u.id + '">' +
          '<span>' + esc(u.username) + '</span>' +
          '<span class="member-pick-meta">' + esc(u.vpn_ip || '未分配') + '</span>';
        row.querySelector('input').checked = !!selected[u.id];
        list.appendChild(row);
      });
      document.getElementById('memberModalOk').onclick = saveDNSExcludeModal;
      document.getElementById('memberModal').classList.remove('hidden');
    }

    async function saveDNSExcludeModal() {
      if (!dnsExcludeEdit) return;
      var ids = [];
      document.querySelectorAll('.dns-exclude-cb:checked').forEach(function(cb) {
        var n = parseInt(cb.value, 10);
        if (n > 0) ids.push(n);
      });
      try {
        await HaoVPN.refreshCSRF();
        await HaoVPN.api('/api/v1/dns-servers/' + dnsExcludeEdit.id + '/excludes', {
          method: 'PUT',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({exclude_user_ids: ids})
        });
        closeMemberModal();
        setDirty(true);
        await loadDNS(dnsOffset);
      } catch (e) {
        alert('保存排除失败: ' + (e.message || e));
      }
    }

    async function delDNS(id) {
      if (!confirm('删除该托管 DNS？')) return;
      await HaoVPN.refreshCSRF();
      await HaoVPN.api('/api/v1/dns-servers/' + id, {method: 'DELETE'});
      setDirty(true);
      await loadDNS(0);
    }

    function esc(s) {
      return String(s == null ? '' : s)
        .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    // 应用生效：btnApplyPeersBanner / btnApplyPeers（须在源码出现 id，供 CSP 回归测绑定）
    ['btnApplyPeersBanner', 'btnApplyPeers'].forEach(function (id) {
      var btn = document.getElementById(id);
      if (btn) {
        btn.addEventListener('click', function () { applyPeers(); });
      }
    });
    document.getElementById('btnSavePeersPolicy').addEventListener('click', function () { savePeersPolicy(); });
    document.getElementById('btnRefreshRegistry').addEventListener('click', function () { loadRegistry(regOffset); });
    document.getElementById('btnAddRoute').addEventListener('click', function () { addRoute(); });
    document.getElementById('btnRefreshRoutes').addEventListener('click', function () { loadRoutes(routeOffset); });
    document.getElementById('btnAddAccess').addEventListener('click', function () { addAccess(); });
    document.getElementById('btnRefreshAccess').addEventListener('click', function () { loadAccess(accessOffset); });
    document.getElementById('btnAddDNS').addEventListener('click', function () { addDNS(); });
    document.getElementById('btnRefreshDNS').addEventListener('click', function () { loadDNS(dnsOffset); });
    var btnPickDNS = document.getElementById('btnPickDNSMembers');
    if (btnPickDNS) {
      btnPickDNS.addEventListener('click', function () { openDNSCreateMemberModal(); });
    }
    document.getElementById('dnsLimit').onchange = function () { loadDNS(0); };
    document.getElementById('regLimit').onchange = function () { loadRegistry(0); };
    document.getElementById('routeLimit').onchange = function () { loadRoutes(0); };
    document.getElementById('accessLimit').onchange = function () { loadAccess(0); };

    (async function() {
      try {
        await HaoVPN.refreshCSRF();
        await loadUsers();
        await loadPeersPolicy();
        await loadRegistry(0);
        await loadRoutes(0);
        await loadDNS(0);
        await loadAccess(0);
        await loadPending();
      } catch (e) {
        console.error(e);
        alert('加载失败: ' + (e.message || e));
      }
    })();
  
