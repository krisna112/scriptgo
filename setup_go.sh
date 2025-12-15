#!/bin/bash
# Xray Panel Go Installer
# This script sets up the Go-based Xray Panel

if [ "${EUID}" -ne 0 ]; then
    echo "❌ Please run as root"
    exit 1
fi

APP_DIR="/usr/local/xray_panel"
BIN_NAME="xray_panel"
REPO_BIN="https://github.com/your-repo/releases/download/v1.0.0/xray_panel_linux_amd64" # Placeholder

echo "📥 Installing Xray Panel (Go Edition)..."

# 0. Dependencies (Merged from setup.sh)
echo "📦 Installing System Dependencies..."
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq wget curl git jq net-tools zip unzip socat qrencode bc nginx certbot python3-certbot-nginx ufw fail2ban

# Create directories
mkdir -p /etc/xray /var/log/xray /usr/local/etc/xray
touch /etc/xray/clients.db /etc/xray/inbounds.db
chmod 666 /etc/xray/{clients.db,inbounds.db}

# 1. Prepare Directory
mkdir -p "$APP_DIR"
cp -r templates "$APP_DIR/" 2>/dev/null
cp -r static "$APP_DIR/" 2>/dev/null

# 2. Build or Copy Binary
# Assuming we are running this where the binary is present or we build it
if [ -f "xray_panel" ]; then
    cp xray_panel "$APP_DIR/$BIN_NAME"
else
    echo "⚠️ Binary not found locally. Building..."
    if command -v go &> /dev/null; then
        cd go_panel
        go build -o ../xray_panel cmd/main.go
        cd ..
        cp xray_panel "$APP_DIR/$BIN_NAME"
    else
        echo "❌ Go not installed and binary not found."
        exit 1
    fi
fi

chmod +x "$APP_DIR/$BIN_NAME"

# 3. Systemd Service (Server Mode)
cat > /etc/systemd/system/xray-panel.service <<EOF
[Unit]
Description=Xray Panel (Go)
After=network.target xray.service

[Service]
Type=simple
User=root
WorkingDirectory=$APP_DIR
ExecStart=$APP_DIR/$BIN_NAME -port 2053
Restart=always

[Install]
WantedBy=multi-user.target
EOF

# 4. Systemd Timer (Expiry Check)
cat > /etc/systemd/system/xray-xp.service <<EOF
[Unit]
Description=Xray Panel Expiry Check

[Service]
Type=oneshot
ExecStart=$APP_DIR/$BIN_NAME -xp
EOF

cat > /etc/systemd/system/xray-xp.timer <<EOF
[Unit]
Description=Run Xray Panel Expiry Check Daily
[Timer]
OnCalendar=daily
Persistent=true
[Install]
WantedBy=timers.target
EOF

# 5. Systemd Timer (Quota Check)
cat > /etc/systemd/system/xray-quota.service <<EOF
[Unit]
Description=Xray Panel Quota Check

[Service]
Type=oneshot
ExecStart=$APP_DIR/$BIN_NAME -quota
EOF

cat > /etc/systemd/system/xray-quota.timer <<EOF
[Unit]
Description=Run Xray Panel Quota Check Regularly
[Timer]
OnBootSec=2min
OnUnitActiveSec=5min
[Install]
WantedBy=timers.target
EOF

# 6. Alias for CLI
echo "alias menu='$APP_DIR/$BIN_NAME -menu'" >> /root/.bashrc

# Reload and Start
systemctl daemon-reload
systemctl enable --now xray-panel
systemctl enable --now xray-xp.timer
systemctl enable --now xray-quota.timer

echo "✅ Installation Complete!"
echo "   Run 'menu' to access the panel."
