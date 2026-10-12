let appwriteLib = null;
async function getAppwrite() {
  if (!appwriteLib) {
    appwriteLib = await import('./lib/appwrite.js');
  }
  return appwriteLib;
}

function toEmail(input) {
  if (input.includes('@')) return input;
  return `${input.trim().toLowerCase()}@chat.com`;
}

export class ApiError extends Error {
  constructor(message, status, code = null, data = null, isNonRetryable = true) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.data = data;
    this.isNonRetryable = isNonRetryable;
  }
}

// Step 1.1: 判定错误是否属于不可重试 (Non-retryable) 终态错误
// 来源: HTTP 状态码与业务响应 code
// 用于: Step 2.2 决定是否立即中断并抛出异常还是允许指数退避重试
export function isNonRetryable(status, businessCode = null) {
  if (businessCode !== null && businessCode !== undefined) {
    if (businessCode === 1000 || businessCode === 1005 || businessCode === 1012) {
      return false;
    }
    return true;
  }
  if (status >= 400 && status < 500 && status !== 429) {
    return true;
  }
  return false;
}

// Step 1.2: 计算带有随机抖动 (Full Jitter) 的退避重试延迟
// 来源: 当前重试轮次 attempt 及初始延迟 initialDelayMs
// 用于: Step 2.3 在下次请求发起前进行休眠等待
export function calculateRetryDelay(attempt, baseDelay = 100, maxDelay = 3000) {
  const exponential = Math.min(maxDelay, baseDelay * Math.pow(2, attempt));
  return Math.floor(Math.random() * exponential);
}

function genIdempotencyKey() {
  if (typeof crypto !== 'undefined' && crypto.randomUUID) {
    return crypto.randomUUID();
  }
  return 'idemp-' + Date.now() + '-' + Math.random().toString(36).substring(2, 11);
}

