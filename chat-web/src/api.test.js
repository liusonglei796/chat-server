import test from 'node:test';
import assert from 'node:assert/strict';
import http from 'node:http';
import { request, ApiError, isNonRetryable } from './api.js';

// Helper to start a temporary local HTTP mock server
function createMockServer(handler) {
  return new Promise((resolve) => {
    const server = http.createServer(handler);
    server.listen(0, '127.0.0.1', () => {
      const port = server.address().port;
      resolve({
        server,
        url: `http://127.0.0.1:${port}`,
        close: () => new Promise((res) => server.close(res)),
      });
    });
  });
}

test('isNonRetryable should correctly identify non-retryable errors', () => {
  // HTTP 4xx (except 429) -> non-retryable
  assert.equal(isNonRetryable(400, null), true);
  assert.equal(isNonRetryable(401, null), true);
  assert.equal(isNonRetryable(403, null), true);
  assert.equal(isNonRetryable(404, null), true);
  assert.equal(isNonRetryable(422, null), true);
  // HTTP 429 -> retryable
  assert.equal(isNonRetryable(429, null), false);
  // HTTP 5xx -> retryable
  assert.equal(isNonRetryable(500, null), false);
  assert.equal(isNonRetryable(503, null), false);

  // Business errors -> non-retryable
  assert.equal(isNonRetryable(200, 1001), true); // 参数错误
  assert.equal(isNonRetryable(200, 1002), true); // 用户已存在
  assert.equal(isNonRetryable(200, 1003), true); // 用户不存在
  assert.equal(isNonRetryable(200, 1004), true); // 密码错误
  assert.equal(isNonRetryable(200, 2001), true); // 余额不足等业务报错

  // Transient business errors / success
  assert.equal(isNonRetryable(200, 1000), false); // 成功
  assert.equal(isNonRetryable(200, 1005), false); // 服务繁忙 (可重试)
  assert.equal(isNonRetryable(200, 1012), false); // 业务限流 (可重试)
});

test('HTTP 400 参数错误: 必须立即中断并抛出异常，绝不进行重试', async () => {
  let callCount = 0;
  const mock = await createMockServer((req, res) => {
    callCount++;
    res.writeHead(400, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ code: 1001, msg: '参数非法: 用户名不能为空' }));
  });

  try {
    await assert.rejects(
      async () => {
        await request(mock.url + '/test-400', { method: 'POST' }, { maxRetries: 3, initialDelayMs: 10 });
      },
      (err) => {
        assert.ok(err instanceof ApiError);
        assert.equal(err.status, 400);
        assert.equal(err.code, 1001);
        assert.equal(err.isNonRetryable, true);
        assert.match(err.message, /用户名不能为空/);
        return true;
      }
    );
    // 严格断言仅调用了 1 次，没有发生任何重试！
    assert.equal(callCount, 1, '400 错误必须立即中断，调用次数应为 1');
  } finally {
    await mock.close();
  }
});

test('HTTP 401 鉴权失败: 必须立即中断并抛出异常，绝不进行重试', async () => {
  let callCount = 0;
  const mock = await createMockServer((req, res) => {
    callCount++;
    res.writeHead(401, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ code: 1006, msg: '未授权或登录已过期' }));
  });

  try {
    await assert.rejects(
      async () => {
        await request(mock.url + '/test-401', { method: 'GET' }, { maxRetries: 3, initialDelayMs: 10 });
      },
      (err) => {
        assert.ok(err instanceof ApiError);
        assert.equal(err.status, 401);
        assert.equal(err.isNonRetryable, true);
        return true;
      }
    );
    assert.equal(callCount, 1, '401 错误必须立即中断，调用次数应为 1');
  } finally {
    await mock.close();
  }
});

