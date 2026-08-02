import os

with open("web/index.html", "r", encoding="utf-8") as f:
    lines = f.readlines()

style_start, style_end = -1, -1
script_start, script_end = -1, -1

for i, line in enumerate(lines):
    if "<style>" in line and style_start == -1: style_start = i
    if "</style>" in line and style_end == -1: style_end = i
    if "<script>" in line and script_start == -1: script_start = i
    if "</script>" in line and script_end == -1: script_end = i

os.makedirs('web/css', exist_ok=True)
os.makedirs('web/js', exist_ok=True)

# 1. CSS
with open('web/css/main.css', 'w', encoding="utf-8") as f:
    f.writelines(lines[style_start+1:style_end])

# 2. JS
with open('web/js/app.js', 'w', encoding="utf-8") as f:
    f.writelines(lines[script_start+1:script_end])

# 3. HTML
new_html = lines[:style_start] + ['  <link rel="stylesheet" href="/web/css/main.css">\n'] + lines[style_end+1:script_start] + ['  <script src="/web/js/app.js"></script>\n'] + lines[script_end+1:]
with open('web/index.html', 'w', encoding="utf-8") as f:
    f.writelines(new_html)

print("Precise split complete.")
