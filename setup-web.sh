#!/bin/bash
# Xray Web Panel Installer v3.1 (Stable)
# Requires files to be in 'web_panel' folder in GitHub repo

REPO="https://raw.githubusercontent.com/krisna112/scriptpanelvps/main"
WEB_DIR="/opt/xray-web-panel"

echo "📦 [1/5] Installing System Dependencies..."
export DEBIAN_FRONTEND=noninteractive
apt update -qq
apt install -y python3 python3-pip python3-venv python3-full nginx curl unzip

# 1. Setup Directory
echo "📂 [2/5] Setting up directories..."
rm -rf "$WEB_DIR"
mkdir -p "$WEB_DIR/templates"

# 2. Setup Python Venv (Improved)
echo "🐍 [3/5] Setting up Python Environment..."
python3 -m venv "$WEB_DIR/venv"

# Update PIP dulu agar tidak error saat install library
"$WEB_DIR/venv/bin/pip" install --upgrade pip

# Install Library (dipisah agar lebih jelas log-nya)
echo "   - Installing Flask & Gunicorn..."
"$WEB_DIR/venv/bin/pip" install flask psutil requests gunicorn

# 3. Download Files from Repo
echo "⬇️  [4/5] Downloading Web Panel files..."
# Download Logic (App)
wget -q -O "$WEB_DIR/app.py" "${REPO}/web_panel/app.py"
wget -q -O "$WEB_DIR/security.py" "${REPO}/web_panel/security.py"

# Download Templates
wget -q -O "$WEB_DIR/templates/base.html" "${REPO}/web_panel/templates/base.html"
wget -q -O "$WEB_DIR/templates/dashboard.html" "${REPO}/web_panel/templates/dashboard.html"
wget -q -O "$WEB_DIR/templates/login.html" "${REPO}/web_panel/templates/login.html"
wget -q -O "$WEB_DIR/templates/form.html" "${REPO}/web_panel/templates/form.html"
wget -q -O "$WEB_DIR/templates/settings.html" "${REPO}/web_panel/templates/settings.html"

# Pastikan permission benar
chmod -R 755 "$WEB_DIR"
chown -R root:root "$WEB_DIR"

# 4. Create Default Admin Config (First Run)
WEB_USER=$(cat /root/web_user 2>/dev/null || echo "admin")
WEB_PASS=$(cat /root/web_pass 2>/dev/null || echo "admin123")
WEB_PORT=$(cat /root/web_port 2>/dev/null || echo "2053")

if [ ! -f "/etc/xray/web_admin.json" ]; then
    echo "{\"username\": \"$WEB_USER\", \"password\": \"$WEB_PASS\"}" > /etc/xray/web_admin.json
    chmod 666 /etc/xray/web_admin.json
fi

# 5. Nginx Configuration (SSL)
echo "⚙️ [5/5] Configuring Nginx..."
DOMAIN=$(cat /root/domain)
cat > /etc/nginx/conf.d/xray-panel.conf << EOF
server {
    listen $WEB_PORT ssl http2;
    server_name $DOMAIN;

    ssl_certificate /etc/xray/xray.crt;
    ssl_certificate_key /etc/xray/xray.key;
    
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384:ECDHE-ECDSA-CHACHA20-POLY1305:ECDHE-RSA-CHACHA20-POLY1305:DHE-RSA-AES128-GCM-SHA256:DHE-RSA-AES256-GCM-SHA384;
    
    location / {
        proxy_pass http://127.0.0.1:5000;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    }
}
EOF
ufw allow $WEB_PORT/tcp >/dev/null 2>&1

# 6. Service Configuration
cat > /etc/systemd/system/xray-web.service << EOF
[Unit]
Description=Xray Web Panel
After=network.target

[Service]
User=root
WorkingDirectory=$WEB_DIR
ExecStart=$WEB_DIR/venv/bin/gunicorn --workers 3 --bind 0.0.0.0:5000 app:app
Restart=always

[Install]
WantedBy=multi-user.target
EOF

# 7. Start
systemctl daemon-reload
systemctl restart nginx
systemctl enable xray-web
systemctl restart xray-web

echo ""
echo "✅ Web Panel Installed & Synced from Repo!"
echo "🔗 URL: https://$DOMAIN:$WEB_PORT"
