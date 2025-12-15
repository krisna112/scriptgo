#!/bin/bash
# Advanced Xray Panel v3.3.6 - UPDATE FIX & ANTI-DUPLICATE
# Features: 
# 1. Tampilan Terstruktur (Original Layout)
# 2. Total Usage Akumulatif (Anti-Reset)
# 3. Update System Fix (Real GitHub Fetch)
# 4. Bot Management
# 5. Anti-Duplicate & Web Status

CONFIG="/usr/local/etc/xray/config.json"
DB_CLIENTS="/etc/xray/clients.db"
DB_INBOUNDS="/etc/xray/inbounds.db"
VERSION_FILE="/etc/xray/version"
REPO="https://raw.githubusercontent.com/krisna112/scriptpanelvps/main"
DOMAIN=$(cat /root/domain 2>/dev/null || echo "Not Configured")
WEB_PORT=$(cat /root/web_port 2>/dev/null || echo "2053")

# Colors
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; CYAN='\033[0;36m'; PURPLE='\033[0;35m'
WHITE='\033[1;37m'; NC='\033[0m'

# Ensure dependencies
for cmd in jq qrencode bc awk netstat curl; do
    if ! command -v $cmd &> /dev/null; then apt-get install -y -qq $cmd net-tools 2>/dev/null; fi
done

# Initialize Version
if [ ! -f "$VERSION_FILE" ]; then echo "3.3.6" > "$VERSION_FILE"; fi

#=============================================================================
# 1. CORE HELPER FUNCTIONS
#=============================================================================

function show_header() {
    clear
    local_ver=$(cat "$VERSION_FILE" 2>/dev/null || echo "Unknown")
    echo -e "${PURPLE}╔════════════════════════════════════════════════════════╗${NC}"
    echo -e "${WHITE}║        ADVANCED XRAY PANEL v${local_ver}                        ║${NC}"
    echo -e "${PURPLE}╚════════════════════════════════════════════════════════╝${NC}"
    echo -e "  ${CYAN}🌐 Domain${NC}      : ${YELLOW}$DOMAIN${NC}"
    
    if systemctl is-active --quiet xray; then echo -e "  ${CYAN}🟢 Xray${NC}       : ${GREEN}RUNNING${NC}"; else echo -e "  ${CYAN}🔴 Xray${NC}       : ${RED}STOPPED${NC}"; fi
    
    # ADDED: Web Panel Status
    if systemctl is-active --quiet xray-web; then echo -e "  ${CYAN}🌍 Web${NC}        : ${GREEN}RUNNING (Port $WEB_PORT)${NC}"; else echo -e "  ${CYAN}🌍 Web${NC}        : ${RED}STOPPED${NC}"; fi
    
    if systemctl is-active --quiet telegram-bot; then echo -e "  ${CYAN}🤖 Bot${NC}        : ${GREEN}RUNNING${NC}"; else echo -e "  ${CYAN}🤖 Bot${NC}        : ${RED}STOPPED${NC}"; fi
    
    active_inbound=$(grep "^active" "$DB_INBOUNDS" 2>/dev/null | cut -d';' -f2)
    if [ -n "$active_inbound" ]; then echo -e "  ${CYAN}📡 Inbound${NC}    : ${GREEN}$active_inbound (Port 443)${NC}"; else echo -e "  ${CYAN}📡 Inbound${NC}    : ${RED}Not Configured${NC}"; fi
    
    total_users=$(wc -l < "$DB_CLIENTS" 2>/dev/null || echo "0"); ram_usage=$(free -m | awk 'NR==2{printf "%.1f%%", $3*100/$2}'); uptime_info=$(uptime -p | sed 's/up //')
    echo -e "  ${CYAN}👥 Users${NC}      : ${WHITE}$total_users${NC}"; echo -e "  ${CYAN}💾 RAM${NC}        : ${WHITE}$ram_usage${NC}"; echo -e "  ${CYAN}⏱️  Uptime${NC}     : ${WHITE}$uptime_info${NC}"
    echo -e "${PURPLE}════════════════════════════════════════════════════════${NC}"
}

function restart_xray() {
    echo -e "${YELLOW}♻️  Restarting Services...${NC}"; systemctl restart xray
    if systemctl list-unit-files | grep -q telegram-bot; then systemctl restart telegram-bot; fi
    sleep 2
    if systemctl is-active --quiet xray; then echo -e "${GREEN}✅ Services restarted!${NC}"; else echo -e "${RED}❌ Failed to restart Xray!${NC}"; fi
}

function format_bytes() {
    local bytes=$1
    if [[ -z "$bytes" ]] || [[ ! "$bytes" =~ ^[0-9]+$ ]]; then echo "0 B"; return; fi
    if [ "$bytes" -lt 1024 ]; then echo "${bytes} B"
    elif [ "$bytes" -lt 1048576 ]; then echo "$(awk "BEGIN {printf \"%.2f\", $bytes / 1024}") KB"
    elif [ "$bytes" -lt 1073741824 ]; then echo "$(awk "BEGIN {printf \"%.2f\", $bytes / 1048576}") MB"
    else echo "$(awk "BEGIN {printf \"%.2f\", $bytes / 1073741824}") GB"; fi
}

