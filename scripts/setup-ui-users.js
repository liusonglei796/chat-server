const tag = Date.now();
const userA = `uia_${tag}`;
const userB = `uib_${tag}`;
const pass = 'Password123!';
const API = 'http://localhost:19080';

async function main() {
  async function post(url, body, headers = {}) {
    const res = await fetch(API + url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', ...headers },
      body: JSON.stringify(body)
    });
    const text = await res.text();
    try {
      return { status: res.status, data: JSON.parse(text) };
    } catch {
      return { status: res.status, text };
    }
  }

  // Register A
  const regA = await post('/auth/register', { username: userA, password: pass, client_token: `ui-a-${tag}` });
  const aID = regA.data.uuid;

  // Register B
  const regB = await post('/auth/register', { username: userB, password: pass, client_token: `ui-b-${tag}` });
  const bID = regB.data.uuid;

  // Login A
  const logA = await post('/auth/login', { username: userA, password: pass });
  const aTok = logA.data.access_token;

  // Login B
  const logB = await post('/auth/login', { username: userB, password: pass });
  const bTok = logB.data.access_token;

  console.log(`Registered userA: ${userA} (${aID}), userB: ${userB} (${bID})`);

  // Friend apply
  const appRes = await post('/friends/apply', {
    user_id: aID,
    friend_id: bID,
    message: 'hi from ui test',
    client_token: `app-${tag}`
  }, { 'Authorization': `Bearer ${aTok}`, 'x-user-id': aID });

  console.log('Friend apply response:', appRes.status, appRes.data);

  // Friend approve
  const aprRes = await post('/friends/applies/approve', {
    user_id: bID,
    applicant_id: aID
  }, { 'Authorization': `Bearer ${bTok}`, 'x-user-id': bID });

  console.log('Friend approve response:', aprRes.status, aprRes.data);

  // Output JSON for the browser test
  console.log('UI_TEST_USERS_READY:' + JSON.stringify({
    userA, userB, pass, aID, bID
  }));
}

main().catch(err => {
  console.error(err);
  process.exit(1);
});
