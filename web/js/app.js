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

    /* ==================== API Client ==================== */
    const api = {
      async req(path, opts = {}) {
        const headers = { 'Content-Type': 'application/json' };
        const tok = localStorage.getItem('access_token');
        if (tok) headers.Authorization = 'Bearer ' + tok;
        const res = await fetch(API_BASE + path, {
          ...opts,
          headers: { ...headers, ...opts.headers },
          body: opts.body ? (typeof opts.body === 'string' ? opts.body : JSON.stringify(opts.body)) : undefined,
        });
        const data = await res.json();
        // code 1000 = success
        if (data.code === 1000) return data.data;
        // token expired / unauthorized
        if (data.code === 1003 || data.code === 1004 || res.status === 401) {
          await this.refresh();
          return this.req(path, opts);
        }
        throw new Error(data.msg || '请求失败');
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
        };
        this.conn.onmessage = (e) => {
          try {
            const d = JSON.parse(e.data);
            if (d.type === 'kick') { App.showToast('您的账号在其他地方登录', 'error'); App.logout(); return; }
            if (d.content || d.send_id) App.onIncoming(d);
          } catch { }
        };
        this.conn.onclose = () => { this.stopHB(); App.setStatus('已断开'); this.reconnect(); };
        this.conn.onerror = () => { };
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
      async addFriend() {
        const v = document.getElementById('add-friend-input').value.trim();
        if (!v) { this.showToast('请输入用户ID', 'error'); return; }
        try { await api.addFriend(v); document.getElementById('add-friend-input').value = ''; this.showToast('好友申请已发送', 'success'); this.loadFriends(); }
        catch (e) { this.showToast(e.message || '添加失败', 'error'); }
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
      async createGroup() {
        const v = document.getElementById('create-group-input').value.trim();
        if (!v) { this.showToast('请输入群组名称', 'error'); return; }
        try { await api.createGroup(v); document.getElementById('create-group-input').value = ''; this.showToast('群组已创建', 'success'); this.loadGroups(); this.loadSessions(); }
        catch (e) { this.showToast(e.message || '创建失败', 'error'); }
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
          const data = await api.getMessages(this.session.targetId, this.msgPage, 20);
          if (data?.length) {
            const oldH = c.scrollHeight;
            const oldT = c.scrollTop;
            c.insertAdjacentHTML('afterbegin', data.reverse().map(m => this.renderMsg(m)).join(''));
            c.scrollTop = c.scrollHeight - oldH + oldT;
            this.msgPage++;
            if (data.length < 20) this.hasMore = false;
          } else { this.hasMore = false; }
        } catch (e) { console.error('loadMsgs:', e); }
        finally { this.loadingMsgs = false; }
      },
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
        return '<div class="message ' + (self ? 'sent' : 'received') + '">' +
          '<div class="message-avatar">' + avHtml + '</div>' +
          '<div class="message-content">' + senderHtml + content + '<div class="message-time">' + fmtMsgTime(m.created_at) + '</div></div></div>';
      },
      async sendMessage() {
        const input = document.getElementById('chat-input');
        const content = input.value.trim();
        if (!content || !this.session) return;
        const btn = document.getElementById('chat-send-btn');
        btn.disabled = true;
        // Optimistic render
        const c = document.getElementById('chat-messages');
        c.insertAdjacentHTML('beforeend', this.renderMsg({ send_id: this.me.uuid, send_name: this.me.nickname, send_avatar: this.me.avatar, type: 1, content, created_at: new Date().toISOString() }));
        this.scrollBottom();
        input.value = ''; this.autoResizeInput(input);
        // Send via WS
        const sent = WS.send({ session_id: this.session.sessionId, type: 1, content, send_id: this.me.uuid, send_name: this.me.nickname, send_avatar: this.me.avatar, receive_id: this.session.targetId, client_msg_id: Date.now() + '-' + Math.random().toString(36).substr(2, 9) });
        if (!sent) {
          try { await api.req('/messages/send', { method: 'POST', body: { session_id: this.session.sessionId, content, msg_type: 1 } }); }
          catch (e) { this.showToast('发送失败: ' + e.message, 'error'); }
        }
        btn.disabled = false;
      },
      onIncoming(d) {
        const isCurrent = d.session_id === this.session?.sessionId || d.receive_id === this.session?.targetId || d.send_id === this.session?.targetId;
        if (isCurrent && document.getElementById('chat-active').style.display !== 'none') {
          document.getElementById('chat-messages').insertAdjacentHTML('beforeend', this.renderMsg(d));
          this.scrollBottom();
        }
        // Update session list
        const s = this.sessions.find(s => s.sessionId === d.session_id);
        if (s) { s.lastMessage = d.content || ''; s.lastTime = d.created_at || new Date().toISOString(); this.renderSessions(); }
        else { this.loadSessions(); }
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
