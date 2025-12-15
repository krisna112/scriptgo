cat > /usr/bin/quota.sh << 'EOF'
#!/bin/bash
# Quota Enforcer v3.3.6 (STATS PERSISTENCE)
# Fix: Separated Upload/Download persistence & JQ logic

CONFIG="/usr/local/etc/xray/config.json"
DB="/etc/xray/clients.db"
STATS_DB="/etc/xray/stats.db"
LOCK="/var/run/xray-quota.lock"

# 1. Mechanism Locking
if [ -f "$LOCK" ]; then
    pid=$(cat "$LOCK")
    if kill -0 "$pid" > /dev/null 2>&1; then exit 0; fi
fi
echo $$ > "$LOCK"
trap "rm -f $LOCK" EXIT

# 2. Validation
[ ! -f "$DB" ] && exit 0
if ! command -v jq &> /dev/null; then exit 0; fi

# Load Stats Database into Associative Array
declare -A user_up
declare -A user_down

if [ -f "$STATS_DB" ]; then
    while IFS=';' read -r s_user s_up s_down; do
        user_up["$s_user"]=$s_up
        user_down["$s_user"]=$s_down
    done < "$STATS_DB"
fi

tmp=$(mktemp)
tmp_stats=$(mktemp)
need_restart=false
has_data=false
now_epoch=$(date +%s)

# 3. Main Loop
while IFS=';' read -r user quota used exp proto id; do
    if [ -z "$user" ] || [ -z "$id" ]; then continue; fi
    if ! [[ "$used" =~ ^[0-9]+$ ]]; then used=0; fi
    
    # Initialize stats if empty
    curr_acc_up=${user_up["$user"]:-0}
    curr_acc_down=${user_down["$user"]:-0}
    
    # Status Check
    is_disabled=0
    violation="NONE"
    
    if [[ "$proto" == *"DISABLED"* ]] || [[ "$proto" == *"EXPIRED"* ]]; then
        is_disabled=1
        if grep -q "$user" "$CONFIG"; then
            violation="FORCE_REMOVE"
        fi
    fi

    # -----------------------------------------------------------
    # A. FETCH DATA USAGE (With Reset)
    # -----------------------------------------------------------
    up=0; down=0
    if [ "$is_disabled" -eq 0 ]; then
        if systemctl is-active --quiet xray; then
            # Note: -reset flag resets the counter in Xray
            up=$(timeout 3s xray api stats -server=127.0.0.1:10085 -name "user>>>${user}>>>traffic>>>uplink" -reset 2>/dev/null | jq -r '.stat.value // 0')
            down=$(timeout 3s xray api stats -server=127.0.0.1:10085 -name "user>>>${user}>>>traffic>>>downlink" -reset 2>/dev/null | jq -r '.stat.value // 0')
            [[ ! "$up" =~ ^[0-9]+$ ]] && up=0
            [[ ! "$down" =~ ^[0-9]+$ ]] && down=0
        fi
    fi

    # Accumulate Data
    current_inc=$((up + down))
    new_total=$((used + current_inc))
    
    # Update Separate Stats
    new_acc_up=$((curr_acc_up + up))
    new_acc_down=$((curr_acc_down + down))
    
    # Write to temp stats file
    echo "$user;$new_acc_up;$new_acc_down" >> "$tmp_stats"
    
    # -----------------------------------------------------------
    # B. LOGIKA PENGECEKAN
    # -----------------------------------------------------------
    quota_bytes=$(awk "BEGIN {printf \"%.0f\", $quota * 1073741824}")
    exp_epoch=$(date -d "$exp" +%s 2>/dev/null || echo 0)
    
    if [ "$is_disabled" -eq 0 ]; then
        if [ "$now_epoch" -gt "$exp_epoch" ]; then
            violation="EXPIRED"
        elif [ "$quota_bytes" -gt 0 ] && [ "$new_total" -ge "$quota_bytes" ]; then
            violation="LIMIT"
        fi
    fi

    # -----------------------------------------------------------
    # C. EKSEKUSI (HAPUS AKSES)
    # -----------------------------------------------------------
    if [ "$violation" != "NONE" ]; then
        jq --arg u "$user" '(.inbounds = [.inbounds[] | if .settings.clients then .settings.clients |= map(select(.email != $u)) else . end])' "$CONFIG" > /tmp/cfg && mv /tmp/cfg "$CONFIG"
        
        need_restart=true
        
        if [[ "$proto" != *"DISABLED"* ]] && [[ "$proto" != *"EXPIRED"* ]]; then
            if [ "$violation" == "EXPIRED" ]; then
                echo "$user;$quota;$new_total;$exp;$proto-EXPIRED;$id" >> "$tmp"
            else
                echo "$user;$quota;$new_total;$exp;$proto-DISABLED;$id" >> "$tmp"
            fi
        else
            echo "$user;$quota;$new_total;$exp;$proto;$id" >> "$tmp"
        fi
    else
        echo "$user;$quota;$new_total;$exp;$proto;$id" >> "$tmp"
    fi
    
    has_data=true

done < "$DB"

# 4. Save Databases
if [ "$has_data" = true ] && [ -s "$tmp" ]; then
    cat "$tmp" > "$DB"
fi
if [ -s "$tmp_stats" ]; then
    cat "$tmp_stats" > "$STATS_DB"
fi

rm -f "$tmp" "$tmp_stats"

# 5. Restart Xray
if [ "$need_restart" = true ]; then
    systemctl restart xray
    echo "Xray restarted (Users removed)."
fi
EOF

chmod +x /usr/bin/quota.sh
