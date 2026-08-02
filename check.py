with open("web/index.html", "r", encoding="utf-8") as f:
    lines = f.readlines()

for i, line in enumerate(lines):
    if "<style" in line: print(f"style at {i}")
    if "</style" in line: print(f"/style at {i}")
    if "<script" in line: print(f"script at {i}")
    if "</script" in line: print(f"/script at {i}")
