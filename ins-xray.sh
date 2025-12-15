#!/bin/bash
# Xray Core Installation - SECURE & COMPLIANT

DOMAIN=$(cat /root/domain 2>/dev/null || echo "localhost")
CF_EMAIL=$(cat /root/cf_email 2>/dev/null)
CF_KEY=$(cat /root/cf_key 2>/dev/null)

echo "🔧 Installing Xray..."

# Install Xray
bash -c "$(curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ install

if [ ! -f "/usr/local/bin/xray" ]; then
    echo "❌ Failed!"
    exit 1
fi

# SSL Certificate
mkdir -p /etc/xray
if [ -n "$CF_EMAIL" ] && [ -n "$CF_KEY" ]; then
    echo "🔐 Generating SSL..."
    curl -s https://get.acme.sh | sh -s email="$CF_EMAIL"
    export CF_Email="$CF_EMAIL"
    export CF_Key="$CF_KEY"
    ~/.acme.sh/acme.sh --set-default-ca --server letsencrypt
    ~/.acme.sh/acme.sh --issue --dns dns_cf -d "$DOMAIN" -d "*.$DOMAIN" --force
    ~/.acme.sh/acme.sh --installcert -d "$DOMAIN" \
        --fullchainpath /etc/xray/xray.crt \
        --keypath /etc/xray/xray.key \
        --reloadcmd "systemctl restart xray"
else
    openssl req -new -newkey rsa:2048 -days 365 -nodes -x509 \
        -subj "/CN=$DOMAIN" \
        -keyout /etc/xray/xray.key \
        -out /etc/xray/xray.crt
fi

chmod 644 /etc/xray/xray.{crt,key}

# Secure Config
cat > /usr/local/etc/xray/config.json <<'EOF'
{
  "log": {
    "access": "/var/log/xray/access.log",
    "error": "/var/log/xray/error.log",
    "loglevel": "warning"
  },
  "api": {
    "tag": "api",
    "services": ["HandlerService", "LoggerService", "StatsService"]
  },
  "stats": {},
  "policy": {
    "levels": {
      "0": {"statsUserUplink": true, "statsUserDownlink": true}
    },
    "system": {
      "statsInboundUplink": true,
      "statsInboundDownlink": true
    }
  },
  "inbounds": [
    {
      "tag": "api",
      "listen": "127.0.0.1",
      "port": 10085,
      "protocol": "dokodemo-door",
      "settings": {"address": "127.0.0.1"}
    }
  ],
  "outbounds": [
    {
      "protocol": "freedom",
      "settings": {"domainStrategy": "UseIPv4"},
      "tag": "direct"
    },
    {
      "protocol": "blackhole",
      "tag": "blocked"
    }
  ],
  "routing": {
    "rules": [
      {"type": "field", "inboundTag": ["api"], "outboundTag": "api"},
      {"type": "field", "protocol": ["bittorrent"], "outboundTag": "blocked"}
    ]
  },
  "dns": {
    "servers": ["https://1.1.1.1/dns-query", "localhost"]
  }
}
EOF

mkdir -p /var/log/xray
touch /var/log/xray/{access,error}.log
chmod 666 /var/log/xray/*.log

# Service
cat > /etc/systemd/system/xray.service <<'EOF'
[Unit]
Description=Xray VPN Service
After=network.target

[Service]
User=root
ExecStart=/usr/local/bin/xray run -config /usr/local/etc/xray/config.json
Restart=on-failure
LimitNOFILE=1000000

[Install]
WantedBy=multi-user.target
EOF

# Nginx
cat > /etc/nginx/sites-available/xray <<'EOF'
server {
    listen 8080;
    server_name _;
    root /var/www/html;
    
    location ~ ^/(vless-ws|vmess-ws|trojan-ws) {
        if ($http_upgrade != "websocket") { return 404; }
        proxy_pass http://127.0.0.1:10010;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
    
    location ~ ^/(vless-grpc|vmess-grpc|trojan-grpc) {
        grpc_pass grpc://127.0.0.1:2083;
    }
}
EOF

ln -sf /etc/nginx/sites-available/xray /etc/nginx/sites-enabled/
rm -f /etc/nginx/sites-enabled/default
nginx -t && systemctl restart nginx

# Firewall
ufw --force enable
ufw default deny incoming
ufw default allow outgoing
ufw allow 22/tcp
ufw allow 443/tcp
ufw allow 80/tcp
ufw reload

systemctl daemon-reload
systemctl enable xray
systemctl restart xray

sleep 2
if systemctl is-active --quiet xray; then
    echo "✅ Xray installed (SECURE)"
else
    echo "❌ Failed to start"
    exit 1
fi
