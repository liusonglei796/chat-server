import { account, tableApi, DATABASE_ID, ID } from './lib/appwrite';

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

/**
 * 适配 Appwrite BaaS 的请求处理器
 */
export const request = async (url, options = {}) => {
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
        // 为新用户默认关联官方小助手好友
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

      // 确保包含智能助手
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