// Step 2.1: 执行带幂等凭据管理与分级重试的统一网络请求
// 来源: 调用方传入的目标 url、请求 options 及 retryConfig 重试配置
// 用于: 完成端到端 API 调用，并保障瞬态故障自愈与写请求幂等性
export const request = async (url, options = {}, retryConfig = {}) => {
  // 兼容绝对 URL 的纯 HTTP 请求 (如测试环境或外部网关)
  if (url.startsWith('http://') || url.startsWith('https://')) {
    const maxRetries = retryConfig.maxRetries ?? 2;
    const initialDelayMs = retryConfig.initialDelayMs ?? 100;
    const method = (options.method || 'GET').toUpperCase();

    // 确保所有写请求全周期使用同一个 X-Idempotency-Key
    const headers = { ...(options.headers || {}) };
    if (method !== 'GET' && method !== 'HEAD' && !headers['X-Idempotency-Key'] && !headers['x-idempotency-key']) {
      headers['X-Idempotency-Key'] = genIdempotencyKey();
    }

    let lastError = null;

    for (let attempt = 0; attempt <= maxRetries; attempt++) {
      try {
        const res = await fetch(url, {
          ...options,
          headers,
        });

        const status = res.status;
        let bodyData = null;
        try {
          bodyData = await res.json();
        } catch (_) {}

        // 1. 检查 HTTP 4xx 客户端错误 (429 除外)
        if (status >= 400 && status < 500 && status !== 429) {
          throw new ApiError(
            bodyData?.msg || `客户端请求错误 HTTP ${status}`,
            status,
            bodyData?.code ?? null,
            bodyData?.data ?? null,
            true
          );
        }

        // 2. 检查 HTTP 429 限流或 5xx 服务端瞬态错误
        if (status === 429 || status >= 500) {
          if (attempt < maxRetries) {
            const delay = calculateRetryDelay(attempt, initialDelayMs);
            await new Promise((r) => setTimeout(r, delay));
            continue;
          }
          throw new ApiError(
            bodyData?.msg || `服务暂时不可用 HTTP ${status}`,
            status,
            bodyData?.code ?? null,
            bodyData?.data ?? null,
            false
          );
        }

        // 3. 检查业务层返回码
        if (bodyData && typeof bodyData === 'object' && 'code' in bodyData) {
          const bCode = bodyData.code;
          if (bCode === 1000) {
            return bodyData;
          }
          if ((bCode === 1005 || bCode === 1012) && attempt < maxRetries) {
            const delay = calculateRetryDelay(attempt, initialDelayMs);
            await new Promise((r) => setTimeout(r, delay));
            continue;
          }
          // 其他不可重试的业务错误 (如 1001, 1002, 1004, 2001)
          throw new ApiError(
            bodyData.msg || '业务处理失败',
            status,
            bCode,
            bodyData.data ?? null,
            true
          );
        }

        return bodyData;
      } catch (err) {
        lastError = err;
        // 如果是已被判定为不可重试的 ApiError，绝不继续重试，立即向上抛出
        if (err instanceof ApiError && err.isNonRetryable) {
          throw err;
        }
        if (attempt >= maxRetries) {
          throw err;
        }
        const delay = calculateRetryDelay(attempt, initialDelayMs);
        await new Promise((r) => setTimeout(r, delay));
      }
    }
    throw lastError;
  }

  // 业务相对路径：按需动态加载 Appwrite BaaS 适配逻辑
  const { account, tableApi, DATABASE_ID, ID } = await getAppwrite();

  // 1. 登录
  if (url === '/auth/login') {
    const { username, password } = JSON.parse(options.body || '{}');
    const email = toEmail(username);
    try {
      try {
        await account.createEmailPasswordSession(email, password);
      } catch (sessionErr) {
        if (!sessionErr.message?.includes('session is active')) {
          throw sessionErr;
        }
      }
      const user = await account.get();
      return {
        code: 1000,
        msg: 'success',
        data: {
          access_token: user.$id,
          uuid: user.$id,
          username: user.name || username,
        },
      };
    } catch (err) {
      return {
        code: 1004,
        msg: err.message || '登录失败，请检查账号密码',
      };
    }
  }

  // 2. 注册
  if (url === '/auth/register') {
    const { username, password, nickname } = JSON.parse(options.body || '{}');
    const email = toEmail(username);
    try {
      const user = await account.create(ID.unique(), email, password, nickname || username);
      try {
        await account.createEmailPasswordSession(email, password);
        await tableApi(`/tablesdb/${DATABASE_ID}/tables/friends/rows`, 'POST', {
          rowId: 'unique()',
          data: {
            user_id: user.$id,
            friend_id: 'bot-assistant',
            friend_name: 'IM 智能助手',
            status: 1,
          },
        });
      } catch (_) {}
      return {
        code: 1000,
        msg: 'success',
      };
    } catch (err) {
      return {
        code: 1001,
        msg: err.message || '注册失败，请稍后重试',
      };
    }
  }

  // 3. 用户信息
  if (url === '/user/info') {
    try {
      const user = await account.get();
      return {
        code: 1000,
        data: {
          uuid: user.$id,
          username: user.name || user.email.split('@')[0],
          nickname: user.name || user.email.split('@')[0],
        },
      };
    } catch (err) {
      return { code: 401, msg: '未登录' };
    }
  }

  // 4. 好友列表
  if (url === '/friends') {
    try {
      const user = await account.get();
      let list = [];
      try {
        const res = await tableApi(`/tablesdb/${DATABASE_ID}/tables/friends/rows?queries[]=equal("user_id",["${user.$id}"])`);
        list = (res.rows || []).map((r) => ({
          friend_id: r.friend_id,
          remark: r.friend_name,
        }));
      } catch (_) {}

      if (!list.some((f) => f.friend_id === 'bot-assistant')) {
        list.unshift({ friend_id: 'bot-assistant', remark: 'IM 智能助手' });
      }

      return {
        code: 1000,
        data: { list },
      };
    } catch (err) {
      return {
        code: 1000,
        data: {
          list: [{ friend_id: 'bot-assistant', remark: 'IM 智能助手' }],
        },
      };
    }
  }

  return { code: 404, msg: 'Not found' };
};
