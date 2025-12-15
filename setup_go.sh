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

# 0.1 Domain & Cloudflare Setup (Restoring Original Functionality)
echo ""
if [ -f "/root/domain" ]; then
    DOMAIN=$(cat /root/domain)
    echo "Using saved domain: $DOMAIN"
else
    read -p "▶️  Enter your domain: " DOMAIN
    echo "$DOMAIN" > /root/domain
fi

echo ""
echo "🔑 Cloudflare Credentials (Required for Auto-SSL)"
read -p "▶️  Generic Cloudflare Email: " CF_EMAIL
read -p "▶️  Global API Key: " CF_KEY

if [ -n "$CF_EMAIL" ] && [ -n "$CF_KEY" ]; then
    echo "$CF_EMAIL" > /root/cf_email
    echo "$CF_KEY" > /root/cf_key
    
    # Write credentials for certbot-dns-cloudflare
    mkdir -p /root/.secrets
    echo "dns_cloudflare_email = $CF_EMAIL" > /root/.secrets/cloudflare.ini
    echo "dns_cloudflare_api_key = $CF_KEY" >> /root/.secrets/cloudflare.ini
    chmod 600 /root/.secrets/cloudflare.ini
fi

# 0.2 Dependencies (Merged from setup.sh)
echo "📦 Installing System Dependencies..."
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq wget curl git jq net-tools zip unzip socat qrencode bc nginx certbot python3-certbot-nginx python3-certbot-dns-cloudflare ufw fail2ban

# 0.3 Install Xray Core (CRITICAL FIX)
# 0.3 Install Xray Core (CRITICAL FIX)
echo "🚀 Installing Xray Core..."
bash -c "$(curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ install
# Ensure log directory exists
mkdir -p /var/log/xray
chown -R nobody:nogroup /var/log/xray

# FIX PERMISSIONS: Force Xray to run as root (Solves Certbot permission denied)
sed -i 's/User=nobody/User=root/g' /etc/systemd/system/xray.service
sed -i 's/User=nobody/User=root/g' /lib/systemd/system/xray.service 2>/dev/null
systemctl daemon-reload

# 0.4 Default Config Generation
# Xray won't start without a valid config.
echo "⚙️ Generating default Xray config..."
cat > /usr/local/etc/xray/config.json <<EOF
{
  "log": {
    "access": "/var/log/xray/access.log",
    "error": "/var/log/xray/error.log",
    "loglevel": "warning"
  },
  "inbounds": [],
  "outbounds": [
    {
      "protocol": "freedom",
      "settings": {}
    },
    {
      "protocol": "blackhole",
      "settings": {},
      "tag": "blocked"
    }
  ],
  "routing": {
    "rules": [
      {
        "type": "field",
        "ip": ["geoip:private"],
        "outboundTag": "blocked"
      }
    ]
  }
}
EOF

# Create DB files
mkdir -p /etc/xray /usr/local/etc/xray
touch /etc/xray/clients.db /etc/xray/inbounds.db
chmod 666 /etc/xray/{clients.db,inbounds.db}

# 0.5 SSL Generation (Critical Step)
if [ -f "/root/.secrets/cloudflare.ini" ]; then
    echo "🔐 Requesting SSL Certificate via Cloudflare..."
    certbot certonly --dns-cloudflare --dns-cloudflare-credentials /root/.secrets/cloudflare.ini \
    --agree-tos --email "$CF_EMAIL" --non-interactive \
    -d "$DOMAIN" -d "*.$DOMAIN"
    
    # Link certs to /etc/xray
    ln -sf "/etc/letsencrypt/live/$DOMAIN/fullchain.pem" /etc/xray/xray.crt
    ln -sf "/etc/letsencrypt/live/$DOMAIN/privkey.pem" /etc/xray/xray.key
    echo "✅ SSL Certificate Installed!"
else
    echo "⚠️ Skipping SSL: No Cloudflare credentials provided."
fi

# 1. Prepare Directory
# 1. Prepare Directory
mkdir -p "$APP_DIR"
# Copy from go_panel directory (source of truth now)
cp -r go_panel/templates "$APP_DIR/" 2>/dev/null
cp -r go_panel/static "$APP_DIR/" 2>/dev/null

# 2. Build or Copy Binary
# Assuming we are running this where the binary is present or we build it
if [ -f "xray_panel" ]; then
    cp xray_panel "$APP_DIR/$BIN_NAME"
else
    echo "⚠️ Binary not found locally. Preparing to build..."
    
    # Auto-Install Go if missing
    if ! command -v go &> /dev/null; then
        echo "⚠️ Go not found. Installing Go..."
        wget -q https://go.dev/dl/go1.22.5.linux-amd64.tar.gz
        rm -rf /usr/local/go && tar -C /usr/local -xzf go1.22.5.linux-amd64.tar.gz
        export PATH=$PATH:/usr/local/go/bin
        rm -f go1.22.5.linux-amd64.tar.gz
        echo "✅ Go installed!"
    fi

    echo "🔨 Building Binary..."
    if command -v go &> /dev/null; then
        cd go_panel
        go mod tidy
        go build -o ../xray_panel cmd/main.go
        cd ..
        cp xray_panel "$APP_DIR/$BIN_NAME"
        echo "✅ Build Success!"
    else
        echo "❌ Critical Error: Failed to install or run Go."
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
# 6. Global Launcher for CLI
echo "#!/bin/bash" > /usr/bin/menu
echo "$APP_DIR/$BIN_NAME -menu" >> /usr/bin/menu
chmod +x /usr/bin/menu

# Reload and Start
systemctl daemon-reload
systemctl enable --now xray-panel
systemctl enable --now xray-xp.timer
systemctl enable --now xray-quota.timer

echo "✅ Installation Complete!"
echo "   Run 'menu' to access the panel."
