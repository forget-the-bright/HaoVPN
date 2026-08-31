/**
 * 登录页逻辑：登录、首次强制改密。
 * 从 templates/login.html 抽出；须外置脚本以符合 CSP script-src 'self'
 *（禁止内联 <script> 与 onclick=；见 security/tls_policy.go）。
 */
(function () {
  'use strict';

  if (location.search.includes('change=1')) {
    var alertEl = document.getElementById('loginAlert');
    if (alertEl) alertEl.classList.remove('hidden');
  }

  var loginForm = document.getElementById('loginForm');
  if (loginForm) {
    loginForm.addEventListener('submit', async function (e) {
      e.preventDefault();
      var errEl = document.getElementById('loginError');
      errEl.classList.add('hidden');
      var fd = new FormData(e.target);
      try {
        var r = await fetch('/api/v1/login', { method: 'POST', body: fd });
        var j = await r.json();
        if (!j.ok) {
          errEl.textContent = j.error || '登录失败';
          errEl.classList.remove('hidden');
          return;
        }
        HaoVPN.setCSRF(j.csrf_token || '');
        if (j.must_change_password) {
          document.getElementById('loginForm').classList.add('hidden');
          document.getElementById('changeBox').classList.remove('hidden');
          var pw = document.getElementById('password').value;
          if (pw) document.getElementById('old_password').value = pw;
        } else {
          location.href = '/';
        }
      } catch (ex) {
        errEl.textContent = ex.message || '网络错误';
        errEl.classList.remove('hidden');
      }
    });
  }

  var changeForm = document.getElementById('changeForm');
  if (changeForm) {
    changeForm.addEventListener('submit', async function (e) {
      e.preventDefault();
      var fd = new FormData(e.target);
      try {
        var j = await HaoVPN.api('/api/v1/password', { method: 'POST', body: fd });
        if (j.ok) {
          HaoVPN.toast('密码已更新，请使用新密码重新登录', 'ok');
          location.href = '/login';
        } else {
          HaoVPN.toast(j.error || '修改失败', 'error');
        }
      } catch (ex) {
        HaoVPN.toast(ex.message, 'error');
      }
    });
  }
})();
