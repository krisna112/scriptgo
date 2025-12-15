#!/bin/bash

REPO="https://raw.githubusercontent.com/krisna112/scriptpanelvps/main"
BACKUP_DIR="/root/xray-backup-$(date +%Y%m%d-%H%M%S)"


# Helper Function for Update Logging
function update_file() {
    local target="$1"
    local url="$2"
    local name=$(basename "$target")
    
    echo -n -e "  - Updating $name... "
    if wget -q -O "$target" "$url"; then
        chmod +x "$target"
        echo -e "\033[1;32m[OK]\033[0m"
    else
        echo -e "\033[1;31m[FAILED]\033[0m"
    fi
}

echo -e "\033[1;33m🔄 Updating VPS Components...\033[0m"

# 1. Install Dependencies
apt install -y jq > /dev/null 2>&1

# 2. Backup Config
mkdir -p "$BACKUP_DIR"
cp -r /etc/xray "$BACKUP_DIR/" 2>/dev/null
cp /usr/local/etc/xray/config.json "$BACKUP_DIR/" 2>/dev/null

# 3. Download Core Scripts (Force Cache Bypass)
# Mengambil menu dari folder menu_cli sesuai request Anda
update_file "/usr/bin/menu" "${REPO}/menu_cli/menu.sh?t=$(date +%s)"
update_file "/usr/bin/menu-user.sh" "${REPO}/menu_cli/menu-user.sh?t=$(date +%s)"
update_file "/usr/bin/menu-monitor.sh" "${REPO}/menu_cli/menu-monitor.sh?t=$(date +%s)"
update_file "/usr/bin/xp.sh" "${REPO}/xp.sh?t=$(date +%s)"
update_file "/usr/bin/quota.sh" "${REPO}/quota.sh?t=$(date +%s)"

# 4. Update Bot Telegram
mkdir -p /usr/lib/xray-telegram-bot
update_file "/usr/lib/xray-telegram-bot/xray_utils.py" "${REPO}/telegram_bot/xray_utils.py?t=$(date +%s)"
update_file "/usr/lib/xray-telegram-bot/handlers.py" "${REPO}/telegram_bot/handlers.py?t=$(date +%s)"
update_file "/usr/lib/xray-telegram-bot/bot.py" "${REPO}/telegram_bot/bot.py?t=$(date +%s)"
update_file "/usr/lib/xray-telegram-bot/utils.py" "${REPO}/telegram_bot/utils.py?t=$(date +%s)"

# 5. Update Web Panel (Full Sync)
echo -e "\033[1;33m🔄 Updating Web Panel...\033[0m"
WEB_DIR="/opt/xray-web-panel"
mkdir -p "$WEB_DIR/templates"

update_file "$WEB_DIR/app.py" "${REPO}/web_panel/app.py?t=$(date +%s)"
update_file "$WEB_DIR/templates/dashboard.html" "${REPO}/web_panel/templates/dashboard.html?t=$(date +%s)"
update_file "$WEB_DIR/templates/form.html" "${REPO}/web_panel/templates/form.html?t=$(date +%s)"
update_file "$WEB_DIR/templates/login.html" "${REPO}/web_panel/templates/login.html?t=$(date +%s)"
update_file "$WEB_DIR/templates/settings.html" "${REPO}/web_panel/templates/settings.html?t=$(date +%s)"
update_file "$WEB_DIR/templates/base.html" "${REPO}/web_panel/templates/base.html?t=$(date +%s)"

# Restart Web Panel jika servicenya ada
if systemctl is-active --quiet xray-web; then 
    systemctl restart xray-web
fi

# 6. Reset Services (Ensure Anti-Reset is Active)
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

systemctl daemon-reload
systemctl enable xray-save.service
systemctl restart xray-quota.timer xray-expiry.timer
systemctl start xray-save.service
if systemctl is-active --quiet telegram-bot; then systemctl restart telegram-bot; fi

# 7. Update Version
curl -s "${REPO}/version?t=$(date +%s)" > /etc/xray/version 2>/dev/null

echo -e "\033[1;32m✅ Update Complete! Version synced.\033[0m"
