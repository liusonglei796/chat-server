import re

with open("web/css/main.css", "r", encoding="utf-8") as f:
    css = f.read()

# 1. Replace variables with premium ones and add dark mode
premium_vars = """
    :root {
      /* Premium HSL Palette */
      --primary: hsl(252, 100%, 65%);
      --primary-hover: hsl(252, 100%, 75%);
      --primary-bg: hsla(252, 100%, 65%, 0.1);
      
      --bg: hsl(220, 20%, 97%);
      --surface: hsla(0, 0%, 100%, 0.7); /* Glass */
      --surface-hover: hsla(0, 0%, 100%, 0.9);
      
      --border: hsla(220, 20%, 80%, 0.4);
      --border-light: hsla(220, 20%, 90%, 0.4);
      
      --text: hsl(220, 20%, 20%);
      --text-secondary: hsl(220, 10%, 40%);
      --text-muted: hsl(220, 10%, 60%);
      
      --danger: hsl(350, 100%, 65%);
      --danger-bg: hsla(350, 100%, 65%, 0.1);
      --success: hsl(150, 80%, 40%);
      --success-bg: hsla(150, 80%, 40%, 0.1);
      
      --shadow-sm: 0 4px 6px -1px rgba(0, 0, 0, 0.05);
      --shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.1);
      --shadow-lg: 0 20px 40px -10px rgba(0, 0, 0, 0.2);
      
      --radius: 12px;
      --radius-lg: 20px;
      --radius-full: 9999px;
      
      --transition: 300ms cubic-bezier(0.34, 1.56, 0.64, 1); /* Spring */
      --sidebar-w: 320px;
      --header-h: 70px;
      
      --glass-blur: blur(20px);
    }
    
    @media (prefers-color-scheme: dark) {
      :root {
        --bg: hsl(220, 25%, 10%);
        --surface: hsla(220, 25%, 15%, 0.65);
        --surface-hover: hsla(220, 25%, 20%, 0.8);
        --border: hsla(220, 20%, 30%, 0.4);
        --border-light: hsla(220, 20%, 20%, 0.4);
        --text: hsl(220, 20%, 95%);
        --text-secondary: hsl(220, 10%, 75%);
        --text-muted: hsl(220, 10%, 50%);
        --shadow: 0 10px 30px -5px rgba(0, 0, 0, 0.5);
      }
    }
"""

css = re.sub(r':root\s*\{[^}]+\}', premium_vars, css, count=1, flags=re.DOTALL)

# 2. Add Google Font Inter
font_import = "@import url('https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap');\n"
if "@import" not in css:
    css = font_import + css

css = re.sub(r"font-family:\s*[^;]+;", "font-family: 'Inter', system-ui, -apple-system, sans-serif;", css)

# 3. Enhance specific components with Glassmorphism
css = css.replace("background: var(--surface);", "background: var(--surface); backdrop-filter: var(--glass-blur); -webkit-backdrop-filter: var(--glass-blur);")
css = css.replace("background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);", "background: linear-gradient(135deg, hsl(252, 100%, 70%) 0%, hsl(280, 100%, 65%) 100%);")
css = css.replace("transition: all .2s;", "transition: all var(--transition);")
css = css.replace("transition: background .2s, color .2s;", "transition: all var(--transition);")

# Update App background to have a nice modern mesh gradient or something
mesh_bg = """
    #chat-app {
      display: flex;
      height: 100vh;
      width: 100vw;
      overflow: hidden;
      background: radial-gradient(circle at top left, hsla(252, 100%, 75%, 0.15), transparent 40%),
                  radial-gradient(circle at bottom right, hsla(280, 100%, 65%, 0.15), transparent 40%);
      background-color: var(--bg);
    }
"""
css = re.sub(r'#chat-app\s*\{[^}]+\}', mesh_bg, css, flags=re.DOTALL)

# Button enhancements
btn_css = """
    .btn {
      transition: all var(--transition);
      transform: scale(1);
    }
    .btn:active {
      transform: scale(0.95);
    }
    .btn:hover {
      transform: translateY(-2px);
      box-shadow: 0 5px 15px var(--primary-bg);
    }
"""
css += btn_css

# Save back
with open("web/css/main.css", "w", encoding="utf-8") as f:
    f.write(css)

print("CSS Optimization complete.")