function get_user_traffic() {
    local user=$1
    # FIX: Menggunakan JQ dan Timeout 1s agar cepat
    local uplink=$(timeout 1s xray api stats --server=127.0.0.1:10085 -name "user>>>${user}>>>traffic>>>uplink" 2>/dev/null | jq -r '.stat.value // 0')
    local downlink=$(timeout 1s xray api stats --server=127.0.0.1:10085 -name "user>>>${user}>>>traffic>>>downlink" 2>/dev/null | jq -r '.stat.value // 0')
    [[ ! "$uplink" =~ ^[0-9]+$ ]] && uplink=0
    [[ ! "$downlink" =~ ^[0-9]+$ ]] && downlink=0
    echo "$uplink $downlink"
}

function get_user_status() {
    local user=$1
    local log="/var/log/xray/access.log"
    # Check last occurrence in logs (last 2000 lines for efficiency)
    local last_ts=$(tail -n 2000 "$log" | grep "email: ${user}" | tail -n 1 | awk '{print $1" "$2}')
    
    if [ -z "$last_ts" ]; then
        echo "OFFLINE"
        return
    fi
    
    # Convert to Epoch and compare
    local log_epoch=$(date -d "$last_ts" +%s 2>/dev/null)
    local now=$(date +%s)
    
    # 10 Seconds Threshold
    if [ -n "$log_epoch" ] && [ $((now - log_epoch)) -le 10 ]; then
        echo "ONLINE"
    else
        echo "OFFLINE"
    fi
}

function generate_link_bash() {
    local user=$1; local proto=$2; local domain=$3; local uuid=$4; local port="443"
    local p=$(echo "$proto" | cut -d'-' -f1); local t=$(echo "$proto" | cut -d'-' -f2)
    if [ "$p" == "VLESS" ]; then if [ "$t" == "XTLS" ]; then echo "vless://${uuid}@${domain}:${port}?security=tls&encryption=none&flow=xtls-rprx-vision&type=tcp&sni=${domain}&alpn=h2,http/1.1#${user}"; elif [ "$t" == "WS" ]; then echo "vless://${uuid}@${domain}:${port}?security=tls&encryption=none&type=ws&path=%2Fvless-ws&host=${domain}&sni=${domain}&alpn=h2,http/1.1#${user}"; else echo "vless://${uuid}@${domain}:${port}?security=tls&encryption=none&type=grpc&serviceName=vless-grpc&mode=multi&sni=${domain}&alpn=h2#${user}"; fi
    elif [ "$p" == "VMESS" ]; then if [ "$t" == "WS" ]; then json="{\"v\":\"2\",\"ps\":\"${user}\",\"add\":\"${domain}\",\"port\":\"${port}\",\"id\":\"${uuid}\",\"aid\":\"0\",\"scy\":\"auto\",\"net\":\"ws\",\"type\":\"none\",\"host\":\"${domain}\",\"path\":\"/vmess-ws\",\"tls\":\"tls\",\"sni\":\"${domain}\",\"alpn\":\"h2,http/1.1\"}"; else json="{\"v\":\"2\",\"ps\":\"${user}\",\"add\":\"${domain}\",\"port\":\"${port}\",\"id\":\"${uuid}\",\"aid\":\"0\",\"scy\":\"auto\",\"net\":\"grpc\",\"type\":\"none\",\"host\":\"${domain}\",\"path\":\"vmess-grpc\",\"tls\":\"tls\",\"sni\":\"${domain}\",\"alpn\":\"h2\"}"; fi; echo "vmess://$(echo -n "$json" | base64 -w 0)"
    elif [ "$p" == "TROJAN" ]; then if [ "$t" == "WS" ]; then echo "trojan://${uuid}@${domain}:${port}?security=tls&type=ws&path=%2Ftrojan-ws&host=${domain}&sni=${domain}&alpn=h2,http/1.1#${user}"; else echo "trojan://${uuid}@${domain}:${port}?security=tls&type=grpc&serviceName=trojan-grpc&mode=multi&sni=${domain}&alpn=h2#${user}"; fi; fi
}

function show_user_config_details() {
    local user=$1; local user_entry=$(grep "^$user;" "$DB_CLIENTS"); IFS=';' read -r u q used exp proto uuid <<< "$user_entry"
    local link=$(generate_link_bash "$user" "$proto" "$DOMAIN" "$uuid")
    clear; echo -e "${CYAN}╔════════════════════════════════════════════════════════╗${NC}"; echo -e "${CYAN}║                USER CONFIGURATION DETAILS              ║${NC}"; echo -e "${CYAN}╚════════════════════════════════════════════════════════╝${NC}"; echo -e "  👤 User      : ${GREEN}$user${NC}"; echo -e "  🔐 UUID/Pass: ${YELLOW}$uuid${NC}"; echo -e "  📡 Protocol : ${PURPLE}$proto${NC}"; echo -e "  ⏳ Expired   : ${RED}$exp${NC}"; echo -e "\n${CYAN}🔗 ORIGINAL URI:${NC}\n${WHITE}$link${NC}\n\n${CYAN}📱 QR CODE:${NC}"; qrencode -t ANSIUTF8 "$link"; echo ""; read -p "Press Enter to go back..."
}

