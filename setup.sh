#!/bin/bash
# Advanced Xray Panel Installer v3.3.2 STABLE
# VPS Compliant - No Open Proxy - Private VPN Only

REPO="https://raw.githubusercontent.com/krisna112/scriptpanelvps/main"

if [ "${EUID}" -ne 0 ]; then
    echo "❌ Script harus dijalankan sebagai root!"
    exit 1
fi

clear
echo "╔══════════════════════════════════════════════════════╗"
echo "║     ADVANCED XRAY PANEL v3.3.2                       ║"
echo "╚══════════════════════════════════════════════════════╝"
echo ""
echo "🔒 Security: Enabled"
echo "💾 Persistence: Enabled (Anti-Reset Traffic)"
echo ""
read -p "▶️  Continue Installation? (y/n): " confirm
if [ "$confirm" != "y" ]; then exit 0; fi

# Update system & Install Dependencies (JQ WAJIB ADA)
echo ""
echo "📦 Installing dependencies..."
export DEBIAN_FRONTEND=noninteractive
apt update -qq && apt upgrade -y -qq
apt install -y -qq wget curl git jq net-tools zip unzip socat qrencode bc nginx certbot python3-certbot-nginx ufw fail2ban

# Create directories
mkdir -p /etc/xray /var/log/xray /usr/local/etc/xray
touch /etc/xray/clients.db /etc/xray/inbounds.db
chmod 666 /etc/xray/{clients.db,inbounds.db}
chmod 755 /var/log/xray

# Domain Setup
if [ -f "/root/domain" ]; then
    DOMAIN=$(cat /root/domain)
else
    read -p "▶️  Enter your domain: " DOMAIN
    echo "$DOMAIN" > /root/domain
fi

# Cloudflare Credentials (Required for SSL & Web Panel)
echo ""
echo "🔑 Cloudflare Credentials (Required for Auto-SSL)"
echo "   Please enter your Cloudflare Email and Global API Key."
echo "   (Leave empty if you don't have them, but SSL might fail for Web Panel)"
read -p "▶️  Generic Cloudflare Email: " CF_EMAIL
read -p "▶️  Global API Key: " CF_KEY

if [ -n "$CF_EMAIL" ] && [ -n "$CF_KEY" ]; then
    echo "$CF_EMAIL" > /root/cf_email
    echo "$CF_KEY" > /root/cf_key
fi

# Web Panel Configuration (Custom Port & Credentials)
echo ""
echo "⚙️  Web Panel Configuration"
read -p "▶️  Set Web Panel Port (Default: 2053): " WEB_PORT
read -p "▶️  Set Admin Username (Default: admin): " WEB_USER
read -p "▶️  Set Admin Password (Default: admin123): " WEB_PASS

# Set Defaults if empty
WEB_PORT=${WEB_PORT:-2053}
WEB_USER=${WEB_USER:-admin}
WEB_PASS=${WEB_PASS:-admin123}

# Save to files for setup-web.sh to read
echo "$WEB_PORT" > /root/web_port
echo "$WEB_USER" > /root/web_user
echo "$WEB_PASS" > /root/web_pass

# Download scripts
echo "📥 Downloading scripts..."
wget -q -O /root/ssh-vpn.sh "${REPO}/ssh-vpn.sh" && chmod +x /root/ssh-vpn.sh
wget -q -O /root/ins-xray.sh "${REPO}/ins-xray.sh" && chmod +x /root/ins-xray.sh
wget -q -O /root/setup-web.sh "${REPO}/setup-web.sh" && chmod +x /root/setup-web.sh
wget -q -O /usr/bin/menu "${REPO}/menu_cli/menu.sh" && chmod +x /usr/bin/menu
wget -q -O /usr/bin/xp.sh "${REPO}/xp.sh" && chmod +x /usr/bin/xp.sh
# Download Quota Script Terbaru
wget -q -O /usr/bin/quota.sh "${REPO}/quota.sh" && chmod +x /usr/bin/quota.sh

# Execute Installers
bash /root/ssh-vpn.sh
bash /root/ins-xray.sh
bash /root/setup-web.sh

# Configure Systemd Services
echo "⏰ Configuring services..."

# 1. Quota Check (Regular)
cat > /etc/systemd/system/xray-quota.service <<'EOF'
[Unit]
Description=Xray Quota Checker
After=xray.service
[Service]
Type=oneshot
ExecStart=/usr/bin/quota.sh
EOF

cat > /etc/systemd/system/xray-quota.timer <<'EOF'
[Unit]
Description=Quota Check Timer
[Timer]
OnBootSec=2min
OnUnitActiveSec=1min
Persistent=true
[Install]
WantedBy=timers.target
EOF

# 2. Expiry Check
cat > /etc/systemd/system/xray-expiry.service <<'EOF'
[Unit]
Description=Xray Expiry Checker
After=xray.service
[Service]
Type=oneshot
ExecStart=/usr/bin/xp.sh
EOF

cat > /etc/systemd/system/xray-expiry.timer <<'EOF'
[Unit]
Description=Expiry Check Timer
[Timer]
OnCalendar=daily
Persistent=true
[Install]
WantedBy=timers.target
EOF

# 3. SAVE DATA SERVICE (ANTI-RESET FIX)
# Ini kunci agar data tersimpan saat Reboot
cat > /etc/systemd/system/xray-save.service <<'EOF'
[Unit]
Description=Save Xray Traffic Data Before Shutdown
After=xray.service
DefaultDependencies=no
Before=shutdown.target reboot.target halt.target
[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/bin/true
ExecStop=/usr/bin/quota.sh
TimeoutStopSec=10
[Install]
WantedBy=multi-user.target
EOF

# Enable & Start
systemctl daemon-reload
systemctl enable xray-quota.timer xray-expiry.timer xray-save.service
systemctl start xray-quota.timer xray-expiry.timer xray-save.service

# Finalize
echo 'alias menu="/usr/bin/menu"' >> /root/.bashrc
echo "3.3.2" > /etc/xray/version
rm -f /root/setup.sh /root/ssh-vpn.sh /root/ins-xray.sh /root/setup-web.sh
history -c

echo "✅ INSTALLATION COMPLETED. REBOOTING..."
echo "---------------------------------------------------------"
echo "🌐 Web Panel: https://$DOMAIN:$WEB_PORT"
echo "👤 User: $WEB_USER"
echo "🔑 Pass: $WEB_PASS"
echo "---------------------------------------------------------"
sleep 5
reboot
