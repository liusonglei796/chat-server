import { Client, Account, ID } from 'appwrite';

const ENDPOINT = import.meta.env.VITE_APPWRITE_ENDPOINT || 'https://sgp.cloud.appwrite.io/v1';
const PROJECT_ID = import.meta.env.VITE_APPWRITE_PROJECT_ID || '6ab10b94003e7b324fdd';

export const DATABASE_ID = import.meta.env.VITE_APPWRITE_DATABASE_ID || 'chat_db';

export const client = new Client()
  .setEndpoint(ENDPOINT)
  .setProject(PROJECT_ID);

export const account = new Account(client);
export { ID };

/**
 * 封装对 TablesDB REST API 的调用（自动携带客户端 Session Cookie）
 */
export async function tableApi(path, method = 'GET', body = null) {
  const url = `${ENDPOINT}${path}`;
  const headers = {
    'Content-Type': 'application/json',
    'X-Appwrite-Project': PROJECT_ID,
  };

  const options = {
    method,
    headers,
    credentials: 'include',
  };

  if (body) {
    options.body = JSON.stringify(body);
  }

  const res = await fetch(url, options);
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(data.message || `Request failed (${res.status})`);
  }
  return data;
}
