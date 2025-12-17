#!/bin/bash
# Xray Panel Go Installer - Fixed & Robust Version

# Warna untuk output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

if [ "${EUID}" -ne 0 ]; then
    echo -e "${RED}❌ Please run as root${NC}"
    exit 1
fi

APP_DIR="/usr/local/xray_panel"
BIN_NAME="xray_panel"

echo -e "${GREEN}📥 Installing Xray Panel (Go Edition)...${NC}"

# 1. SETUP ENVIRONMENT & DEPENDENCIES
echo -e "\n${YELLOW}📦 Installing System Dependencies...${NC}"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq wget curl git jq net-tools zip unzip socat qrencode bc nginx certbot python3-certbot-nginx python3-certbot-dns-cloudflare ufw fail2ban build-essential

# 2. CREATE SWAP (PENTING AGAR TIDAK ERROR SAAT BUILD)
# Cek jika swap kurang dari 1GB, buat swap file
SWAP_SIZE=$(free -m | grep Swap | awk '{print $2}')
if [ "$SWAP_SIZE" -lt 1000 ]; then
    echo -e "${YELLOW}⚠️  Low Memory detected. Creating 2GB Swap file for building...${NC}"
    fallocate -l 2G /swapfile
    chmod 600 /swapfile
    mkswap /swapfile
    swapon /swapfile
    echo '/swapfile none swap sw 0 0' >> /etc/fstab
    echo -e "${GREEN}✅ Swap created!${NC}"
fi

# 3. DOMAIN & CLOUDFLARE SETUP
echo ""
if [ -f "/root/domain" ]; then
    DOMAIN=$(cat /root/domain)
    echo -e "Using saved domain: ${GREEN}$DOMAIN${NC}"
else
    read -p "▶️  Enter your domain: " DOMAIN
    echo "$DOMAIN" > /root/domain
fi

# Cloudflare Credentials (Optional but Recommended)
if [ ! -f "/root/.secrets/cloudflare.ini" ]; then
    echo ""
    echo -e "${YELLOW}🔑 Cloudflare Credentials (Required for Auto-SSL)${NC}"
    echo "Press Enter to skip if you don't use Cloudflare DNS."
    read -p "▶️  Generic Cloudflare Email: " CF_EMAIL
    read -p "▶️  Global API Key: " CF_KEY

    if [ -n "$CF_EMAIL" ] && [ -n "$CF_KEY" ]; then
        echo "$CF_EMAIL" > /root/cf_email
        echo "$CF_KEY" > /root/cf_key
        
        mkdir -p /root/.secrets
        echo "dns_cloudflare_email = $CF_EMAIL" > /root/.secrets/cloudflare.ini
        echo "dns_cloudflare_api_key = $CF_KEY" >> /root/.secrets/cloudflare.ini
        chmod 600 /root/.secrets/cloudflare.ini
    fi
fi

# 4. INSTALL XRAY CORE
echo -e "\n${YELLOW}🚀 Installing Xray Core...${NC}"
bash -c "$(curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ install
mkdir -p /var/log/xray
chown -R nobody:nogroup /var/log/xray

# Fix Permissions
sed -i 's/User=nobody/User=root/g' /etc/systemd/system/xray.service
sed -i 's/User=nobody/User=root/g' /lib/systemd/system/xray.service 2>/dev/null
systemctl daemon-reload

# 5. GENERATE DEFAULT CONFIG
echo -e "${YELLOW}⚙️  Generating default Xray config...${NC}"
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
mkdir -p /etc/xray
touch /etc/xray/clients.db /etc/xray/inbounds.db
chmod 666 /etc/xray/{clients.db,inbounds.db}

# 6. SSL CERTIFICATE
if [ -f "/root/.secrets/cloudflare.ini" ]; then
    echo -e "${YELLOW}🔐 Requesting SSL Certificate via Cloudflare...${NC}"
    certbot certonly --dns-cloudflare --dns-cloudflare-credentials /root/.secrets/cloudflare.ini \
    --agree-tos --email "$(cat /root/cf_email)" --non-interactive \
    -d "$DOMAIN" -d "*.$DOMAIN"
    
    ln -sf "/etc/letsencrypt/live/$DOMAIN/fullchain.pem" /etc/xray/xray.crt
    ln -sf "/etc/letsencrypt/live/$DOMAIN/privkey.pem" /etc/xray/xray.key
    echo "$DOMAIN" > /etc/xray/domain
else
    echo -e "${RED}⚠️  Skipping SSL: No Cloudflare credentials provided.${NC}"
fi

# 7. BUILD & INSTALL PANEL (THE CRITICAL PART)
echo -e "\n${YELLOW}🔨 Building Xray Panel Binary...${NC}"

# Prepare Directory
mkdir -p "$APP_DIR"

# Install Go if missing
if ! command -v go &> /dev/null; then
    echo "⚠️ Go not found. Installing Go..."
    wget -q https://go.dev/dl/go1.22.5.linux-amd64.tar.gz
    rm -rf /usr/local/go && tar -C /usr/local -xzf go1.22.5.linux-amd64.tar.gz
    export PATH=$PATH:/usr/local/go/bin
    echo "export PATH=\$PATH:/usr/local/go/bin" >> /etc/profile
fi

# Build Process
if [ -d "go_panel" ]; then
    cd go_panel
    
    echo "   - Downloading Go modules..."
    go mod tidy
    
    echo "   - Compiling (this may take a while)..."
    go build -o ../xray_panel cmd/main.go
    
    if [ $? -ne 0 ]; then
        echo -e "${RED}❌ Build Failed! Check errors above.${NC}"
        exit 1
    fi
    
    cd ..
else
    echo -e "${RED}❌ Error: Directory 'go_panel' not found!${NC}"
    echo "Make sure you run this script inside the repo folder."
    exit 1
fi

# Check and Install Binary
if [ -f "xray_panel" ]; then
    cp xray_panel "$APP_DIR/$BIN_NAME"
    chmod +x "$APP_DIR/$BIN_NAME"
    
    # Copy Assets
    cp -r go_panel/templates "$APP_DIR/" 2>/dev/null
    cp -r go_panel/static "$APP_DIR/" 2>/dev/null
    
    echo -e "${GREEN}✅ Build & Install Success!${NC}"
else
    echo -e "${RED}❌ Critical Error: Binary 'xray_panel' was not created.${NC}"
    exit 1
fi

# 8. SYSTEMD SERVICES
echo -e "${YELLOW}⚙️  Configuring Services...${NC}"

# Main Service
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

# Expiry Check
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

# 9. CLI SHORTCUT
echo "#!/bin/bash" > /usr/bin/menu
echo "$APP_DIR/$BIN_NAME -menu" >> /usr/bin/menu
chmod +x /usr/bin/menu

# 10. START SERVICES
systemctl daemon-reload
systemctl enable --now xray-panel
systemctl enable --now xray-xp.timer

echo -e "\n${GREEN}=========================================${NC}"
echo -e "${GREEN}      ✅ INSTALLATION COMPLETE!          ${NC}"
echo -e "${GREEN}=========================================${NC}"
echo -e "▶️  Type ${YELLOW}menu${NC} to access the panel."
echo -e "▶️  Web Panel: http://$DOMAIN:2053"
