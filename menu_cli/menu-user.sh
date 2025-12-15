#!/bin/bash
# User Management Module

function format_bytes() {
    local bytes=$1
    
    if ! [[ "$bytes" =~ ^[0-9]+$ ]]; then
        echo "0 B"
        return
    fi
    
    if [ "$bytes" -eq 0 ]; then
        echo "0 B"
    elif [ "$bytes" -lt 1024 ]; then
        echo "${bytes} B"
    elif [ "$bytes" -lt 1048576 ]; then
        echo "$(awk "BEGIN {printf \"%.2f\", $bytes / 1024}") KB"
    elif [ "$bytes" -lt 1073741824 ]; then
        echo "$(awk "BEGIN {printf \"%.2f\", $bytes / 1048576}") MB"
    elif [ "$bytes" -lt 1099511627776 ]; then
        echo "$(awk "BEGIN {printf \"%.2f\", $bytes / 1073741824}") GB"
    else
        echo "$(awk "BEGIN {printf \"%.2f\", $bytes / 1099511627776}") TB"
    fi
}

function create_user() {
    show_header
    
    if [ ! -s "$DB_INBOUNDS" ]; then
        echo -e "${RED}⚠️  No inbound configured!${NC}"
        echo -e "${YELLOW}Add inbound first (menu [1])${NC}"
        read -p "Press Enter..."
        return
    fi
    
    active_inbound=$(grep "^active" "$DB_INBOUNDS" | cut -d';' -f2)
    protocol=$(echo "$active_inbound" | cut -d'-' -f1)
    transport=$(echo "$active_inbound" | cut -d'-' -f2)
    
    echo -e "${BLUE}╔════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║          CREATE USER - $active_inbound               ${NC}"
    echo -e "${BLUE}╚════════════════════════════════════════════════════════╝${NC}"
    
    read -p "📝 Username    : " user
    [ -z "$user" ] && { echo -e "${RED}Empty!${NC}"; sleep 2; return; }
    
    grep -q "^$user;" "$DB_CLIENTS" && { echo -e "${RED}Exists!${NC}"; sleep 2; return; }
    
    read -p "⏰ Days        : " exp_days
    read -p "💾 Quota (GB)  : " quota
    read -p "🔒 SNI (Enter=$DOMAIN): " custom_sni
    [ -z "$custom_sni" ] && custom_sni="$DOMAIN"
    
    exp_date=$(date -d "+${exp_days} days" +"%Y-%m-%d %H:%M:%S")
    tmp=$(mktemp)
    tag="${protocol,,}-${transport,,}"
    
    if [ "$protocol" == "VLESS" ]; then
        uuid=$(cat /proc/sys/kernel/random/uuid)
        
        jq --arg tag "$tag" --arg u "$user" --arg id "$uuid" \
            '(.inbounds[] | select(.tag==$tag).settings.clients) += [{"id":$id,"email":$u,"level":0}]' \
            "$CONFIG" > "$tmp" && mv "$tmp" "$CONFIG"
        
        if [ "$transport" == "XTLS" ]; then
            jq --arg tag "$tag" --arg u "$user" \
                '(.inbounds[] | select(.tag==$tag).settings.clients[] | select(.email==$u)).flow = "xtls-rprx-vision"' \
                "$CONFIG" > "$tmp" && mv "$tmp" "$CONFIG"
            link="vless://${uuid}@${DOMAIN}:443?security=tls&encryption=none&flow=xtls-rprx-vision&type=tcp&sni=${custom_sni}&alpn=h2,http/1.1&allowInsecure=1#${user}"
        elif [ "$transport" == "WS" ]; then
            link="vless://${uuid}@${DOMAIN}:443?security=tls&encryption=none&type=ws&path=%2Fvless-ws&host=${DOMAIN}&sni=${custom_sni}&alpn=h2,http/1.1&allowInsecure=1#${user}"
        else
            link="vless://${uuid}@${DOMAIN}:443?security=tls&encryption=none&type=grpc&serviceName=vless-grpc&mode=multi&sni=${custom_sni}&alpn=h2&allowInsecure=1#${user}"
        fi
        
        echo "${user};${quota};0;${exp_date};${active_inbound};${uuid}" >> "$DB_CLIENTS"
        
    elif [ "$protocol" == "VMESS" ]; then
        uuid=$(cat /proc/sys/kernel/random/uuid)
        
        jq --arg tag "$tag" --arg u "$user" --arg id "$uuid" \
            '(.inbounds[] | select(.tag==$tag).settings.clients) += [{"id":$id,"email":$u,"level":0}]' \
            "$CONFIG" > "$tmp" && mv "$tmp" "$CONFIG"
        
        if [ "$transport" == "WS" ]; then
            json="{\"v\":\"2\",\"ps\":\"${user}\",\"add\":\"${DOMAIN}\",\"port\":\"443\",\"id\":\"${uuid}\",\"aid\":\"0\",\"scy\":\"auto\",\"net\":\"ws\",\"type\":\"none\",\"host\":\"${DOMAIN}\",\"path\":\"/vmess-ws\",\"tls\":\"tls\",\"sni\":\"${custom_sni}\",\"alpn\":\"h2,http/1.1\",\"allowInsecure\":1}"
        else
            json="{\"v\":\"2\",\"ps\":\"${user}\",\"add\":\"${DOMAIN}\",\"port\":\"443\",\"id\":\"${uuid}\",\"aid\":\"0\",\"scy\":\"auto\",\"net\":\"grpc\",\"type\":\"none\",\"host\":\"${DOMAIN}\",\"path\":\"vmess-grpc\",\"tls\":\"tls\",\"sni\":\"${custom_sni}\",\"alpn\":\"h2\",\"allowInsecure\":1}"
        fi
        link="vmess://$(echo -n "$json" | base64 -w 0)"
        
        echo "${user};${quota};0;${exp_date};${active_inbound};${uuid}" >> "$DB_CLIENTS"
        
    elif [ "$protocol" == "TROJAN" ]; then
        read -p "🔑 Password (Enter=auto): " manual_pass
        password=${manual_pass:-$(openssl rand -hex 16)}
        
        jq --arg tag "$tag" --arg u "$user" --arg pass "$password" \
            '(.inbounds[] | select(.tag==$tag).settings.clients) += [{"password":$pass,"email":$u,"level":0}]' \
            "$CONFIG" > "$tmp" && mv "$tmp" "$CONFIG"
        
        if [ "$transport" == "WS" ]; then
            link="trojan://${password}@${DOMAIN}:443?security=tls&type=ws&path=%2Ftrojan-ws&host=${DOMAIN}&sni=${custom_sni}&alpn=h2,http/1.1&allowInsecure=1#${user}"
        else
            link="trojan://${password}@${DOMAIN}:443?security=tls&type=grpc&serviceName=trojan-grpc&mode=multi&sni=${custom_sni}&alpn=h2&allowInsecure=1#${user}"
        fi
        
        echo "${user};${quota};0;${exp_date};${active_inbound};${password}" >> "$DB_CLIENTS"
    fi
    
    restart_xray
    
    clear
    echo -e "${GREEN}╔════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║          ✅ USER CREATED!                              ║${NC}"
    echo -e "${GREEN}╚════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "${CYAN}Account:${NC}"
    echo -e "  User    : ${WHITE}$user${NC}"
    echo -e "  Quota   : ${WHITE}${quota} GB${NC}"
    echo -e "  Expired : ${WHITE}$exp_date${NC}"
    echo ""
    echo -e "${YELLOW}🔗 Link:${NC}"
    echo -e "${GREEN}$link${NC}"
    echo ""
    
    generate_qr "$link" "$user"
    read -p "Press Enter..."
}

