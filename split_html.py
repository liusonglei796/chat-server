import re
import os

with open("web/index.html", "r", encoding="utf-8") as f:
    content = f.read()

# Extract styles
style_match = re.search(r'<style>(.*?)</style>', content, re.DOTALL)
if style_match:
    styles = style_match.group(1)
    os.makedirs('web/css', exist_ok=True)
    with open('web/css/main.css', 'w', encoding="utf-8") as f:
        f.write(styles)

# Extract scripts
script_match = re.search(r'<script>(.*?)</script>', content, re.DOTALL)
if script_match:
    scripts = script_match.group(1)
    os.makedirs('web/js', exist_ok=True)
    with open('web/js/app.js', 'w', encoding="utf-8") as f:
        f.write(scripts)

# Create new index.html
new_html = re.sub(r'<style>.*?</style>', '<link rel="stylesheet" href="/static/css/main.css">', content, flags=re.DOTALL)
new_html = re.sub(r'<script>.*?</script>', '<script src="/static/js/app.js"></script>', new_html, flags=re.DOTALL)

with open('web/index.html', 'w', encoding="utf-8") as f:
    f.write(new_html)

print("Split complete.")