#=============================================================================
# 2. MENU ACTION FUNCTIONS (ADD/DEL/EDIT)
#=============================================================================

function add_inbound() {
    show_header
    echo -e "${BLUE}╔════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║           ADD INBOUND CONFIGURATION                    ║${NC}"
    echo -e "${BLUE}╚════════════════════════════════════════════════════════╝${NC}"
    if [ -s "$DB_INBOUNDS" ]; then echo -e "${YELLOW}⚠️  Inbound already configured!${NC}"; read -p "Press Enter..."; return; fi
    
    echo -e "${CYAN}Choose Protocol for Port 443:${NC}"
    echo -e "  [1] VLESS ${GREEN}(Recommended)${NC}"
    echo -e "  [2] VMESS ${YELLOW}(Compatible)${NC}"
    echo -e "  [3] TROJAN ${YELLOW}(Stealth)${NC}"
    read -p "▶️  Choose [1-3]: " proto_choice
    case $proto_choice in 1) protocol="VLESS" ;; 2) protocol="VMESS" ;; 3) protocol="TROJAN" ;; *) echo -e "${RED}Invalid!${NC}"; sleep 2; return ;; esac
    
    echo -e "${CYAN}Choose Transport:${NC}"
    if [ "$protocol" == "VLESS" ]; then echo -e "  [1] TCP/XTLS ${GREEN}(Fastest)${NC}"; fi
    echo -e "  [2] WebSocket ${YELLOW}(Universal)${NC}"
    echo -e "  [3] gRPC ${YELLOW}(Stealth)${NC}"
    read -p "▶️  Choose: " trans_choice
    case $trans_choice in 1) if [ "$protocol" != "VLESS" ]; then echo -e "${RED}XTLS only for VLESS!${NC}"; sleep 2; return; fi; transport="XTLS" ;; 2) transport="WS" ;; 3) transport="gRPC" ;; *) echo -e "${RED}Invalid!${NC}"; sleep 2; return ;; esac
    
    tmp=$(mktemp); tag="${protocol,,}-${transport,,}"
    
    # Logic JQ untuk menambah inbound sesuai pilihan
    # OPTIMIZED: Added fakedns and quic to sniffing destOverride for better gaming/UDP support
    if [ "$protocol" == "VLESS" ] && [ "$transport" == "XTLS" ]; then
        jq --arg tag "$tag" '.inbounds += [{"tag": $tag, "port": 443, "protocol": "vless", "settings": {"clients": [], "decryption": "none", "fallbacks": [{"dest": 8080, "xver": 1}]}, "streamSettings": {"network": "tcp", "security": "tls", "tlsSettings": {"alpn": ["h2", "http/1.1"], "certificates": [{"certificateFile": "/etc/xray/xray.crt", "keyFile": "/etc/xray/xray.key"}]}}, "sniffing": {"enabled": true, "destOverride": ["http", "tls", "quic", "fakedns"]}}]' "$CONFIG" > "$tmp" && mv "$tmp" "$CONFIG"
    elif [ "$transport" == "WS" ]; then
        path="/${protocol,,}-ws"
        jq --arg tag "$tag" --arg proto "${protocol,,}" --arg path "$path" '.inbounds += [{"tag": $tag, "port": 443, "protocol": $proto, "settings": {"clients": [], "decryption": "none"}, "streamSettings": {"network": "ws", "security": "tls", "tlsSettings": {"alpn": ["h2", "http/1.1"], "certificates": [{"certificateFile": "/etc/xray/xray.crt", "keyFile": "/etc/xray/xray.key"}]}, "wsSettings": {"path": $path}}, "sniffing": {"enabled": true, "destOverride": ["http", "tls", "quic", "fakedns"]}}]' "$CONFIG" > "$tmp" && mv "$tmp" "$CONFIG"
    elif [ "$transport" == "gRPC" ]; then
        service="${protocol,,}-grpc"
        jq --arg tag "$tag" --arg proto "${protocol,,}" --arg service "$service" '.inbounds += [{"tag": $tag, "port": 443, "protocol": $proto, "settings": {"clients": [], "decryption": "none"}, "streamSettings": {"network": "grpc", "security": "tls", "tlsSettings": {"alpn": ["h2"], "certificates": [{"certificateFile": "/etc/xray/xray.crt", "keyFile": "/etc/xray/xray.key"}]}, "grpcSettings": {"serviceName": $service, "multiMode": true}}}]' "$CONFIG" > "$tmp" && mv "$tmp" "$CONFIG"
    fi
    
    echo "active;$protocol-$transport;443" > "$DB_INBOUNDS"
    restart_xray
    echo -e "${GREEN}✅ Inbound added!${NC}"
    read -p "Press Enter..."
}

