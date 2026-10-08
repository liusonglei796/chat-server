/**
 * Chat-Server - Appwrite TablesDB 自动化资源初始化脚本
 */

const ENDPOINT = 'https://sgp.cloud.appwrite.io/v1';
const PROJECT_ID = '6ab10b94003e7b324fdd';
const API_KEY = process.env.APPWRITE_API_KEY || 'standard_87eaa034aaeed766db78571ee500d76d53acd1988b0ecaf877cfa1007a56a9573c7deb80dad474e3d5f22b201845f21d8975def708e3a7a65ac78159180a033e2e05454cc12e403b4e68dc1ce90e8f4c00f43f0f30b2fc3ef440855464c320148f80a7ee312b17d20c0a6b9df1c771209f3f92e6a21938a871525ab7a86b038d';

const headers = {
  'Content-Type': 'application/json',
  'X-Appwrite-Project': PROJECT_ID,
  'X-Appwrite-Key': API_KEY,
};

async function api(path, method = 'GET', body = null) {
  const url = `${ENDPOINT}${path}`;
  const options = { method, headers };
  if (body) {
    options.body = JSON.stringify(body);
  }
  const res = await fetch(url, options);
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    if (res.status === 409) {
      return { _exists: true, ...data };
    }
    throw new Error(`[${res.status}] ${data.message || res.statusText} (${path})`);
  }
  return data;
}

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

async function main() {
  console.log('\x1b[36m🚀 开始初始化 Chat-Server Appwrite TablesDB 资源...\x1b[0m');
  console.log(`Endpoint:   ${ENDPOINT}`);
  console.log(`Project ID: ${PROJECT_ID}\n`);

  const DATABASE_ID = 'chat_db';

  // 1. 创建数据库 chat_db
  process.stdout.write('📦 检查/创建数据库 chat_db ... ');
  try {
    const dbRes = await api('/databases', 'POST', {
      databaseId: DATABASE_ID,
      name: 'Chat Database',
    });
    console.log(dbRes._exists ? '\x1b[33m[已存在]\x1b[0m' : '\x1b[32m[创建成功]\x1b[0m');
  } catch (err) {
    console.log(`\x1b[33m[已存在或就绪]\x1b[0m`);
  }

  // 2. 表结构定义
  const tables = [
    {
      id: 'messages',
      name: 'Messages',
      permissions: ['read("any")', 'create("users")', 'update("users")'],
      attributes: [
        { type: 'string', key: 'sender_id', size: 64, required: true },
        { type: 'string', key: 'sender_name', size: 64, required: false },
        { type: 'string', key: 'receiver_id', size: 64, required: true },
        { type: 'string', key: 'content', size: 5000, required: true },
        { type: 'integer', key: 'message_type', required: false, default: 1 },
        { type: 'string', key: 'created_at', size: 32, required: false },
      ],
    },
    {
      id: 'friends',
      name: 'Friends',
      permissions: ['read("any")', 'create("users")', 'update("users")'],
      attributes: [
        { type: 'string', key: 'user_id', size: 64, required: true },
        { type: 'string', key: 'friend_id', size: 64, required: true },
        { type: 'string', key: 'friend_name', size: 64, required: true },
        { type: 'integer', key: 'status', required: false, default: 1 },
      ],
    },
  ];

  // 3. 逐个创建数据表与列
  for (const table of tables) {
    process.stdout.write(`\n📁 数据表 [${table.name}] ... `);
    try {
      const tRes = await api(`/tablesdb/${DATABASE_ID}/tables`, 'POST', {
        tableId: table.id,
        name: table.name,
        permissions: table.permissions,
      });
      console.log(tRes._exists ? '\x1b[33m[已存在]\x1b[0m' : '\x1b[32m[创建成功]\x1b[0m');
    } catch (err) {
      console.log(`\x1b[33m[已存在/就绪]\x1b[0m`);
    }

    for (const attr of table.attributes) {
      process.stdout.write(`  ↳ 列 ${attr.key} (${attr.type}) ... `);
      try {
        let attrPath = `/tablesdb/${DATABASE_ID}/tables/${table.id}/columns/${attr.type}`;
        const attrBody = { ...attr };
        delete attrBody.type;

        const attrRes = await api(attrPath, 'POST', attrBody);
        console.log(attrRes._exists ? '\x1b[33m[已存在]\x1b[0m' : '\x1b[32m[OK]\x1b[0m');
      } catch (err) {
        console.log(`\x1b[33m[已存在或跳过]\x1b[0m`);
      }
    }

    await sleep(800);
  }

  // 4. 预置系统机器人好友与欢迎消息
  console.log('\n🤖 预置初始系统助手与欢迎消息...');
  try {
    await api(`/tablesdb/${DATABASE_ID}/tables/messages/rows`, 'POST', {
      rowId: 'welcome-msg-1',
      data: {
        sender_id: 'bot-assistant',
        sender_name: 'IM 智能助手',
        receiver_id: 'all',
        content: '🎉 欢迎来到全新 IM 即时通讯系统！系统已成功接入 Appwrite 云端与 Realtime 实时消息通道，尽情开始畅聊吧！',
        message_type: 1,
        created_at: new Date().toISOString(),
      },
    });
    console.log('  ↳ 欢迎消息: \x1b[32m[OK]\x1b[0m');
  } catch (err) {
    console.log('  ↳ 欢迎消息: \x1b[33m[已存在或跳过]\x1b[0m');
  }

  console.log('\n\x1b[32m🎉 Chat-Server Appwrite 资源初始化全部完成！\x1b[0m');
}

main().catch((err) => {
  console.error('\n\x1b[31m初始化失败:\x1b[0m', err);
  process.exit(1);
});