test('HTTP 404 资源不存在: 必须立即中断并抛出异常，绝不进行重试', async () => {
  let callCount = 0;
  const mock = await createMockServer((req, res) => {
    callCount++;
    res.writeHead(404, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ code: 1008, msg: '目标资源不存在' }));
  });

  try {
    await assert.rejects(
      async () => {
        await request(mock.url + '/test-404', { method: 'GET' }, { maxRetries: 3, initialDelayMs: 10 });
      },
      (err) => {
        assert.ok(err instanceof ApiError);
        assert.equal(err.status, 404);
        assert.equal(err.isNonRetryable, true);
        return true;
      }
    );
    assert.equal(callCount, 1, '404 错误必须立即中断，调用次数应为 1');
  } finally {
    await mock.close();
  }
});

test('具体业务逻辑报错（如余额不足、密码错误）: 必须立即中断并抛出异常，绝不进行重试', async () => {
  let callCount = 0;
  const mock = await createMockServer((req, res) => {
    callCount++;
    // HTTP 状态码为 200，但 body 返回业务逻辑报错
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ code: 2001, msg: '账户可用余额不足，无法扣费' }));
  });

  try {
    await assert.rejects(
      async () => {
        await request(mock.url + '/pay', { method: 'POST', body: JSON.stringify({ amount: 100 }) }, { maxRetries: 3, initialDelayMs: 10 });
      },
      (err) => {
        assert.ok(err instanceof ApiError);
        assert.equal(err.code, 2001);
        assert.equal(err.isNonRetryable, true);
        assert.equal(err.message, '账户可用余额不足，无法扣费');
        return true;
      }
    );
    // 严格断言仅调用 1 次，绝不因为业务错误盲目重试！
    assert.equal(callCount, 1, '业务逻辑报错必须立即中断，调用次数应为 1');
  } finally {
    await mock.close();
  }
});

test('HTTP 429 限流（唯一可重试的 4xx）: 允许退避重试并最终成功', async () => {
  let callCount = 0;
  const mock = await createMockServer((req, res) => {
    callCount++;
    if (callCount === 1) {
      res.writeHead(429, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ code: 1012, msg: 'Too Many Requests' }));
    } else {
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ code: 1000, msg: 'success', data: { status: 'ok' } }));
    }
  });

  try {
    const res = await request(mock.url + '/rate-limited', { method: 'POST' }, { maxRetries: 2, initialDelayMs: 10 });
    assert.equal(res.code, 1000);
    assert.equal(res.data.status, 'ok');
    assert.equal(callCount, 2, '429 错误应自动重试第 2 次并成功');
  } finally {
    await mock.close();
  }
});

test('HTTP 503 服务端错误: 重试成功且全周期传递同一个 X-Idempotency-Key', async () => {
  let callCount = 0;
  const receivedKeys = [];

  const mock = await createMockServer((req, res) => {
    callCount++;
    receivedKeys.push(req.headers['x-idempotency-key']);
    if (callCount < 3) {
      res.writeHead(503, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ msg: 'Service Temporarily Unavailable' }));
    } else {
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ code: 1000, msg: 'success', data: 'recovered' }));
    }
  });

  try {
    const res = await request(mock.url + '/write-op', { method: 'POST', body: JSON.stringify({ item: 'test' }) }, { maxRetries: 3, initialDelayMs: 10 });
    assert.equal(res.code, 1000);
    assert.equal(res.data, 'recovered');
    assert.equal(callCount, 3, '前 2 次失败后应在第 3 次成功');

    // 验证核心准则：3 次重试必须共享同一个业务幂等键
    assert.equal(receivedKeys.length, 3);
    assert.ok(receivedKeys[0], '第 1 次应带有 X-Idempotency-Key');
    assert.equal(receivedKeys[0], receivedKeys[1], '第 2 次重试必须使用相同的幂等键');
    assert.equal(receivedKeys[1], receivedKeys[2], '第 3 次重试必须使用相同的幂等键');
  } finally {
    await mock.close();
  }
});
