import React, { useEffect, useState, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { request } from '../api';
import { useWebSocket } from '../hooks/useWebSocket';

function Chat() {
  const navigate = useNavigate();
  const [userInfo, setUserInfo] = useState(null);
  const [friends, setFriends] = useState([]);
  const [activeTab, setActiveTab] = useState('friends');
  const [activeChat, setActiveChat] = useState(null);
  const [inputMsg, setInputMsg] = useState('');
  
  const { messages, sendMessage, status, setMessages } = useWebSocket('/ws');
  const messagesEndRef = useRef(null);

  useEffect(() => {
    const token = localStorage.getItem('token');
    if (!token) {
      navigate('/login');
      return;
    }
    
    request('/user/info').then(res => {
      if (res && res.code === 1000) setUserInfo(res.data);
    });

    request('/friends').then(res => {
      if (res && res.code === 1000) setFriends(res.data.list || []);
    });
  }, [navigate]);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages, activeChat]);

  const handleSend = () => {
    if (!inputMsg.trim() || !activeChat) return;
    
    // Simulate optimistic UI update
    const myId = userInfo?.uuid;
    const optimisticMsg = {
      message_id: Date.now().toString(),
      sender_id: myId,
      receiver_id: activeChat.friend_id,
      content: inputMsg,
      message_type: 1 // Text
    };
    setMessages(prev => [...prev, optimisticMsg]);
    
    sendMessage({
      cmd: 2, // CMD_SEND_MESSAGE
      data: {
        receiver_id: activeChat.friend_id,
        content: inputMsg,
        message_type: 1
      }
    });
    
    setInputMsg('');
  };

  const getChatMessages = () => {
    if (!activeChat || !userInfo) return [];
    return messages.filter(m => 
      (m.sender_id === activeChat.friend_id && m.receiver_id === userInfo.uuid) ||
      (m.sender_id === userInfo.uuid && m.receiver_id === activeChat.friend_id)
    );
  };

  return (
    <div className="glass-panel" style={{ width: '90vw', maxWidth: '1200px', height: '85vh', display: 'flex', overflow: 'hidden' }}>
      {/* Sidebar */}
      <div style={{ width: '300px', borderRight: '1px solid var(--glass-border)', background: 'rgba(255, 255, 255, 0.3)', display: 'flex', flexDirection: 'column' }}>
        <div style={{ padding: '20px', display: 'flex', alignItems: 'center', gap: '12px', borderBottom: '1px solid var(--glass-border)' }}>
          <div style={{ width: '40px', height: '40px', borderRadius: '50%', background: 'var(--primary)', color: 'white', display: 'flex', alignItems: 'center', justifyContent: 'center', fontWeight: 'bold' }}>
            {userInfo?.nickname?.[0] || 'U'}
          </div>
          <div style={{ flex: 1, fontWeight: '600' }}>
            {userInfo?.nickname || '加载中...'}
            <div style={{ fontSize: '12px', color: status === 'connected' ? 'var(--success)' : 'var(--danger)' }}>
              {status === 'connected' ? '在线' : '连接中...'}
            </div>
          </div>
          <button className="btn-icon" onClick={() => { localStorage.clear(); navigate('/login'); }}>退出</button>
        </div>

        <div style={{ display: 'flex', borderBottom: '1px solid var(--glass-border)' }}>
          <div style={{ flex: 1, textAlign: 'center', padding: '12px 0', cursor: 'pointer', fontWeight: activeTab === 'friends' ? 'bold' : 'normal', borderBottom: activeTab === 'friends' ? '2px solid var(--primary)' : '2px solid transparent' }} onClick={() => setActiveTab('friends')}>
            好友
          </div>
        </div>

        <div style={{ flex: 1, overflowY: 'auto', padding: '10px' }}>
          {activeTab === 'friends' && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
              {friends.length === 0 ? <div style={{ textAlign: 'center', padding: '20px', color: 'var(--text-muted)', fontSize: '14px' }}>暂无好友</div> : (
                friends.map(f => (
                  <div key={f.friend_id} onClick={() => setActiveChat(f)} className="btn" style={{ justifyContent: 'flex-start', padding: '12px', background: activeChat?.friend_id === f.friend_id ? 'var(--primary)' : 'var(--surface-hover)', color: activeChat?.friend_id === f.friend_id ? 'white' : 'var(--text-main)', borderRadius: '8px' }}>
                    {f.remark || f.friend_id}
                  </div>
                ))
              )}
            </div>
          )}
        </div>
      </div>

      {/* Main Chat Area */}
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', background: 'rgba(255, 255, 255, 0.1)' }}>
        {!activeChat ? (
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', color: 'var(--text-muted)' }}>
            <div style={{ fontSize: '48px', marginBottom: '16px' }}>💬</div>
            <h2>欢迎使用 Chat Server</h2>
            <p style={{ marginTop: '8px' }}>选择一个会话或好友开始聊天</p>
          </div>
        ) : (
          <>
            <div style={{ padding: '20px', borderBottom: '1px solid var(--glass-border)', display: 'flex', alignItems: 'center', background: 'rgba(255, 255, 255, 0.2)' }}>
              <h3>{activeChat.remark || activeChat.friend_id}</h3>
            </div>
            
            <div style={{ flex: 1, padding: '20px', overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: '16px' }}>
              {getChatMessages().map((m, idx) => {
                const isMe = m.sender_id === userInfo?.uuid;
                return (
                  <div key={idx} style={{ display: 'flex', justifyContent: isMe ? 'flex-end' : 'flex-start' }}>
                    <div style={{ 
                      maxWidth: '60%', 
                      padding: '12px 16px', 
                      borderRadius: '16px',
                      background: isMe ? 'var(--primary)' : 'var(--surface)',
                      color: isMe ? 'white' : 'var(--text-main)',
                      borderTopRightRadius: isMe ? '4px' : '16px',
                      borderTopLeftRadius: !isMe ? '4px' : '16px',
                      boxShadow: '0 2px 8px rgba(0,0,0,0.05)'
                    }}>
                      {m.content}
                    </div>
                  </div>
                );
              })}
              <div ref={messagesEndRef} />
            </div>

            <div style={{ padding: '20px', borderTop: '1px solid var(--glass-border)', background: 'rgba(255, 255, 255, 0.2)' }}>
              <div style={{ display: 'flex', gap: '12px' }}>
                <input 
                  type="text" 
                  className="form-input" 
                  placeholder="输入消息..." 
                  value={inputMsg}
                  onChange={e => setInputMsg(e.target.value)}
                  onKeyDown={e => e.key === 'Enter' && handleSend()}
                />
                <button className="btn btn-primary" onClick={handleSend}>发送</button>
              </div>
            </div>
          </>
        )}
      </div>
    </div>
  );
}

export default Chat;
