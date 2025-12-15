#!/bin/bash
# Security Tools Installer - VPS COMPLIANT
# NO Squid, NO Dropbear, NO Open Proxy

echo "📦 Installing security tools..."

export DEBIAN_FRONTEND=noninteractive
apt install -y -qq fail2ban iptables-persistent

# Fail2Ban
cat > /etc/fail2ban/jail.local <<'EOF'
[DEFAULT]
bantime = 3600
findtime = 600
maxretry = 3

[sshd]
enabled = true
port = ssh
filter = sshd
logpath = /var/log/auth.log
maxretry = 3
EOF

systemctl enable fail2ban
systemctl restart fail2ban

echo "✅ Security tools installed (SECURE MODE)"
