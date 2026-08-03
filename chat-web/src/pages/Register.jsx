import React, { useState } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { request } from '../api';

function Register() {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();

  const handleRegister = async (e) => {
    e.preventDefault();
    setError('');
    setSuccess('');
    
    if (!username || !password) {
      setError('请完整填写所有字段');
      return;
    }

    setLoading(true);
    // Default nickname to username
    const res = await request('/auth/register', {
      method: 'POST',
      body: JSON.stringify({ username, password, nickname: username })
    });
    setLoading(false);

    if (res && res.code === 1000) {
      setSuccess('注册成功！即将跳转到登录页...');
      setTimeout(() => navigate('/login'), 1500);
    } else {
      setError(res?.msg || '注册失败，请稍后重试');
    }
  };

  return (
    <div className="glass-panel" style={{ width: '400px', padding: '40px', textAlign: 'center' }}>
      <h2 style={{ marginBottom: '24px', color: 'var(--primary)' }}>注册新账号</h2>
      {error && <div style={{ color: 'var(--danger)', marginBottom: '16px', fontSize: '14px' }}>{error}</div>}
      {success && <div style={{ color: 'var(--success)', marginBottom: '16px', fontSize: '14px' }}>{success}</div>}
      <form onSubmit={handleRegister} style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
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
          {loading ? '提交中...' : '注 册'}
        </button>
      </form>
      <div style={{ marginTop: '20px', fontSize: '14px' }}>
        已有账号？ <Link to="/login" style={{ color: 'var(--primary)', textDecoration: 'none' }}>去登录</Link>
      </div>
    </div>
  );
}

export default Register;
