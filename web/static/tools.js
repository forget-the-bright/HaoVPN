// 页面脚本：tools — 从模板内联迁出，配合 CSP script-src 'self'
    HaoVPN.setActiveNav('tools');
    var logOffset = 0;

    document.getElementById('backupBtn').onclick = async function () {
      try {
        await HaoVPN.downloadPost('/api/v1/backup', 'HaoVPN-backup.db');
        HaoVPN.toast('备份已开始下载', 'ok');
      } catch (ex) {
        HaoVPN.toast(ex.message, 'error');
      }
    };

    document.getElementById('logSource').onchange = function () {
      var hist = this.value === 'history';
      document.getElementById('levelGroup').style.display = hist ? '' : 'none';
      document.getElementById('kwGroup').style.display = hist ? '' : 'none';
      logOffset = 0;
      loadLogs();
    };

    async function loadLogs(off) {
      if (off !== undefined) logOffset = off;
      try {
        await HaoVPN.refreshCSRF();
        var src = document.getElementById('logSource').value;
        var tail = document.getElementById('logTail').value;
        var params = { source: src, tail: tail };
        if (src === 'history') {
          params.offset = logOffset;
          params.level = document.getElementById('logLevel').value;
          params.q = document.getElementById('logQ').value.trim();
        }
        var data = await HaoVPN.api('/api/v1/logs' + HaoVPN.buildQuery(params));
        var lines = data.lines || [];
        document.getElementById('logLines').textContent = lines.join('\n') || '(空)';
        var meta = (data.file || '') + (data.truncated ? ' · 仅显示尾部' : '');
        if (src === 'history') meta = (data.file || 'logs.db') + ' · 共 ' + (data.total || 0) + ' 条';
        document.getElementById('logMeta').textContent = meta;
        if (src === 'history') {
          HaoVPN.renderPager(document.getElementById('logPager'), data.total, parseInt(tail, 10), logOffset, loadLogs);
        } else {
          document.getElementById('logPager').innerHTML = '';
        }
      } catch (ex) {
        HaoVPN.toast(ex.message, 'error');
      }
    }

    document.getElementById('logQ').oninput = HaoVPN.debounce(function () {
      if (document.getElementById('logSource').value === 'history') {
        logOffset = 0;
        loadLogs();
      }
    }, 400);

    document.getElementById('btnRefreshLogs').addEventListener('click', function () { loadLogs(); });

    loadLogs();
  