function list_users() {
    show_header
    echo -e "${YELLOW}╔════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${YELLOW}║                    USER LIST                                   ║${NC}"
    echo -e "${YELLOW}╚════════════════════════════════════════════════════════════════╝${NC}"
    
    printf "%-4s %-15s %-12s %-10s %-12s %-12s\n" "No" "User" "Protocol" "Quota" "Used" "Days Left"
    echo "----------------------------------------------------------------"
    
    i=1
    while IFS=';' read -r user quota used exp proto id; do
        [ -z "$user" ] && continue
        
        [ -z "$used" ] || ! [[ "$used" =~ ^[0-9]+$ ]] && used=0
        
        used_hr=$(format_bytes $used)
        exp_ts=$(date -d "$exp" +%s 2>/dev/null || echo 0)
        now=$(date +%s)
        days=$(( (exp_ts - now) / 86400 ))
        
        [ "$days" -lt 0 ] && days="${RED}EXPIRED${NC}" || days="${GREEN}${days}d${NC}"
        
        printf "%-4s %-15s %-12s %-10s %-12s " "$i" "$user" "$proto" "${quota}GB" "$used_hr"
        echo -e "$days"
        ((i++))
    done < "$DB_CLIENTS"
    
    [ $i -eq 1 ] && echo -e "${YELLOW}No users${NC}"
    
    echo "----------------------------------------------------------------"
    read -p "Press Enter..."
}

function delete_user() {
    show_header
    echo -e "${RED}╔════════════════════════════════════════════════════════╗${NC}"
    echo -e "${RED}║          DELETE USER                                  ║${NC}"
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
    
    [ -z "$user" ] && { echo -e "${RED}Invalid!${NC}"; sleep 2; return; }
    
    read -p "⚠️  Delete '$user'? (y/n): " confirm
    [ "$confirm" != "y" ] && return
    
    tmp=$(mktemp)
    jq --arg u "$user" '(.inbounds[].settings.clients) |= map(select(.email != $u))' "$CONFIG" > "$tmp" && mv "$tmp" "$CONFIG"
    grep -v "^$user;" "$DB_CLIENTS" > /tmp/db.tmp && mv /tmp/db.tmp "$DB_CLIENTS"
    
    restart_xray
    echo -e "${GREEN}✅ Deleted!${NC}"
    sleep 2
}
