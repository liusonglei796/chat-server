import urllib.request
import urllib.error
import json
import uuid
import time
import sys
import os

BASE_URL = os.environ.get("TARGET_URL", "http://localhost:9080")
if len(sys.argv) > 1:
    arg = sys.argv[1]
    if arg.isdigit():
        BASE_URL = f"http://localhost:{arg}"
    elif arg.startswith("http"):
        BASE_URL = arg

def log(msg):
    print(f"[*] {msg}")

def req(path, data=None, token=None, method="POST"):
    url = f"{BASE_URL}{path}"
    req = urllib.request.Request(url, method=method)
    if data is not None:
        req.data = json.dumps(data).encode('utf-8')
        req.add_header('Content-Type', 'application/json')
    if token:
        req.add_header('Authorization', f'Bearer {token}')
        
    try:
        with urllib.request.urlopen(req) as response:
            res_data = response.read().decode('utf-8')
            try:
                return response.status, json.loads(res_data)
            except:
                return response.status, res_data
    except urllib.error.HTTPError as e:
        res_data = e.read().decode('utf-8')
        try:
            return e.code, json.loads(res_data)
        except:
            return e.code, res_data
    except Exception as e:
        print(f"Request failed: {e}")
        return 500, {}

def test_flow():
    log(f"Testing against base URL: {BASE_URL}")

    # 1. Register User A & B
    user_a = f"test_a_{uuid.uuid4().hex[:6]}"
    user_b = f"test_b_{uuid.uuid4().hex[:6]}"
    
    log(f"Registering A ({user_a}) & B ({user_b})...")
    status, res = req("/auth/register", {"username": user_a, "password": "password123", "nickname": "A"})
    if status != 200 or res.get('code') != 1000:
        log(f"Register A failed: {res}")
        return False
        
    status, res = req("/auth/register", {"username": user_b, "password": "password123", "nickname": "B"})
    if status != 200 or res.get('code') != 1000:
        log(f"Register B failed: {res}")
        return False
    
    # 2. Login
    log("Logging in A & B...")
    status_a, res_a = req("/auth/login", {"username": user_a, "password": "password123"})
    status_b, res_b = req("/auth/login", {"username": user_b, "password": "password123"})
    
    if status_a != 200 or not res_a or res_a.get('code') != 1000:
        log(f"Login A failed: {res_a}")
        return False
        
    if status_b != 200 or not res_b or res_b.get('code') != 1000:
        log(f"Login B failed: {res_b}")
        return False
        
    token_a = res_a['data']['access_token']
    token_b = res_b['data']['access_token']
    
    # 3. Get Profiles
    _, p_a = req("/user/info", token=token_a, method="GET")
    _, p_b = req("/user/info", token=token_b, method="GET")
    
    id_a = p_a['data']['uuid']
    id_b = p_b['data']['uuid']
    
    # 4. A applies to B
    log("A applies to B as friend...")
    status, res = req("/friends/apply", {"friend_id": id_b, "message": "hello"}, token=token_a)
    if status != 200 or res.get('code') != 1000:
        log(f"Apply failed: {res}")
        return False
        
    # 5. B approves A
    log("B approves A...")
    status, res = req("/friends/applies/approve", {"applicant_id": id_a}, token=token_b)
    if status != 200 or res.get('code') != 1000:
        log(f"Approve failed: {res}")
        return False
        
    # 6. Check friends list
    log("Checking friends list...")
    status, friends_res = req("/friends", token=token_a, method="GET")
    friends = friends_res.get('data', {}).get('list', [])
    print("Friends list:", friends)
    friend_id_field = 'friend_id'
    if friends and isinstance(friends[0], dict) and 'user_id' in friends[0]:
        friend_id_field = 'user_id'
    elif friends and isinstance(friends[0], dict) and 'id' in friends[0]:
        friend_id_field = 'id'
        
    if not friends or friends[0][friend_id_field] != id_b:
        log("Friends list check failed.")
        return False
        
    # 7. A creates a group
    log("A creates a group...")
    status, res = req("/groups", {"name": "Test Group"}, token=token_a)
    if status != 200 or res.get('code') != 1000:
        log(f"Group create failed: {res}")
        return False
        
    # 8. Check joined groups
    log("Checking joined groups...")
    status, groups_a = req("/groups/joined", token=token_a, method="GET")
    if status != 200 or groups_a.get('code') != 1000:
        log(f"Get joined groups failed: {groups_a}")
        return False

    # 9. Check Open Session Allowed
    log("Checking session open permission between A and B...")
    status, sess_chk = req(f"/sessions/check?target_id={id_b}", token=token_a, method="GET")
    if status != 200 or sess_chk.get('code') != 1000:
        log(f"Check open session failed: {sess_chk}")
        return False

    # 10. Open Session
    log("A opens session with B...")
    status, sess_open = req("/sessions", {"receive_id": id_b}, token=token_a, method="POST")
    if status != 200 or sess_open.get('code') != 1000:
        log(f"Open session failed: {sess_open}")
        return False

    # 11. Check User Direct Sessions
    log("Checking A's direct session list...")
    status, sess_list = req("/sessions/direct", token=token_a, method="GET")
    if status != 200 or sess_list.get('code') != 1000:
        log(f"Get sessions list failed: {sess_list}")
        return False

    # 12. Query messages between A and B
    log("Checking messages between A and B...")
    status, msg_list = req(f"/messages?target_id={id_b}", token=token_a, method="GET")
    if status != 200 or msg_list.get('code') != 1000:
        log(f"Get messages failed: {msg_list}")
        return False

    print("\n[SUCCESS] ALL EXTENDED HTTP APIs ARE WORKING CORRECTLY!")
    return True

if __name__ == "__main__":
    test_flow()