function delete_inbound_silent() {
    tmp=$(mktemp)
    jq '.inbounds = [.inbounds[] | select(.tag == "api")]' "$CONFIG" > "$tmp" && mv "$tmp" "$CONFIG"
    > "$DB_INBOUNDS"
    > "$DB_CLIENTS"
}

function delete_inbound() {
    show_header
    echo -e "${RED}╔════════════════════════════════════════════════════════╗${NC}"
    echo -e "${RED}║           DELETE INBOUND                               ║${NC}"
    echo -e "${RED}╚════════════════════════════════════════════════════════╝${NC}"
    if [ ! -s "$DB_INBOUNDS" ]; then echo -e "${YELLOW}No inbound!${NC}"; read -p "Press Enter..."; return; fi
    read -p "Delete Inbound? (y/n): " confirm
    [ "$confirm" != "y" ] && return
    delete_inbound_silent
    restart_xray
    echo -e "${GREEN}✅ Deleted!${NC}"
    read -p "Press Enter..."
}

function edit_inbound() {
    show_header
    echo -e "${BLUE}╔════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║           EDIT INBOUND                                 ║${NC}"
    echo -e "${BLUE}╚════════════════════════════════════════════════════════╝${NC}"
    if [ ! -s "$DB_INBOUNDS" ]; then echo -e "${YELLOW}No inbound configured!${NC}"; read -p "Press Enter..."; return; fi
    current=$(grep "^active" "$DB_INBOUNDS" | cut -d';' -f2)
    echo -e "${CYAN}Current: ${WHITE}$current${NC}"
    echo -e "${YELLOW}⚠️  Will delete all users!${NC}"
    read -p "Continue? (y/n): " confirm
    [ "$confirm" != "y" ] && return
    delete_inbound_silent
    add_inbound
}

function create_user() {
    show_header
    if [ ! -s "$DB_INBOUNDS" ]; then echo -e "${RED}⚠️  No inbound! Add inbound first.${NC}"; read -p "Press Enter..."; return; fi
    active_inbound=$(grep "^active" "$DB_INBOUNDS" | cut -d';' -f2)
    protocol=$(echo "$active_inbound" | cut -d'-' -f1)
    transport=$(echo "$active_inbound" | cut -d'-' -f2)
    
    echo -e "${BLUE}╔════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║           CREATE USER - $active_inbound                ${NC}"
    echo -e "${BLUE}╚════════════════════════════════════════════════════════╝${NC}"
    
    read -p "📝 Username     : " user
    [ -z "$user" ] && { echo -e "${RED}Empty!${NC}"; sleep 1; return; }
    
    # ADDED: Anti-Duplicate Username
    if grep -q "^$user;" "$DB_CLIENTS"; then echo -e "${RED}❌ Username already exists!${NC}"; sleep 2; return; fi
    
    read -p "⏰ Days         : " exp_days
    read -p "💾 Quota (GB)   : " quota
    
    exp_date=$(date -d "+${exp_days} days" +"%Y-%m-%d %H:%M:%S")
    tmp=$(mktemp)
    tag="${protocol,,}-${transport,,}"
    
    # UUID GENERATION & CHECK
    if [ "$protocol" == "VLESS" ] || [ "$protocol" == "VMESS" ]; then
        uuid=$(cat /proc/sys/kernel/random/uuid)
        id_save=$uuid
    else
        read -p "🔑 Password (Enter=auto): " manual_pass
        password=${manual_pass:-$(openssl rand -hex 16)}
        id_save=$password
    fi
    
    # ADDED: Anti-Duplicate UUID/Password
    if grep -q "$id_save" "$DB_CLIENTS"; then
        echo -e "${RED}❌ UUID/Password already exists! Please try again.${NC}"
        sleep 2; return
    fi
    
    if [ "$protocol" == "VLESS" ] || [ "$protocol" == "VMESS" ]; then
        jq --arg tag "$tag" --arg u "$user" --arg id "$uuid" '(.inbounds[] | select(.tag==$tag).settings.clients) += [{"id":$id,"email":$u,"level":0}]' "$CONFIG" > "$tmp" && mv "$tmp" "$CONFIG"
        
        if [ "$protocol" == "VLESS" ] && [ "$transport" == "XTLS" ]; then
            jq --arg tag "$tag" --arg u "$user" '(.inbounds[] | select(.tag==$tag).settings.clients[] | select(.email==$u)).flow = "xtls-rprx-vision"' "$CONFIG" > "$tmp" && mv "$tmp" "$CONFIG"
        fi
    else
        jq --arg tag "$tag" --arg u "$user" --arg pass "$password" '(.inbounds[] | select(.tag==$tag).settings.clients) += [{"password":$pass,"email":$u,"level":0}]' "$CONFIG" > "$tmp" && mv "$tmp" "$CONFIG"
    fi
    
    echo "${user};${quota};0;${exp_date};${active_inbound};${id_save}" >> "$DB_CLIENTS"
    restart_xray
    echo -e "${GREEN}✅ User created!${NC}"
    show_user_config_details "$user"
}

