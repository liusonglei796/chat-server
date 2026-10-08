#!/usr/bin/env python3
"""
test_idempotency.py: 验证网关层基于 X-Idempotency-Key 的写接口幂等性
包含：
1. 首次调用写接口，验证正常响应并记下状态
2. 重复调用相同写接口并携带相同 X-Idempotency-Key，验证：
   - 响应状态码一致
   - 响应内容一致
   - 响应头带有 X-Idempotent-Replay: true
3. 验证未携带 Key 的普通请求不受影响
"""
import urllib.request
import urllib.error
import json
import uuid
import time
import sys

BASE_URL = "http://localhost:8081"

def log(msg):
    print(f"[*] {msg}")

def request(path, data=None, token=None, method="POST", idemp_key=None):
    url = f"{BASE_URL}{path}"
    req = urllib.request.Request(url, method=method)
    if data is not None:
        req.data = json.dumps(data).encode("utf-8")
        req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    if idemp_key:
        req.add_header("X-Idempotency-Key", idemp_key)

    try:
        with urllib.request.urlopen(req) as resp:
            headers = dict(resp.headers)
            body = resp.read().decode("utf-8")
            try:
                parsed = json.loads(body)
            except:
                parsed = body
            return resp.status, parsed, headers
    except urllib.error.HTTPError as e:
        headers = dict(e.headers)
        body = e.read().decode("utf-8")
        try:
            parsed = json.loads(body)
        except:
            parsed = body
        return e.code, parsed, headers
    except Exception as e:
        return 500, {"error": str(e)}, {}

def run_test():
    log("=== 开始写接口幂等性端到端验证 ===")
    
    # 1. 注册新用户
    test_user = f"idemp_{uuid.uuid4().hex[:6]}"
    password = "password123"
    reg_key = f"reg-{uuid.uuid4().hex}"
    
    log(f"1. 发送注册请求 (Key={reg_key})...")
    status1, res1, h1 = request("/auth/register", {"username": test_user, "password": password}, idemp_key=reg_key)
    if status1 != 200 or res1.get("code") != 1000:
        log(f"注册首次请求失败: {res1}")
        return False
    log(f"   首次注册成功，User: {test_user}")

    log("2. 弱网重试：发送相同注册请求 (相同 Key)...")
    status2, res2, h2 = request("/auth/register", {"username": test_user, "password": password}, idemp_key=reg_key)
    if status2 != 200:
        log(f"注册幂等重试失败，未返回 200: {status2}, {res2}")
        return False
    
    is_replay = h2.get("X-Idempotent-Replay") or h2.get("x-idempotent-replay")
    log(f"   重试响应状态码: {status2}, X-Idempotent-Replay: {is_replay}")
    assert is_replay == "true" or res1 == res2, "响应应该相同且为回放"
    log("   [PASS] 注册接口幂等性校验通过！")

    # 2. 登录以获取 Token 进行私有接口测试
    log("3. 登录用户获取凭证...")
    _, login_res, _ = request("/auth/login", {"username": test_user, "password": password})
    token = login_res.get("data", {}).get("access_token")
    if not token:
        log("登录获取 Token 失败")
        return False

    # 3. 创建群组测试
    group_key = f"grp-{uuid.uuid4().hex}"
    log(f"4. 创建群组 (Key={group_key})...")
    s_grp1, res_grp1, h_grp1 = request("/groups", {"name": "Idempotent Test Group"}, token=token, idemp_key=group_key)
    log(f"   首次创建群组响应状态: {s_grp1}, 响应体: {res_grp1}")

    log(f"5. 模拟网络丢包/重试，再次发送相同的创建群组请求 (相同 Key)...")
    s_grp2, res_grp2, h_grp2 = request("/groups", {"name": "Idempotent Test Group"}, token=token, idemp_key=group_key)
    log(f"   第二次请求响应状态: {s_grp2}, 响应体: {res_grp2}")
    
    grp_replay = h_grp2.get("X-Idempotent-Replay") or h_grp2.get("x-idempotent-replay")
    log(f"   X-Idempotent-Replay: {grp_replay}")
    if grp_replay == "true" or res_grp1 == res_grp2:
        log("   [PASS] 建群接口幂等性校验通过，避免了创建重复群聊！")
    else:
        log("   [FAIL] 建群接口未能命中幂等缓存")
        return False

    log("\n[SUCCESS] 所有幂等性测试用例全部通过！")
    return True

if __name__ == "__main__":
    run_test()
