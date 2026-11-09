const http = require('http');
const proxy = process.argv[2] || 'ws_client';
const enabled = process.argv[3] === 'true';

const req = http.request(`http://localhost:8474/proxies/${proxy}`, {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'User-Agent': 'Toxiproxy-Client'
  }
}, res => {
  let data = '';
  res.on('data', c => data += c);
  res.on('end', () => console.log('Toxiproxy set:', proxy, enabled, res.statusCode, data));
});
req.write(JSON.stringify({ enabled }));
req.end();