function delete_user() {
    show_header
    echo -e "${RED}╔════════════════════════════════════════════════════════╗${NC}"
    echo -e "${RED}║           DELETE USER                                  ║${NC}"
    echo -e "${RED}╚════════════════════════════════════════════════════════╝${NC}"
    i=1
    declare -a users
    while IFS=';' read -r user q u e p id; do
        [ -z "$user" ] && continue
        users[i]="$user"
        echo -e "  [$i] $user"
        ((i++))
    done < "$DB_CLIENTS"
    
    [ $i -eq 1 ] && { echo -e "${YELLOW}No users${NC}"; sleep 2; return; }
    
    echo ""
    read -p "▶️  Number: " num
    user="${users[$num]}"
    [ -z "$user" ] && return
    
    read -p "⚠️  Delete '$user'? (y/n): " confirm
    [ "$confirm" != "y" ] && return
    
    tmp=$(mktemp)
    jq --arg u "$user" '(.inbounds = [.inbounds[] | if .settings.clients then .settings.clients |= map(select(.email != $u)) else . end])' "$CONFIG" > "$tmp" && mv "$tmp" "$CONFIG"
    grep -v "^$user;" "$DB_CLIENTS" > /tmp/db.tmp && mv /tmp/db.tmp "$DB_CLIENTS"
    
    restart_xray
    echo -e "${GREEN}✅ Deleted!${NC}"
    sleep 2
}

#=============================================================================
# 3. STATS & MONITOR FUNCTIONS (ANTI-RESET)
#=============================================================================

function list_users() {
    while true; do
        show_header
        echo -e "${YELLOW}╔═══════════════════════════════════════════════════════════════════════════════╗${NC}"
        echo -e "${YELLOW}║                            USER MANAGEMENT LIST                               ║${NC}"
        echo -e "${YELLOW}╚═══════════════════════════════════════════════════════════════════════════════╝${NC}"
        
        echo -e "${CYAN}📡 Fetching accumulated data...${NC}"
        echo ""
        printf "%-4s %-15s %-10s %-12s %-10s %-12s\n" "No" "User" "Quota" "Total Used" "Info" "Status"
        echo "-------------------------------------------------------------------------------"
        i=1; total_usage_bytes=0; online_count=0; declare -a users_array
        
        # Load Stats Database
        STATS_DB="/etc/xray/stats.db"
        declare -A hist_up
        declare -A hist_down
        if [ -f "$STATS_DB" ]; then
            while IFS=';' read -r s_user s_up s_down; do
                hist_up["$s_user"]=$s_up
                hist_down["$s_user"]=$s_down
            done < "$STATS_DB"
        fi

        
        while IFS=';' read -r user quota used_db exp proto id; do
            [ -z "$user" ] && continue
            users_array[$i]="$user"
            if ! [[ "$used_db" =~ ^[0-9]+$ ]]; then used_db=0; fi

            traffic=$(get_user_traffic "$user")
            up=$(echo "$traffic" | cut -d' ' -f1); down=$(echo "$traffic" | cut -d' ' -f2)
            
            # Historical Stats
            h_up=${hist_up["$user"]:-0}
            h_down=${hist_down["$user"]:-0}
            
            # TOTAL = HISTORICAL + SESSION (ignoring DB used field to avoid double counting if confused, sticking to stats.db as source of truth)
            # Actually, typically DB_CLIENTS has the 'last saved' total. 
            # If we use stats.db, that is safer as it separates up/down.
            # Logic: Total = h_up + h_down + current_session_up + current_session_down
            
            total_acc=$((h_up + h_down + up + down))

            used_hr=$(format_bytes "$total_acc")
            
            status_check=$(get_user_status "$user")
            if [ "$status_check" == "ONLINE" ]; then online_status="${GREEN}● ON${NC}"; ((online_count++)); else online_status="${RED}○ OFF${NC}"; fi
            
            exp_ts=$(date -d "$exp" +%s 2>/dev/null || echo 0)
            now=$(date +%s)
            # CEILING ROUNDING: (diff + 86399) / 86400
            # If diff=1s -> (1+86399)/86400 = 1 day. If diff=0 -> 0.
            days=$(( (exp_ts - now + 86399) / 86400 ))
            quota_bytes=$(awk "BEGIN {printf \"%.0f\", $quota * 1073741824}")
            # CEILING ROUNDING
            days_left=$(( (exp_ts - now + 86399) / 86400 ))
            
            if [ "$now" -gt "$exp_ts" ]; then info_text="EXPIRED"
            elif [ "$quota_bytes" -gt 0 ] && [ "$total_acc" -ge "$quota_bytes" ]; then info_text="LIMIT"
            else info_text="${days_left}d"; fi
            
            printf "%-4s %-15s %-10s %-12s %-10s %-12b\n" "$i" "$user" "${quota}GB" "$used_hr" "$info_text" "$online_status"
            ((i++))
        done < "$DB_CLIENTS"
        
        echo "-------------------------------------------------------------------------------"
        echo -e "${CYAN}[number] Details | [r] Refresh | [Enter] Back${NC}"
        read -p "Choose: " opt
        if [ -z "$opt" ]; then return; elif [[ "$opt" =~ ^[0-9]+$ ]] && [ "$opt" -lt "$i" ] && [ "$opt" -gt 0 ]; then show_user_config_details "${users_array[$opt]}"; elif [ "$opt" == "r" ] || [ "$opt" == "R" ]; then list_users 1; fi
    done
}

