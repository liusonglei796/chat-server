const API = 'http://localhost:19080';
const WS_URL = 'http://localhost:18000/ws';

async function main() {
  // 1. Login to get token
  const res = await fetch(API + '/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: 'uia_1793270686198', password: 'Password123!' })
  });
  const data = await res.json();
  console.log('Login response:', data);
  const token = data.accessToken || data.access_token;
  console.log('Got token length:', token.length);

  // 2. Connect WS with query token
  const urlWithToken = `${WS_URL}?token=${token}`;
  console.log('Connecting to:', urlWithToken.substring(0, 50) + '...');
  
  // Test native fetch/upgrade or http request
  const http = require('http');
  const req = http.request(urlWithToken, {
    headers: {
      'Connection': 'Upgrade',
      'Upgrade': 'websocket',
      'Sec-WebSocket-Version': '13',
      'Sec-WebSocket-Key': 'dGhlIHNhbXBsZSBub25jZQ=='
    }
  });

  req.on('upgrade', (res, socket, upgradeHead) => {
    console.log('WS Upgrade successful! Status:', res.statusCode);
    socket.destroy();
  });

  req.on('response', (res) => {
    console.log('WS Upgrade failed with HTTP status:', res.statusCode);
    res.on('data', d => console.log('Body:', d.toString()));
  });

  req.on('error', err => {
    console.error('Req error:', err.message);
  });

  req.end();
}

main().catch(console.error);
