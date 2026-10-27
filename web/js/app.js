    /* ==================== Config ==================== */
    const API_BASE = window.location.origin;
    function wsBaseUrl() {
      const p = location.protocol === 'https:' ? 'wss:' : 'ws:';
      return p + '//' + location.host;
    }
    function avatarUrl(a) {
      if (!a) return '';
      if (a.startsWith('http://') || a.startsWith('https://')) return a;
      return API_BASE + '/static/avatars/' + a;
    }
    function fileUrl(u) {
      if (!u) return '';
      if (u.startsWith('http://') || u.startsWith('https://')) return u;
      return API_BASE + '/static/files/' + u;
    }
    function esc(s) { if (!s) return ''; const d = document.createElement('div'); d.textContent = s; return d.innerHTML; }
    function fmtTime(t) {
      if (!t) return '';
      const d = new Date(t), now = new Date(), diff = now - d;
      if (diff < 60000) return '刚刚';
      if (diff < 3600000) return Math.floor(diff / 60000) + '分钟前';
      if (diff < 86400000) return Math.floor(diff / 3600000) + '小时前';
      if (diff < 172800000) return '昨天';
      return (d.getMonth() + 1) + '/' + d.getDate();
    }
    function fmtMsgTime(t) {
      if (!t) return '';
      const d = new Date(t);
      return d.getHours().toString().padStart(2, '0') + ':' + d.getMinutes().toString().padStart(2, '0');
    }

    // Step 1.1: 生成高唯一性客户端幂等键
    // 来源: 浏览器加密随机源或时间戳与随机数
    // 用于: Step 1.3 为写请求头及服务层提供防重凭据
    function genIdempotencyKey() {
      if (typeof crypto !== 'undefined' && crypto.randomUUID) return crypto.randomUUID();
      return 'idemp-' + Date.now() + '-' + Math.random().toString(36).substring(2, 11);
    }

    // Step 1.2: 计算具备 Full Jitter 随机抖动的指数退避重试延迟
    // 来源: 当前重试轮次 retryCount 及基础延迟 baseDelay
    // 用于: Step 1.3 在网络故障与 5xx 场景下次重试前的休眠等待
    function calculateRetryDelay(retryCount, baseDelay = 150, maxDelay = 2500) {
      const exponential = Math.min(maxDelay, baseDelay * Math.pow(2, retryCount));
      return Math.floor(Math.random() * exponential);
    }

    /* ==================== GapHoldingBuffer (时序空洞暂存与自愈队列) ==================== */
    class GapHoldingBuffer {
      constructor(initialSeq, onRelease) {
        this.lastContinuousSeq = Number(initialSeq || 0);
        this.pendingQueue = new Map();
        this.onRelease = onRelease;
      }

      getWatermark() {
        return this.lastContinuousSeq;
      }

      setWatermark(seq) {
        const s = Number(seq || 0);
        if (s > this.lastContinuousSeq) {
          this.lastContinuousSeq = s;
          this.drain();
        }
      }

      push(msg) {
        const seq = Number(msg.seq_id || 0);
        if (!seq) {
          this.onRelease(msg);
          return true;
        }
        if (seq <= this.lastContinuousSeq) {
          return false; // 重复或已展示历史消息
        }
        if (seq === this.lastContinuousSeq + 1) {
          this.lastContinuousSeq = seq;
          this.onRelease(msg);
          this.drain();
          return true;
        }
        // 发现时序空洞断层 (如期望 lastContinuousSeq+1，收到更高 seq)
        this.pendingQueue.set(seq, msg);
        return false;
      }

      reconcileDelta(messages) {
        if (!messages || !messages.length) return;
        const sorted = [...messages].sort((a, b) => Number(a.seq_id || 0) - Number(b.seq_id || 0));
        for (const msg of sorted) {
          this.push(msg);
        }
      }

      drain() {
        while (this.pendingQueue.has(this.lastContinuousSeq + 1)) {
          const nextSeq = this.lastContinuousSeq + 1;
          const nextMsg = this.pendingQueue.get(nextSeq);
          this.pendingQueue.delete(nextSeq);
          this.lastContinuousSeq = nextSeq;
          this.onRelease(nextMsg);
        }
      }

      hasGap() {
        return this.pendingQueue.size > 0;
      }
    }

    /* ==================== API Client ==================== */
    const api = {
      // Step 1.3: 执行具备多领域分级重试与幂等保障的 HTTP 请求
      // 来源: 业务触发的目标 path、请求配置 opts 及当前重试计数 retryCount
      // 用于: 向边缘网关 Envoy / 微服务发送请求，并在遇到瞬态网络异常时自愈重试
      async req(path, opts = {}, retryCount = 0) {
        // Step 1.3.1: 确定业务领域及对应的重试策略 (Domain-Specific Retry Strategy)
        // 来源: 请求路径 path
        // 用于: 限制最大重试次数与超时时间，防止安全敏感操作盲目重试
        let maxRetries = 2;
        let timeoutMs = 8000;
        let allowAutoRetry = true;

        if (path.startsWith('/auth/login') || path.startsWith('/auth/refresh')) {
          // 安全鉴权领域：严格反幂等，严禁自动静默重试，超时 5 秒
          maxRetries = 0;
          timeoutMs = 5000;
          allowAutoRetry = false;
        } else if (path.startsWith('/messages/send')) {
          // IM 消息发送领域：允许 2 次静默重试，超时 6 秒
          maxRetries = 2;
          timeoutMs = 6000;
        } else if (path.startsWith('/groups') || path.startsWith('/friends/apply')) {
          // 资源创建与审批领域：允许 2 次重试，超时 8 秒
          maxRetries = 2;
          timeoutMs = 8000;
        }

        const headers = { 'Content-Type': 'application/json' };
        const tok = localStorage.getItem('access_token');
        if (tok) headers.Authorization = 'Bearer ' + tok;
        const method = (opts.method || 'GET').toUpperCase();

        // 仅对非安全敏感的写操作注入 X-Idempotency-Key；若重试则严格沿用第一次生成的 key
        if (method !== 'GET' && method !== 'HEAD' && !path.startsWith('/auth/login') && !opts.headers?.['X-Idempotency-Key']) {
          headers['X-Idempotency-Key'] = genIdempotencyKey();
        }
        const fullHeaders = { ...headers, ...opts.headers };

        // 设置单次请求超时 AbortController
        const controller = new AbortController();
        const timeoutId = setTimeout(() => controller.abort(), timeoutMs);

        try {
          // Step 1.3.2: 发起真实网络调用
          // 来源: 拼装好的全量 headers、序列化后的 body 及 abort signal
          // 用于: 接收网关或微服务返回的 Response
          const res = await fetch(API_BASE + path, {
            ...opts,
            headers: fullHeaders,
            signal: controller.signal,
            body: opts.body ? (typeof opts.body === 'string' ? opts.body : JSON.stringify(opts.body)) : undefined,
          });
          clearTimeout(timeoutId);

          // 1. 401 鉴权失效（非认证接口）尝试刷新一次 Token
          if (res.status === 401 && !path.startsWith('/auth/')) {
            await this.refresh();
            return this.req(path, { ...opts, headers: fullHeaders }, retryCount);
          }

          // 2. HTTP 4xx 客户端错误（除 429 限流外）：重试必败，立即中断抛出异常
          if (res.status >= 400 && res.status < 500 && res.status !== 429) {
            let errData = null;
            try { errData = await res.json(); } catch (_) {}
            throw new Error(errData?.msg || ('客户端请求错误 HTTP ' + res.status));
          }

          // 3. HTTP 429 限流处理
          if (res.status === 429) {
            if (allowAutoRetry && retryCount < maxRetries) {
              const retryAfter = parseInt(res.headers.get('Retry-After') || '1', 10);
              const delay = isNaN(retryAfter) ? calculateRetryDelay(retryCount) : retryAfter * 1000;
              await new Promise(r => setTimeout(r, delay));
              return this.req(path, { ...opts, headers: fullHeaders }, retryCount + 1);
            }
            throw new Error('操作过于频繁，请稍候再试');
          }

          // 4. HTTP 5xx 服务端错误：在重试配额内执行 Full Jitter 退避重试
          if (res.status >= 500) {
            if (allowAutoRetry && retryCount < maxRetries) {
              const delay = calculateRetryDelay(retryCount);
              await new Promise(r => setTimeout(r, delay));
              return this.req(path, { ...opts, headers: fullHeaders }, retryCount + 1);
            }
            let errData = null;
            try { errData = await res.json(); } catch (_) {}
            throw new Error(errData?.msg || ('服务繁忙，请稍后重试 (HTTP ' + res.status + ')'));
          }

          const data = await res.json();

          // 5. 业务处理成功
          if (data.code === 1000) return data.data;

          // 6. 业务级瞬态服务繁忙 (1005)：在配额内允许重试
          if (data.code === 1005 && allowAutoRetry && retryCount < maxRetries) {
            const delay = calculateRetryDelay(retryCount);
            await new Promise(r => setTimeout(r, delay));
            return this.req(path, { ...opts, headers: fullHeaders }, retryCount + 1);
          }

          // 7. 业务状态码 1006 未授权（非认证接口）：刷新 Token
          if (data.code === 1006 && !path.startsWith('/auth/')) {
            await this.refresh();
            return this.req(path, { ...opts, headers: fullHeaders }, retryCount);
          }

          // 8. 具体业务逻辑报错 (1001 参数错误, 1002 已存在, 1004 凭据错误等)：立即中断
          throw new Error(data.msg || '业务处理失败');

        } catch (err) {
          clearTimeout(timeoutId);
          // 判定是否属于网络中断或超时
          const isNetworkError = err instanceof TypeError;
          const isTimeout = err.name === 'AbortError';

          if ((isNetworkError || isTimeout) && allowAutoRetry && retryCount < maxRetries) {
            const delay = calculateRetryDelay(retryCount);
            await new Promise(r => setTimeout(r, delay));
            return this.req(path, { ...opts, headers: fullHeaders }, retryCount + 1);
          }
          if (isTimeout) {
            throw new Error('网络请求超时，请检查连接后重试');
          }
          throw err;
        }
      },
      async refresh() {
        const rt = localStorage.getItem('refresh_token');
        if (!rt) { App.logout(); throw new Error('登录已过期'); }
        try {
          const res = await fetch(API_BASE + '/auth/refresh', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ refresh_token: rt }),
          });
          const d = await res.json();
          if (d.code === 1000) {
            localStorage.setItem('access_token', d.data.access_token);
            return d.data.access_token;
          }
        } catch { }
        App.logout();
        throw new Error('登录已过期，请重新登录');
      },
      login(username, password) {
        return this.req('/auth/login', { method: 'POST', body: { username, password } });
      },
      register(username, password) {
        return this.req('/auth/register', { method: 'POST', body: { username, password } });
      },
      logout() { return this.req('/auth/logout', { method: 'POST' }).catch(() => { }); },
      getUserInfo() { return this.req('/user/info'); },
      updateUserInfo(data) { return this.req('/user/info', { method: 'PUT', body: { uuid: App.me.uuid, ...data } }); },
      uploadAvatar(file) {
        return new Promise(async (resolve, reject) => {
          const fd = new FormData();
          fd.append('avatar', file);
          try {
            const res = await fetch(API_BASE + '/upload/avatar', {
              method: 'POST',
              headers: { Authorization: 'Bearer ' + localStorage.getItem('access_token') },
              body: fd,
            });
            const d = await res.json();
            if (d.code === 1000) resolve(d.data);
            else reject(new Error(d.msg || '上传失败'));
          } catch (e) { reject(e); }
        });
      },
      getFriendList() { return this.req('/friends'); },
      addFriend(friendId) { return this.req('/friends/apply', { method: 'POST', body: { friend_id: friendId } }); },
      deleteFriend(friendId) { return this.req('/friends?friend_id=' + encodeURIComponent(friendId), { method: 'DELETE' }); },
      getGroups() { return this.req('/groups/joined'); },
      createGroup(name) { return this.req('/groups', { method: 'POST', body: { group_name: name } }); },
      getDirectSessions() { return this.req('/sessions/direct'); },
      getGroupSessions() { return this.req('/sessions/group'); },
      openSession(receiveId) { return this.req('/sessions', { method: 'POST', body: { receive_id: receiveId } }); },
      getMessages(targetId, page = 1, pageSize = 20) {
        return this.req('/messages?target_id=' + encodeURIComponent(targetId) + '&page=' + page + '&page_size=' + pageSize);
      },
      syncMessages(targetId, afterSeq = 0, pageSize = 50) {
        return this.req('/messages?target_id=' + encodeURIComponent(targetId) + '&cursor=after_seq:' + afterSeq + '&page_size=' + pageSize);
      },
    };

    /* ==================== WebSocket ==================== */
    const WS = {
      conn: null, reconnectTimer: null, attempts: 0, maxAttempts: 10, hbTimer: null,
      connect() {
        if (this.conn) this.conn.close();
        const uid = App.me?.uuid;
        if (!uid) return;
        const tok = localStorage.getItem('access_token');
        try {
          this.conn = new WebSocket(wsBaseUrl() + '/ws', ['Bearer', tok]);
        } catch {
          this.conn = new WebSocket(wsBaseUrl() + '/ws?token=' + tok);
        }
        this.conn.onopen = () => {
          this.attempts = 0;
          this.startHB();
          App.setStatus('已连接');
          App.onReconnected();
        };
        this.conn.onmessage = (e) => {
          try {
            const d = JSON.parse(e.data);
            if (d.type === 'kick') { App.showToast('您的账号在其他地方登录', 'error'); App.logout(); return; }
            if (d.type === 'pong') return;
            if (d.type === 'sync_resp') { App.onSyncResp(d); return; }
            if (d.content || d.send_id) App.onIncoming(d);
          } catch { }
        };
        this.conn.onclose = () => { this.stopHB(); App.setStatus('已断开'); this.reconnect(); };
        this.conn.onerror = () => { };
      },
      sync(targetId, afterSeq = 0, limit = 50) {
        return this.send({
          action: 'sync',
          target_id: targetId,
          after_seq: afterSeq,
          limit: limit,
        });
      },
      send(d) {
        if (this.conn && this.conn.readyState === WebSocket.OPEN) { this.conn.send(JSON.stringify(d)); return true; }
        return false;
      },
      startHB() { this.stopHB(); this.hbTimer = setInterval(() => { if (this.conn?.readyState === 1) this.conn.send(JSON.stringify({ type: 'ping' })); }, 30000); },
      stopHB() { if (this.hbTimer) { clearInterval(this.hbTimer); this.hbTimer = null; } },
      reconnect() {
        if (this.attempts >= this.maxAttempts) { App.setStatus('连接失败'); return; }
        this.attempts++;
        this.reconnectTimer = setTimeout(() => { if (localStorage.getItem('access_token')) this.connect(); }, Math.min(1000 * Math.pow(2, this.attempts), 30000));
      },
      disconnect() {
        this.stopHB();
        if (this.reconnectTimer) { clearTimeout(this.reconnectTimer); this.reconnectTimer = null; }
        if (this.conn) { this.conn.close(); this.conn = null; }
      },
    };

    /* ==================== App ==================== */
    const App = {
      me: null,
      session: null,
      sessions: [],
      friends: [],
      groups: [],
      msgs: [],
      msgPage: 1,
      hasMore: true,
      loadingMsgs: false,

      async init() {
        if (typeof document !== 'undefined') {
          document.addEventListener('visibilitychange', () => {
            if (document.visibilityState === 'visible') {
              this.onForegroundVisible();
            }
          });
        }
        const tok = localStorage.getItem('access_token');
        if (tok) {
          try { await this.loadMe(); this.showApp(); } catch { this.showLogin(); }
        } else { this.showLogin(); }
      },

      showLogin() {
        document.getElementById('login-page').style.display = 'flex';
        document.getElementById('app-page').style.display = 'none';
        document.getElementById('login-form').classList.remove('hidden');
        document.getElementById('register-form').classList.add('hidden');
        document.getElementById('register-link').classList.add('hidden');
        document.getElementById('auth-subtitle').textContent = '登录以开始聊天';
      },

      showRegister() {
        document.getElementById('login-form').classList.add('hidden');
        document.getElementById('register-form').classList.remove('hidden');
        document.getElementById('register-link').classList.remove('hidden');
        document.getElementById('auth-subtitle').textContent = '注册新账号';
        document.getElementById('login-error').classList.remove('show');
      },

      showApp() {
        document.getElementById('login-page').style.display = 'none';
        document.getElementById('app-page').style.display = 'block';
        this.updateSidebar();
        this.loadSessions();
        this.loadFriends();
        this.loadGroups();
        WS.connect();
      },

      async loadMe() {
        const d = await api.getUserInfo();
        this.me = { uuid: d.uuid, nickname: d.nickname, telephone: d.telephone, avatar: d.avatar, email: d.email, signature: d.signature, birthday: d.birthday };
        this.updateSidebar();
      },

      updateSidebar() {
        if (!this.me) return;
        const av = document.getElementById('sidebar-avatar');
        av.innerHTML = this.me.avatar
          ? '<img src="' + avatarUrl(this.me.avatar) + '" alt="">'
          : (this.me.nickname || '?').charAt(0).toUpperCase();
        document.getElementById('sidebar-username').textContent = this.me.nickname || '用户';
      },

      /* --- Auth --- */
      async login() {
        const username = document.getElementById('login-username').value.trim();
        const pwd = document.getElementById('login-password').value;
        const errEl = document.getElementById('login-error');
        const btn = document.getElementById('login-btn');
        const btnText = document.getElementById('login-btn-text');
        const spinner = document.getElementById('login-spinner');
        if (!username || !pwd) { errEl.textContent = '请输入用户名和密码'; errEl.classList.add('show'); return; }
        btn.disabled = true; btnText.classList.add('hidden'); spinner.classList.remove('hidden'); errEl.classList.remove('show');
        try {
          const d = await api.login(username, pwd);
          localStorage.setItem('access_token', d.access_token);
          localStorage.setItem('refresh_token', d.refresh_token);
          this.me = { uuid: d.uuid, nickname: d.nickname, telephone: d.telephone, avatar: d.avatar, email: d.email, signature: d.signature, birthday: d.birthday };
          this.showApp();
          this.showToast('登录成功', 'success');
        } catch (e) { errEl.textContent = e.message || '登录失败'; errEl.classList.add('show'); }
        finally { btn.disabled = false; btnText.classList.remove('hidden'); spinner.classList.add('hidden'); }
      },

      async register() {
        const username = document.getElementById('register-username').value.trim();
        const pwd = document.getElementById('register-password').value;
        const confirm = document.getElementById('register-confirm').value;
        const errEl = document.getElementById('login-error');
        const btn = document.getElementById('register-btn');
        const btnText = document.getElementById('register-btn-text');
        const spinner = document.getElementById('register-spinner');
        if (!username || !pwd || !confirm) { errEl.textContent = '请填写所有字段'; errEl.classList.add('show'); return; }
        if (pwd !== confirm) { errEl.textContent = '两次输入的密码不一致'; errEl.classList.add('show'); return; }
        if (username.length < 2 || username.length > 32) { errEl.textContent = '用户名长度为2-32个字符'; errEl.classList.add('show'); return; }
        if (pwd.length < 6 || pwd.length > 32) { errEl.textContent = '密码长度为6-32个字符'; errEl.classList.add('show'); return; }
        btn.disabled = true; btnText.classList.add('hidden'); spinner.classList.remove('hidden'); errEl.classList.remove('show');
        try {
          const d = await api.register(username, pwd);
          localStorage.setItem('access_token', d.access_token);
          localStorage.setItem('refresh_token', d.refresh_token);
          this.me = { uuid: d.uuid, nickname: d.nickname, telephone: d.telephone, avatar: d.avatar, email: d.email, signature: d.signature, birthday: d.birthday };
          this.showApp();
          this.showToast('注册成功', 'success');
        } catch (e) { errEl.textContent = e.message || '注册失败'; errEl.classList.add('show'); }
        finally { btn.disabled = false; btnText.classList.remove('hidden'); spinner.classList.add('hidden'); }
      },

      async logout() {
        // 1. 先调用 WebSocket 登出（此时 Token 还有效）
        const tok = localStorage.getItem('access_token');
        if (tok) {
          try {
            await fetch(wsBaseUrl() + '/ws/logout', {
              method: 'POST',
              headers: { 'Authorization': 'Bearer ' + tok }
            });
          } catch (e) {
            console.warn('WebSocket 登出失败:', e);
          }
        }

        // 2. 再调用 HTTP 登出使 Token 失效
        try { await api.logout(); } catch { }

        // 3. 清理本地状态
        WS.disconnect();
        localStorage.removeItem('access_token');
        localStorage.removeItem('refresh_token');
        this.me = null; this.session = null; this.sessions = []; this.friends = []; this.groups = []; this.msgs = [];
        this.showLogin();
        this.showToast('已退出登录', 'info');
      },

      /* --- Profile --- */
      showProfile() {
        if (!this.me) return;
        const av = document.getElementById('profile-avatar');
        av.innerHTML = this.me.avatar ? '<img src="' + avatarUrl(this.me.avatar) + '" alt="">' : (this.me.nickname || '?').charAt(0).toUpperCase();
        document.getElementById('profile-name').textContent = this.me.nickname || '未设置昵称';
        document.getElementById('profile-uuid').textContent = 'ID: ' + (this.me.uuid || '');
        document.getElementById('profile-phone').textContent = this.me.telephone || '';
        document.getElementById('profile-nickname-input').value = this.me.nickname || '';
        document.getElementById('profile-email-input').value = this.me.email || '';
        document.getElementById('profile-signature-input').value = this.me.signature || '';
        document.getElementById('profile-birthday-input').value = this.me.birthday || '';
        document.getElementById('profile-modal').classList.remove('hidden');
      },
      closeProfile() { document.getElementById('profile-modal').classList.add('hidden'); },
      async saveProfile() {
        const nickname = document.getElementById('profile-nickname-input').value.trim();
        const email = document.getElementById('profile-email-input').value.trim();
        const signature = document.getElementById('profile-signature-input').value.trim();
        const birthday = document.getElementById('profile-birthday-input').value.trim();
        try {
          const data = {};
          if (nickname) data.nickname = nickname;
          data.email = email; data.signature = signature; data.birthday = birthday;
          await api.updateUserInfo(data);
          this.me.nickname = nickname || this.me.nickname;
          this.me.email = email; this.me.signature = signature; this.me.birthday = birthday;
          this.updateSidebar(); this.closeProfile();
          this.showToast('资料已更新', 'success');
        } catch (e) { this.showToast(e.message || '更新失败', 'error'); }
      },
      async uploadAvatar(input) {
        const file = input.files[0]; if (!file) return;
        try {
          const d = await api.uploadAvatar(file);
          const av = d.avatar || d;
          this.me.avatar = av; this.updateSidebar();
          document.getElementById('profile-avatar').innerHTML = '<img src="' + avatarUrl(av) + '" alt="">';
          this.showToast('头像已更新', 'success');
        } catch (e) { this.showToast(e.message || '上传失败', 'error'); }
        input.value = '';
      },

      /* --- Tabs --- */
      switchTab(tab) {
        document.querySelectorAll('.sidebar-tab').forEach(t => t.classList.toggle('active', t.dataset.tab === tab));
        ['sessions', 'friends', 'groups'].forEach(t => {
          document.getElementById('tab-' + t).classList.toggle('hidden', t !== tab);
        });
      },

      /* --- Sessions --- */
      async loadSessions() {
        try {
          const [direct, group] = await Promise.all([api.getDirectSessions(), api.getGroupSessions()]);
          this.sessions = [];
          if (direct?.length) direct.forEach(s => this.sessions.push({ sessionId: s.session_id, type: 'direct', targetId: s.user_id, name: s.user_name, avatar: s.avatar, lastMessage: s.last_message, lastTime: s.last_message_time, isPinned: s.is_pinned }));
          if (group?.length) group.forEach(s => this.sessions.push({ sessionId: s.session_id, type: 'group', targetId: s.group_id, name: s.group_name, avatar: s.avatar, lastMessage: s.last_message, lastTime: s.last_message_time, isPinned: s.is_pinned }));
          this.sessions.sort((a, b) => { if (a.isPinned && !b.isPinned) return -1; if (!a.isPinned && b.isPinned) return 1; return new Date(b.lastTime || 0) - new Date(a.lastTime || 0); });
          this.renderSessions();
        } catch (e) { console.error('loadSessions:', e); }
      },
      renderSessions(filter = '') {
        const c = document.getElementById('session-list');
        let list = this.sessions;
        if (filter) { const f = filter.toLowerCase(); list = list.filter(s => s.name.toLowerCase().includes(f)); }
        if (!list.length) { c.innerHTML = '<div class="empty-list">暂无会话</div>'; return; }
        c.innerHTML = list.map(s => {
          const avHtml = s.avatar ? '<img src="' + avatarUrl(s.avatar) + '" alt="">' : esc(s.name).charAt(0).toUpperCase();
          const active = this.session?.sessionId === s.sessionId ? ' active' : '';
          return '<div class="session-item' + active + '" onclick="App.openSession(\'' + esc(s.sessionId) + '\',\'' + s.type + '\',\'' + esc(s.targetId) + '\',\'' + esc(s.name) + '\',\'' + esc(s.avatar || '') + '\')">' +
            '<div class="session-avatar">' + avHtml + '</div>' +
            '<div class="session-info"><div class="session-name truncate">' + esc(s.name) + '</div><div class="session-last-msg truncate">' + esc(s.lastMessage || '暂无消息') + '</div></div>' +
            '<div class="session-meta"><div class="session-time">' + fmtTime(s.lastTime) + '</div></div></div>';
        }).join('');
      },
      filterSessions(v) { this.renderSessions(v); },

      /* --- Friends --- */
      async loadFriends() {
        try { this.friends = await api.getFriendList() || []; this.renderFriends(); } catch (e) { console.error('loadFriends:', e); }
      },
      renderFriends() {
        const c = document.getElementById('friend-list');
        if (!this.friends.length) { c.innerHTML = '<div class="empty-list">暂无好友</div>'; return; }
        c.innerHTML = this.friends.map(f => {
          const avHtml = f.friend_avatar ? '<img src="' + avatarUrl(f.friend_avatar) + '" alt="">' : (f.friend_name || '?').charAt(0).toUpperCase();
          return '<div class="list-item"><div class="list-item-avatar">' + avHtml + '</div>' +
            '<div class="list-item-info"><div class="list-item-name">' + esc(f.remark || f.friend_name) + '</div><div class="list-item-desc">' + esc(f.friend_phone || '') + '</div></div>' +
            '<div class="list-item-actions"><button class="btn btn-primary btn-sm" onclick="App.chatWithFriend(\'' + esc(f.friend_id) + '\',\'' + esc(f.friend_name) + '\',\'' + esc(f.friend_avatar || '') + '\')">聊天</button>' +
            '<button class="btn btn-secondary btn-sm" onclick="App.removeFriend(\'' + esc(f.friend_id) + '\')">删除</button></div></div>';
        }).join('');
      },
      // Step 2.1: 按钮 Loading 与防连击状态执行器
      // 来源: 用户交互触发的按钮元素 btn 及异步业务闭包 fn
      // 用于: 在请求周期内锁定按钮防连击并渲染转圈 Spinner，结束后安全恢复状态
      async withButtonLoading(btn, fn, loadingText = '处理中...') {
        const targetBtn = btn || (window.event?.target && window.event.target.tagName === 'BUTTON' ? window.event.target : null);
        if (!targetBtn) {
          return fn();
        }
        if (targetBtn.disabled) return;
        const oldHtml = targetBtn.innerHTML;
        targetBtn.disabled = true;
        targetBtn.classList.add('btn-loading');
        targetBtn.innerHTML = `<span class="btn-spinner"></span>${loadingText}`;
        try {
          await fn();
        } finally {
          targetBtn.disabled = false;
          targetBtn.classList.remove('btn-loading');
          targetBtn.innerHTML = oldHtml;
        }
      },

      // Step 2.2: 申请添加好友 (锁定按钮防连击，失败保留输入)
      // 来源: 输入框 #add-friend-input
      // 用于: 调用 api.addFriend 发起好友申请
      async addFriend(btn) {
        const input = document.getElementById('add-friend-input');
        const v = input ? input.value.trim() : '';
        if (!v) { this.showToast('请输入用户ID', 'error'); return; }
        await this.withButtonLoading(btn, async () => {
          try {
            await api.addFriend(v);
            if (input) input.value = '';
            this.showToast('好友申请已发送', 'success');
            this.loadFriends();
          } catch (e) {
            // 失败时保留输入框文本，方便用户重试
            this.showToast(e.message || '申请发送失败，请重试', 'error');
          }
        }, '提交中...');
      },
      async removeFriend(id) {
        if (!confirm('确定删除该好友？')) return;
        try { await api.deleteFriend(id); this.showToast('已删除好友', 'success'); this.loadFriends(); }
        catch (e) { this.showToast(e.message || '删除失败', 'error'); }
      },

      /* --- Groups --- */
      async loadGroups() {
        try { this.groups = await api.getGroups() || []; this.renderGroups(); } catch (e) { console.error('loadGroups:', e); }
      },
      renderGroups() {
        const c = document.getElementById('group-list');
        if (!this.groups.length) { c.innerHTML = '<div class="empty-list">暂无群组</div>'; return; }
        c.innerHTML = this.groups.map(g => {
          const avHtml = g.avatar ? '<img src="' + avatarUrl(g.avatar) + '" alt="">' : (g.groupName || '?').charAt(0).toUpperCase();
          return '<div class="list-item"><div class="list-item-avatar">' + avHtml + '</div>' +
            '<div class="list-item-info"><div class="list-item-name">' + esc(g.groupName) + '</div></div>' +
            '<div class="list-item-actions"><button class="btn btn-primary btn-sm" onclick="App.chatWithGroup(\'' + esc(g.groupId) + '\',\'' + esc(g.groupName) + '\',\'' + esc(g.avatar || '') + '\')">聊天</button></div></div>';
        }).join('');
      },

      // Step 2.3: 创建群组 (锁定等待模式，防止网络抖动重复创建重影群)
      // 来源: 输入框 #create-group-input
      // 用于: 调用 api.createGroup 创建群组
      async createGroup(btn) {
        const input = document.getElementById('create-group-input');
        const v = input ? input.value.trim() : '';
        if (!v) { this.showToast('请输入群组名称', 'error'); return; }
        await this.withButtonLoading(btn, async () => {
          try {
            await api.createGroup(v);
            if (input) input.value = '';
            this.showToast('群组已创建', 'success');
            this.loadGroups();
            this.loadSessions();
          } catch (e) {
            // 失败时保留已输入的群名称，弹出友好错误提示
            this.showToast(e.message || '群组创建超时，请点击重试', 'error');
          }
        }, '创建中...');
      },

      /* --- Chat --- */
      async openSession(sessionId, type, targetId, name, avatar) {
        this.session = { sessionId, type, targetId, name, avatar };
        document.getElementById('chat-empty').style.display = 'none';
        const ca = document.getElementById('chat-active');
        ca.classList.remove('hidden'); ca.style.display = 'flex';
        const caEl = document.getElementById('chat-avatar');
        caEl.innerHTML = avatar ? '<img src="' + avatarUrl(avatar) + '" alt="">' : esc(name).charAt(0).toUpperCase();
        document.getElementById('chat-name').textContent = name;
        document.getElementById('chat-status').textContent = type === 'group' ? '群聊' : '在线';
        document.getElementById('chat-area').classList.add('active');
        document.getElementById('sidebar').classList.add('chat-open');
        this.renderSessions();
        this.msgs = []; this.msgPage = 1; this.hasMore = true;
        await this.loadMsgs();
      },
      closeChat() {
        document.getElementById('chat-area').classList.remove('active');
        document.getElementById('sidebar').classList.remove('chat-open');
      },
      async loadMsgs() {
        if (!this.session || this.loadingMsgs || !this.hasMore) return;
        this.loadingMsgs = true;
        const c = document.getElementById('chat-messages');
        try {
          const res = await api.getMessages(this.session.targetId, this.msgPage, 20);
          const data = res?.list || res;
          if (data?.length) {
            const oldH = c.scrollHeight;
            const oldT = c.scrollTop;
            const reversed = [...data].reverse();
            c.insertAdjacentHTML('afterbegin', reversed.map(m => this.renderMsg(m)).join(''));
            c.scrollTop = c.scrollHeight - oldH + oldT;
            this.msgPage++;
            if (data.length < 20) this.hasMore = false;

            // 同步维护内存消息列表与 GapHoldingBuffer 水位
            for (const m of data) {
              if (!this.msgs.some(existing => (m.seq_id && existing.seq_id === m.seq_id) || (m.client_msg_id && existing.client_msg_id === m.client_msg_id))) {
                this.msgs.push(m);
              }
            }
            let maxSeq = 0;
            for (const m of this.msgs) {
              if (m.seq_id && m.seq_id > maxSeq) maxSeq = m.seq_id;
            }
            const buf = this.getOrCreateGapBuffer(this.session.targetId);
            buf.setWatermark(maxSeq);
          } else { this.hasMore = false; }
        } catch (e) { console.error('loadMsgs:', e); }
        finally { this.loadingMsgs = false; }
      },

      // Step 2.4: 渲染聊天单条消息 (支持发送中转圈与失败红感叹号)
      // 来源: 历史记录或乐观发送消息对象 m
      // 用于: 动态生成带交互状态的聊天气泡 HTML
      renderMsg(m) {
        const self = m.send_id === this.me.uuid;
        const av = m.send_avatar;
        const sender = m.send_name || (self ? '我' : '对方');
        let content = '';
        switch (m.type) {
          case 1: content = '<div class="message-bubble">' + esc(m.content) + '</div>'; break;
          case 2: content = '<div class="message-bubble"><img class="message-image" src="' + fileUrl(m.url) + '" onclick="App.previewImg(\'' + fileUrl(m.url) + '\')" alt="image"></div>'; break;
          case 3: content = '<div class="message-bubble"><a class="message-file" href="' + fileUrl(m.url) + '" target="_blank" download="' + esc(m.file_name || 'file') + '">&#128206; ' + esc(m.file_name || '文件') + '</a></div>'; break;
          default: content = '<div class="message-bubble">' + esc(m.content || '[未知消息]') + '</div>';
        }
        const avHtml = av ? '<img src="' + avatarUrl(av) + '" alt="">' : sender.charAt(0).toUpperCase();
        const senderHtml = !self && this.session?.type === 'group' ? '<div class="message-sender">' + esc(sender) + '</div>' : '';

        // 发送状态指示器：发送中展示微型 Spinner，失败展示红色感叹号
        let statusHtml = '';
        if (m.client_msg_id && m.status === 'sending') {
          statusHtml = `<span class="msg-status sending" id="msg-status-${m.client_msg_id}" title="发送中..."></span>`;
        } else if (m.client_msg_id && m.status === 'failed') {
          statusHtml = `<span class="msg-status failed" id="msg-status-${m.client_msg_id}" title="发送失败，点击重试" onclick="App.retryMessage('${m.client_msg_id}')">&#9888;</span>`;
        }

        return '<div class="message ' + (self ? 'sent' : 'received') + '">' +
          '<div class="message-avatar">' + avHtml + '</div>' +
          '<div class="message-content">' + senderHtml + content + '<div class="message-time">' + fmtMsgTime(m.created_at) + statusHtml + '</div></div></div>';
      },

      // Step 2.5: 发送聊天消息 (乐观更新，不锁定输入框，自动与手动重试协同)
      // 来源: 输入框 #chat-input 的文本内容
      // 用于: 消息先立即假上屏，随后异步通过 WS 或 HTTP 提交
      async sendMessage() {
        const input = document.getElementById('chat-input');
        const content = input ? input.value.trim() : '';
        if (!content || !this.session) return;

        // Step 2.5.1: 生成全周期唯一消息防重标识 client_msg_id
        // 来源: 时间戳与随机字符串
        // 用于: 消息重试全生命周期幂等凭据
        const client_msg_id = Date.now() + '-' + Math.random().toString(36).substring(2, 9);
        this.pendingMessages = this.pendingMessages || {};
        this.pendingMessages[client_msg_id] = {
          sessionId: this.session.sessionId,
          type: 0,
          content,
          targetId: this.session.targetId,
          client_msg_id,
        };

        // Step 2.5.2: 乐观渲染上屏 (附带 status: 'sending')，清空输入框，输入框绝不禁用
        const c = document.getElementById('chat-messages');
        c.insertAdjacentHTML('beforeend', this.renderMsg({
          send_id: this.me.uuid,
          send_name: this.me.nickname,
          send_avatar: this.me.avatar,
          type: 0,
          content,
          created_at: new Date().toISOString(),
          client_msg_id,
          status: 'sending',
        }));
        this.scrollBottom();
        if (input) {
          input.value = '';
          this.autoResizeInput(input);
        }

        // Step 2.5.3: 异步分发发送流程
        this.dispatchMessage(client_msg_id);
      },

      // Step 2.6: 异步派发消息 (WebSocket 优先，HTTP 兜底并自动退避重试)
      // 来源: Step 2.5 生成的 client_msg_id
      // 用于: 执行实际网络调用，成功则清除转圈，失败则挂载红感叹号
      async dispatchMessage(client_msg_id) {
        const pending = this.pendingMessages?.[client_msg_id];
        if (!pending) return;

        const statusEl = document.getElementById(`msg-status-${client_msg_id}`);
        if (statusEl) {
          statusEl.className = 'msg-status sending';
          statusEl.title = '发送中...';
          statusEl.innerHTML = '';
          statusEl.onclick = null;
        }

        try {
          const wsSent = WS.send({
            session_id: pending.sessionId,
            type: pending.type,
            content: pending.content,
            send_id: this.me.uuid,
            send_name: this.me.nickname,
            send_avatar: this.me.avatar,
            receive_id: pending.targetId,
            client_msg_id,
          });

          if (!wsSent) {
            // WS 未连接时使用 HTTP POST 兜底，传递完全相同的 client_msg_id
            await api.req('/messages/send', {
              method: 'POST',
              body: {
                session_id: pending.sessionId,
                type: pending.type,
                content: pending.content,
                send_id: this.me.uuid,
                send_name: this.me.nickname,
                send_avatar: this.me.avatar,
                receive_id: pending.targetId,
                client_msg_id,
              },
              headers: {
                'X-Idempotency-Key': client_msg_id,
              },
            });
          }

          // 成功处理：清除 loading 指示器与 pending 暂存
          if (statusEl) statusEl.remove();
          delete this.pendingMessages[client_msg_id];
        } catch (err) {
          // 彻底失败：转为红色感叹号，绑定点击原地重试
          if (statusEl) {
            statusEl.className = 'msg-status failed';
            statusEl.title = '发送失败，点击重试';
            statusEl.innerHTML = '&#9888;';
            statusEl.onclick = () => this.retryMessage(client_msg_id);
          }
          this.showToast('消息发送失败，可点击红色感叹号重新发送', 'error');
        }
      },

      // Step 2.7: 用户点击红感叹号手动重试
      // 来源: 用户点击 #msg-status-${client_msg_id} 触发
      // 用于: 复用原有 client_msg_id 再次尝试派发，防止重复生成消息
      retryMessage(client_msg_id) {
        this.dispatchMessage(client_msg_id);
      },

      getOrCreateGapBuffer(targetId) {
        if (!this.gapBuffers) this.gapBuffers = new Map();
        if (!this.gapBuffers.has(targetId)) {
          let maxSeq = 0;
          if (this.msgs && this.msgs.length) {
            for (const m of this.msgs) {
              if (m.seq_id && m.seq_id > maxSeq) {
                maxSeq = m.seq_id;
              }
            }
          }
          const buf = new GapHoldingBuffer(maxSeq, (releasedMsg) => {
            this.appendMessageToUI(releasedMsg);
          });
          this.gapBuffers.set(targetId, buf);
        }
        return this.gapBuffers.get(targetId);
      },

      appendMessageToUI(d) {
        if (d.seq_id && this.msgs.some(m => m.seq_id === d.seq_id)) return;
        if (d.client_msg_id && this.msgs.some(m => m.client_msg_id === d.client_msg_id)) return;

        this.msgs.push(d);

        const isCurrent = this.session && (
          d.session_id === this.session.sessionId ||
          d.receive_id === this.session.targetId ||
          d.send_id === this.session.targetId
        );
        if (isCurrent && document.getElementById('chat-active').style.display !== 'none') {
          document.getElementById('chat-messages').insertAdjacentHTML('beforeend', this.renderMsg(d));
          this.scrollBottom();
        }

        const s = this.sessions.find(s => s.sessionId === d.session_id || s.targetId === d.receive_id || s.targetId === d.send_id);
        if (s) {
          s.lastMessage = d.content || (d.type === 2 ? '[图片]' : d.type === 3 ? '[文件]' : '');
          s.lastTime = d.created_at || new Date().toISOString();
          this.renderSessions();
        } else {
          this.loadSessions();
        }
      },

      async syncSessionDelta(targetId, watermark) {
        if (!targetId) return;
        try {
          const res = await api.syncMessages(targetId, watermark, 50);
          const list = res?.list || res || [];
          if (list && list.length > 0) {
            const buf = this.getOrCreateGapBuffer(targetId);
            buf.reconcileDelta(list);
          }
        } catch (e) {
          console.warn('[SyncDelta] error reconciling delta via HTTP, trying WS sync:', e);
          WS.sync(targetId, watermark, 50);
        }
      },

      onSyncResp(d) {
        if (!d || !d.target_id) return;
        const list = d.messages || [];
        if (list.length > 0) {
          const buf = this.getOrCreateGapBuffer(d.target_id);
          buf.reconcileDelta(list);
        }
      },

      onReconnected() {
        if (this.session?.targetId) {
          const buf = this.getOrCreateGapBuffer(this.session.targetId);
          this.syncSessionDelta(this.session.targetId, buf.getWatermark());
        }
        this.loadSessions();
      },

      onForegroundVisible() {
        if (this.session?.targetId) {
          const buf = this.getOrCreateGapBuffer(this.session.targetId);
          this.syncSessionDelta(this.session.targetId, buf.getWatermark());
        }
        this.loadSessions();
      },

      onIncoming(d) {
        if (d.client_msg_id) {
          const statusEl = document.getElementById(`msg-status-${d.client_msg_id}`);
          if (statusEl) statusEl.remove();
          if (this.pendingMessages?.[d.client_msg_id]) {
            delete this.pendingMessages[d.client_msg_id];
          }
        }

        // 己方已乐观上屏的消息
        if (d.send_id === this.me?.uuid && d.client_msg_id) {
          const existing = this.msgs.find(m => m.client_msg_id === d.client_msg_id);
          if (existing && d.seq_id) {
            existing.seq_id = d.seq_id;
            const targetId = this.session?.targetId || d.receive_id;
            if (targetId) {
              const buf = this.getOrCreateGapBuffer(targetId);
              buf.setWatermark(d.seq_id);
            }
          }
          return;
        }

        const targetId = (d.receive_id && d.receive_id[0] === 'G') ? d.receive_id : (d.send_id === this.me?.uuid ? d.receive_id : d.send_id);
        if (!targetId) {
          this.appendMessageToUI(d);
          return;
        }

        const buf = this.getOrCreateGapBuffer(targetId);
        const ok = buf.push(d);

        // 若产生空洞断层 (如期望 5 却收到 8)，向服务端拉取增量差异进行自愈
        if (!ok && buf.hasGap()) {
          this.syncSessionDelta(targetId, buf.getWatermark());
        }
      },
      handleInputKeydown(e) { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); this.sendMessage(); } },
      autoResizeInput(ta) { ta.style.height = 'auto'; ta.style.height = Math.min(ta.scrollHeight, 120) + 'px'; },
      scrollBottom() { const c = document.getElementById('chat-messages'); requestAnimationFrame(() => { c.scrollTop = c.scrollHeight; }); },

      /* --- Chat shortcuts --- */
      async chatWithFriend(fid, name, av) {
        try {
          await api.openSession(fid);
          await this.loadSessions();
          const s = this.sessions.find(s => s.targetId === fid);
          if (s) this.openSession(s.sessionId, s.type, s.targetId, s.name, s.avatar);
        } catch (e) { this.showToast('无法打开会话: ' + e.message, 'error'); }
      },
      async chatWithGroup(gid, name, av) {
        try {
          await api.openSession(gid);
          await this.loadSessions();
          const s = this.sessions.find(s => s.targetId === gid);
          if (s) this.openSession(s.sessionId, s.type, s.targetId, s.name, s.avatar);
        } catch (e) { this.showToast('无法打开会话: ' + e.message, 'error'); }
      },

      /* --- Status --- */
      setStatus(s) {
        const el = document.getElementById('chat-status');
        if (el && this.session?.type === 'direct') el.textContent = s;
      },

      /* --- Image Preview --- */
      previewImg(url) {
        const o = document.createElement('div');
        o.className = 'image-preview-overlay';
        o.innerHTML = '<img src="' + url + '" alt="preview">';
        o.onclick = () => o.remove();
        document.body.appendChild(o);
      },

      /* --- Toast --- */
      showToast(msg, type = 'info') {
        const c = document.getElementById('toast-container');
        const t = document.createElement('div');
        t.className = 'toast ' + type;
        const icons = { success: '&#10003;', error: '&#10007;', info: '&#8505;' };
        t.innerHTML = '<span>' + (icons[type] || '') + '</span><span>' + esc(msg) + '</span>';
        c.appendChild(t);
        setTimeout(() => { t.style.opacity = '0'; t.style.transform = 'translateX(20px)'; t.style.transition = 'all .3s'; setTimeout(() => t.remove(), 300); }, 3000);
      },
    };

    /* ==================== Events ==================== */
    document.getElementById('login-form').addEventListener('submit', e => { e.preventDefault(); App.login(); });
    document.getElementById('register-form').addEventListener('submit', e => { e.preventDefault(); App.register(); });
    document.getElementById('chat-messages').addEventListener('scroll', function () {
      if (this.scrollTop < 50 && App.hasMore && !App.loadingMsgs) App.loadMsgs();
    });
    document.addEventListener('keydown', e => { if (e.key === 'Escape') App.closeProfile(); });

    /* ==================== Init ==================== */
    App.init();
