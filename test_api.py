import urllib.request
import urllib.error
import json
import uuid
import time

BASE_URL = "http://localhost:8081"

def log(msg):
    print(f"[*] {msg}")

def req(path, data=None, token=None):
    url = f"{BASE_URL}{path}"
    req = urllib.request.Request(url)
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
                print(f"Failed to parse JSON. Raw response: {res_data}")
                return response.status, res_data
    except urllib.error.HTTPError as e:
        res_data = e.read().decode('utf-8')
        try:
            return e.code, json.loads(res_data)
        except:
            print(f"Failed to parse JSON. Raw response: {res_data}")
            return e.code, res_data
    except Exception as e:
        print(f"Request failed: {e}")
        return 500, {}

def test_flow():
    # 1. Register User A
    username_a = f"test_a_{uuid.uuid4().hex[:6]}"
    password = "password123"
    
    log(f"Registering {username_a}...")
    status, res = req("/auth/register", {"username": username_a, "password": password})
    
    if status != 200 or res.get('code') != 1000:
        log(f"Registration failed: {res}")
        return False
        
    log("Registration successful.")

    # 2. Login User A
    log(f"Logging in {username_a}...")
    status, res = req("/auth/login", {"username": username_a, "password": password})
    
    if status != 200 or res.get('code') != 1000:
        log(f"Login failed: {res}")
        return False
        
    token_a = res['data']['access_token']
    log(f"Login successful. Token: {token_a[:20]}...")
    
    # 3. Get Profile
    log("Fetching Profile...")
    status, res = req("/user/info", token=token_a)
    if status != 200 or res.get('code') != 1000:
        log(f"Get Profile failed: {res}")
        return False
        
    user_id_a = res['data']['uuid']
    log(f"Profile fetched successfully. User ID: {user_id_a}")
    
    # 4. Try getting friends list
    log("Fetching friends list...")
    status, res = req("/friends", token=token_a)
    if status != 200 or res.get('code') != 1000:
        log(f"Friends list failed: {res}")
        return False
    
    friends_list = res.get('data', {}).get('list')
    num_friends = len(friends_list) if friends_list else 0
    log(f"Friends list fetched successfully. Found {num_friends} friends.")
    
    print("\n[SUCCESS] ALL HTTP APIs ARE WORKING CORRECTLY!")
    return True

if __name__ == "__main__":
    time.sleep(2)
    test_flow()