function realtime_stats() {
    local auto=${1:-0}; local refresh_count=${2:-0}
    show_header
    echo -e "${BLUE}╔═══════════════════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║           REAL-TIME & ACCUMULATED STATISTICS                                  ║${NC}"
    echo -e "${BLUE}╚═══════════════════════════════════════════════════════════════════════════════╝${NC}"
    [ ! -s "$DB_CLIENTS" ] && { echo -e "${YELLOW}No users${NC}"; read -p "Enter..."; return; }
    
    echo -e "${CYAN}🔄 Fetching data...${NC}"; echo ""
    printf "%-12s %-12s %-12s %-14s %-8s %-10s\n" "User" "Up (Live)" "Down (Live)" "TOTAL (Acc)" "Use%" "Status"
    echo "--------------------------------------------------------------------------"
    
    # Load Stats Database
    STATS_DB="/etc/xray/stats.db"
    declare -A hist_up
    declare -A hist_down
    if [ -f "$STATS_DB" ]; then
        while IFS=';' read -r s_user s_up s_down; do
            hist_up["$s_user"]=$s_up
            hist_down["$s_user"]=$s_down
        done < "$STATS_DB"
    fi

    
    total_acc_all=0
    while IFS=';' read -r user quota used_db exp proto id; do
        [ -z "$user" ] && continue
        if ! [[ "$used_db" =~ ^[0-9]+$ ]]; then used_db=0; fi

        traffic=$(get_user_traffic "$user"); up=$(echo "$traffic" | cut -d' ' -f1); down=$(echo "$traffic" | cut -d' ' -f2)
        
        h_up=${hist_up["$user"]:-0}
        h_down=${hist_down["$user"]:-0}
        
        final_up=$((h_up + up))
        final_down=$((h_down + down))
        
        total_acc=$((final_up + final_down))
        total_acc_all=$((total_acc_all + total_acc))
        
        up_hr=$(format_bytes "$final_up"); down_hr=$(format_bytes "$final_down"); total_hr=$(format_bytes "$total_acc")
        
        quota_bytes=$(awk "BEGIN {printf \"%.0f\", $quota * 1073741824}")
        if [ "$quota_bytes" -gt 0 ]; then pct=$(awk "BEGIN {printf \"%.1f\", ($total_acc * 100) / $quota_bytes}"); else pct="0.0"; fi
        
        # Check Status using log-based logic
        chk_status=$(get_user_status "$user")
        if [ "$chk_status" == "ONLINE" ]; then status="${GREEN}ON${NC}"; else status="${RED}OFF${NC}"; fi
        
        printf "%-12s %-12s %-12s %-14s %-8s %-10b\n" "${user:0:10}" "$up_hr" "$down_hr" "$total_hr" "${pct}%" "$status"
    done < "$DB_CLIENTS"
    
    echo "--------------------------------------------------------------------------"
    echo -e "${CYAN}TOTAL TRAFFIC : ${WHITE}$(format_bytes "$total_acc_all")${NC}"
    echo ""
    if [ "$auto" -eq 1 ]; then echo -e "${GREEN}⟳ Auto-refresh... (Ctrl+C to stop)${NC}"; sleep 3; realtime_stats 1 $((refresh_count + 1)); return; fi
    echo -e "${WHITE}[r] Refresh  [a] Auto-refresh  [Enter] Back${NC}"; read -p "Choose: " opt
    case "$opt" in r|R) realtime_stats 0 $((refresh_count + 1)) ;; a|A) realtime_stats 1 0 ;; esac
}

function system_status() {
    show_header
    echo -e "${BLUE}╔════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║           SYSTEM STATUS                                ║${NC}"
    echo -e "${BLUE}╚════════════════════════════════════════════════════════╝${NC}"
    echo -e "${CYAN}🖥️  System:${NC}"
    echo -e "  • Hostname : $(hostname)"
    echo -e "  • Uptime   : $(uptime -p)"
    echo ""
    echo -e "${CYAN}💾 Resources:${NC}"
    free -h | awk 'NR==2{printf "  • RAM  : %s / %s (%.1f%%)\n", $3, $2, $3*100/$2}'
    df -h / | awk 'NR==2{printf "  • Disk : %s / %s (%s)\n", $3, $2, $5}'
    echo ""
    read -p "Press Enter..."
}

function test_api_connection() {
    show_header
    echo -e "${CYAN}Testing Xray API...${NC}"
    if timeout 1 xray api statsquery --server=127.0.0.1:10085 &>/dev/null; then
        echo -e "${GREEN}✅ API Connection Successful!${NC}"
    else
        echo -e "${RED}❌ API Connection FAILED!${NC}"
        echo -e "   Check: systemctl status xray"
    fi
    read -p "Press Enter..."
}

