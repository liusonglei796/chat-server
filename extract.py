import re
import os

with open("web/index.html", "r", encoding="utf-8") as f:
    content = f.read()

# Replace <style> block
style_match = re.search(r'<style>.*?</style>', content, re.DOTALL)
if style_match:
    style_content = style_match.group(0).replace('<style>', '').replace('</style>', '')
    os.makedirs('web/css', exist_ok=True)
    with open('web/css/main.css', 'w', encoding="utf-8") as f:
        f.write(style_content)
    
    # Replace in content
    content = content[:style_match.start()] + '<link rel="stylesheet" href="/web/css/main.css">' + content[style_match.end():]

# Look for all script blocks
script_matches = list(re.finditer(r'<script>.*?</script>', content, re.DOTALL))
if script_matches:
    # Just take the last script block, assuming it's the main app.js
    last_script = script_matches[-1]
    script_content = last_script.group(0).replace('<script>', '').replace('</script>', '')
    
    os.makedirs('web/js', exist_ok=True)
    with open('web/js/app.js', 'w', encoding="utf-8") as f:
        f.write(script_content)
        
    content = content[:last_script.start()] + '<script src="/web/js/app.js"></script>' + content[last_script.end():]

with open('web/index.html', 'w', encoding="utf-8") as f:
    f.write(content)

print("Extraction complete.")
