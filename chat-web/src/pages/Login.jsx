import React, { useState } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { request } from '../api';

function Login() {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();

  const handleLogin = async (e) => {
    e.preventDefault();
    setError('');
    
    if (!username || !password) {
      setError('请输入用户名和密码');
      return;
    }

    setLoading(true);
    const res = await request('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password })
    });
    setLoading(false);

    if (res && res.code === 1000) {
      localStorage.setItem('token', res.data.access_token);
      localStorage.setItem('user_info', JSON.stringify({ uuid: res.data.uuid }));
      navigate('/chat');
    } else {
      setError(res?.msg || '登录失败，请检查用户名和密码');
    }
  };

  return (
    <div className="glass-panel" style={{ width: '400px', padding: '40px', textAlign: 'center' }}>
      <h2 style={{ marginBottom: '24px', color: 'var(--primary)' }}>Chat Server</h2>
      {error && <div style={{ color: 'var(--danger)', marginBottom: '16px', fontSize: '14px' }}>{error}</div>}
      <form onSubmit={handleLogin} style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
        <input 
          type="text" 
          placeholder="用户名" 
          className="form-input"
          value={username} 
          onChange={(e) => setUsername(e.target.value)} 
        />
        <input 
          type="password" 
          placeholder="密码" 
          className="form-input"
          value={password} 
          onChange={(e) => setPassword(e.target.value)} 
        />
        <button type="submit" className="btn btn-primary" style={{ width: '100%', marginTop: '8px' }} disabled={loading}>
          {loading ? '登录中...' : '登 录'}
        </button>
      </form>
      <div style={{ marginTop: '20px', fontSize: '14px' }}>
        没有账号？ <Link to="/register" style={{ color: 'var(--primary)', textDecoration: 'none' }}>立即注册</Link>
      </div>
    </div>
  );
}

export default Login;
