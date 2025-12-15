#!/bin/bash
# Expiry Checker (Secondary Enforcer)
# Runs daily to clean up and double-check expirations

CONFIG="/usr/local/etc/xray/config.json"
DB="/etc/xray/clients.db"

[ ! -f "$DB" ] && exit 0

tmp=$(mktemp)
need_restart=false
now_epoch=$(date +%s)

while IFS=';' read -r user quota used exp proto id; do
    if [ -z "$user" ]; then continue; fi
    
    # Lewati jika sudah ditandai mati
    if [[ "$proto" == *"EXPIRED"* ]] || [[ "$proto" == *"DISABLED"* ]]; then
        echo "$user;$quota;$used;$exp;$proto;$id" >> "$tmp"
        continue
    fi
    
    exp_epoch=$(date -d "$exp" +%s 2>/dev/null || echo 0)
    
    # Cek Expired
    if [ "$now_epoch" -gt "$exp_epoch" ]; then
        # Hapus user dari config
        jq --arg u "$user" '(.inbounds = [.inbounds[] | if .settings.clients then .settings.clients |= map(select(.email != $u)) else . end])' "$CONFIG" > /tmp/cfg && mv /tmp/cfg "$CONFIG"
        need_restart=true
        
        # Tandai di DB
        echo "$user;$quota;$used;$exp;$proto-EXPIRED;$id" >> "$tmp"
    else
        echo "$user;$quota;$used;$exp;$proto;$id" >> "$tmp"
    fi
done < "$DB"

mv "$tmp" "$DB"

if [ "$need_restart" = true ]; then
    systemctl restart xray
fi
