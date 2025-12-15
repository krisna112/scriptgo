#!/bin/bash
# Telegram Bot Setup v3.2.1 - Modular Version

RED='\033[0;31m'; GREEN='\033[0;32m'; NC='\033[0m'

echo -e "${GREEN}Installing Telegram Bot (Modular)...${NC}"

# Install Python & Venv
apt update -qq
apt install -y python3 python3-venv python3-full

# Create Venv
rm -rf /opt/xray-telegram-bot
python3 -m venv /opt/xray-telegram-bot

# Install Deps (FIXED: Added qrcode and pillow for QR Support)
/opt/xray-telegram-bot/bin/pip install python-telegram-bot qrcode[pil] pillow

# Create Directories
DIR="/usr/lib/xray-telegram-bot"
mkdir -p $DIR

# NOTE: Script files should be downloaded here from repo.
# Ensure your setup.sh or update script downloads the python files to $DIR

# Create Config (Hanya jika belum ada)
if [ ! -f /etc/xray/telegram-bot.conf ]; then
    read -p "Bot Token: " TOKEN
    read -p "Admin ID: " ADMIN
    echo "TELEGRAM_BOT_TOKEN=\"$TOKEN\"" > /etc/xray/telegram-bot.conf
    echo "TELEGRAM_ADMIN_ID=\"$ADMIN\"" >> /etc/xray/telegram-bot.conf
fi

# Create Service
cat > /etc/systemd/system/telegram-bot.service <<EOF
[Unit]
Description=Xray Telegram Bot
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=$DIR
EnvironmentFile=/etc/xray/telegram-bot.conf
ExecStart=/opt/xray-telegram-bot/bin/python3 $DIR/bot.py
Restart=always

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable telegram-bot
systemctl start telegram-bot

echo -e "${GREEN}Done!${NC}"