#=============================================================================
# 4. UPDATE & BOT FUNCTIONS (FIXED)
#=============================================================================

function enable_bot() {
    echo -e "${YELLOW}🤖 Starting Bot...${NC}"; systemctl enable --now telegram-bot; sleep 2; 
    if systemctl is-active --quiet telegram-bot; then echo -e "${GREEN}✅ Success!${NC}"; else echo -e "${RED}❌ Failed!${NC}"; fi; read -p "Enter..."
}
function disable_bot() {
    echo -e "${YELLOW}🤖 Stopping Bot...${NC}"; systemctl disable --now telegram-bot; sleep 2;
    if ! systemctl is-active --quiet telegram-bot; then echo -e "${GREEN}✅ Success!${NC}"; else echo -e "${RED}❌ Failed!${NC}"; fi; read -p "Enter..."
}
function update_xray_core() {
    show_header; echo -e "${CYAN}Updating Xray...${NC}"; bash -c "$(curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ install; restart_xray; read -p "Enter..."
}

function enable_bbr() {
    show_header
    echo -e "${BLUE}╔════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║           ENABLE BBR (SPEED UP)                        ║${NC}"
    echo -e "${BLUE}╚════════════════════════════════════════════════════════╝${NC}"
    echo -e "${YELLOW}Enabling TCP BBR Congestion Control...${NC}"
    
    cat > /etc/sysctl.conf <<'EOF'
# Optimized by ScriptPanelVPS
net.core.default_qdisc=fq
net.ipv4.tcp_congestion_control=bbr
EOF
    sysctl -p
    echo ""
    echo -e "${GREEN}✅ BBR Enabled! Verification:${NC}"
    sysctl net.ipv4.tcp_congestion_control
    echo ""
    read -p "Press Enter..."
}

function debug_service() {
    clear
    echo -e "${RED}╔════════════════════════════════════════════════════════╗${NC}"
    echo -e "${WHITE}║             🐞 SYSTEM DIAGNOSTIC TOOL                  ║${NC}"
    echo -e "${RED}╚════════════════════════════════════════════════════════╝${NC}"
    echo -e "Select service to troubleshoot:"
    echo -e " [1] 🟢 Xray Core (VPN) - Cek Config & Log"
    echo -e " [2] 🌍 Web Panel - Cek Error 500/502"
    echo -e " [3] 🤖 Telegram Bot - Cek Koneksi Bot"
    echo -e " [x] Back"
    echo ""
    read -p "Choose: " d_opt

    case $d_opt in
        1)
            echo -e "\n${YELLOW}--- 1. Checking Config JSON Syntax ---${NC}"
            if /usr/local/bin/xray run -test -config /usr/local/etc/xray/config.json; then
                echo -e "${GREEN}✅ Config Valid!${NC}"
            else
                echo -e "${RED}❌ Config Error! (Lihat baris error di atas)${NC}"
            fi
            echo -e "\n${YELLOW}--- 2. Service Status ---${NC}"
            systemctl status xray --no-pager -n 5
            echo -e "\n${YELLOW}--- 3. Recent Error Logs ---${NC}"
            tail -n 10 /var/log/xray/error.log
            ;;
        2)
            echo -e "\n${YELLOW}--- 1. Checking Python Script ---${NC}"
            cd /opt/xray-web-panel
            # Dry run python script to catch syntax error
            if ./venv/bin/python3 -c "import app" 2>/dev/null; then
                 echo -e "${GREEN}✅ Python Syntax OK!${NC}"
            else
                 echo -e "${RED}❌ Python Syntax Error!${NC}"
                 ./venv/bin/python3 -m py_compile app.py
            fi
            echo -e "\n${YELLOW}--- 2. Service Log (Last 15 lines) ---${NC}"
            journalctl -u xray-web -n 15 --no-pager
            ;;
        3)
            echo -e "\n${YELLOW}--- Bot Service Log ---${NC}"
            if [ -f "/etc/xray/telegram-bot.conf" ]; then
                systemctl status telegram-bot --no-pager -n 10
                journalctl -u telegram-bot -n 10 --no-pager
            else
                echo -e "${RED}Bot belum dikonfigurasi!${NC}"
            fi
            ;;
        x) return ;;
    esac
    echo ""
    read -p "Press Enter to return..."
    main_menu
}

function check_update() {
    show_header
    echo -e "${BLUE}╔════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║           CHECK FOR UPDATES                            ║${NC}"
    echo -e "${BLUE}╚════════════════════════════════════════════════════════╝${NC}"
    echo -e "${CYAN}Checking version from GitHub...${NC}"
    
    # FIXED: Anti-Cache Query using date
    local_ver=$(cat "$VERSION_FILE" 2>/dev/null | tr -d ' \r\n' || echo "Unknown")
    remote_ver=$(curl -s "${REPO}/version?t=$(date +%s)" | tr -d ' \r\n')
    
    echo -e "  📄 Current : ${WHITE}$local_ver${NC}"
    echo -e "  🆕 Latest  : ${GREEN}$remote_ver${NC}"
    echo ""
    
    if [ "$local_ver" == "$remote_ver" ]; then
        echo -e "${GREEN}✅ You are up to date!${NC}"
        read -p "Force update anyway? (y/n): " force
        [ "$force" == "y" ] && force_update
    else
        echo -e "${YELLOW}⚡ Update available!${NC}"
        read -p "▶️  Update now? (y/n): " confirm
        [ "$confirm" == "y" ] && force_update
    fi
}

function force_update() {
    echo -e "${YELLOW}🚀 Starting Update...${NC}"
    # Download updater to /tmp to avoid conflict
    wget -q -O /tmp/update-vps.sh "${REPO}/update-vps.sh?t=$(date +%s)"
    if [ -s "/tmp/update-vps.sh" ]; then
        chmod +x /tmp/update-vps.sh
        bash /tmp/update-vps.sh
    else
        echo -e "${RED}❌ Download failed! Check internet.${NC}"
        read -p "Press Enter..."
    fi
    exit 0
}

#=============================================================================
# MAIN MENU LOOP
#=============================================================================

function main_menu() {
    while true; do
        show_header
        echo ""
        echo -e "${WHITE}╔════════════ INBOUND ═══════════════════════╗${NC}"
        echo -e "${WHITE}║${NC} [1] 📡 Add Inbound                          ${WHITE}║${NC}"
        echo -e "${WHITE}║${NC} [2] ✏️  Edit Inbound                         ${WHITE}║${NC}"
        echo -e "${WHITE}║${NC} [3] 🗑️  Delete Inbound                       ${WHITE}║${NC}"
        echo -e "${WHITE}╚════════════════════════════════════════════╝${NC}"
        echo ""
        echo -e "${WHITE}╔════════════ USER ══════════════════════════╗${NC}"
        echo -e "${WHITE}║${NC} [4] 🆕 Create User                          ${WHITE}║${NC}"
        echo -e "${WHITE}║${NC} [5] 👥 List Users (Details & QR)            ${WHITE}║${NC}"
        echo -e "${WHITE}║${NC} [6] 🗑️  Delete User                         ${WHITE}║${NC}"
        echo -e "${WHITE}╚════════════════════════════════════════════╝${NC}"
        echo ""
        echo -e "${WHITE}╔════════════ MONITOR ═══════════════════════╗${NC}"
        echo -e "${WHITE}║${NC} [7] 📊 System Status                        ${WHITE}║${NC}"
        echo -e "${WHITE}║${NC} [8] 📈 Real-time Stats                      ${WHITE}║${NC}"
        echo -e "${WHITE}║${NC} [9] ♻️  Restart Xray                        ${WHITE}║${NC}"
        echo -e "${WHITE}║${NC} [10] 🔧 Test API                            ${WHITE}║${NC}"
        echo -e "${WHITE}╚════════════════════════════════════════════╝${NC}"
        echo ""
        echo -e "${WHITE}╔════════════ SYSTEM & TOOLS ════════════════╗${NC}"
        echo -e "${WHITE}║${NC} [11] 🔄 Check Updates                       ${WHITE}║${NC}"
        echo -e "${WHITE}║${NC} [12] 🔧 Force Update                        ${WHITE}║${NC}"
        echo -e "${WHITE}║${NC} [13] ▶️  Bot Start/Enable                    ${WHITE}║${NC}"
        echo -e "${WHITE}║${NC} [14] ⏹️  Bot Stop/Disable                    ${WHITE}║${NC}"
        echo -e "${WHITE}║${NC} [15] ⬆️  Update Xray Core                    ${WHITE}║${NC}"
        echo -e "${WHITE}║${NC} [16] 🐞 Debug/Troubleshoot                ${WHITE}║${NC}"
        echo -e "${WHITE}║${NC} [17] 🚀 Enable BBR (Jumper)               ${WHITE}║${NC}"
        echo -e "${WHITE}╚════════════════════════════════════════════╝${NC}"
        echo ""
        echo -e "${WHITE}[x]  🚪 Exit${NC}"
        
        read -p "▶️  Choose: " opt
        
        case $opt in
            1) add_inbound ;; 2) edit_inbound ;; 3) delete_inbound ;;
            4) create_user ;; 5) list_users ;; 6) delete_user ;;
            7) system_status ;; 8) realtime_stats 0 0 ;; 9) restart_xray; read -p "Press Enter..." ;;
            10) test_api_connection ;; 11) check_update ;; 12) force_update ;;
            13) enable_bot ;; 14) disable_bot ;; 15) update_xray_core ;;
            16) debug_service ;; 17) enable_bbr ;;
            x|X) clear; echo -e "${GREEN}👋 Bye!${NC}"; exit 0 ;;
        esac
    done
}

if ! command -v xray &> /dev/null; then echo "Xray not installed"; exit 1; fi
main_menu
