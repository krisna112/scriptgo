#!/bin/bash
# Monitoring Module

function get_user_status() {
    local email=$1
    local stats=$(timeout 2 xray api statsquery --server=127.0.0.1:10085 -pattern "user>>>${email}>>>" 2>/dev/null)
    
    if [ -n "$stats" ] && echo "$stats" | grep -q "user>>>${email}"; then
        echo "ONLINE"
    else
        echo "OFFLINE"
    fi
}

function realtime_stats() {
    local auto=${1:-0}
    
    show_header
    echo -e "${BLUE}╔════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║               REAL-TIME USAGE STATISTICS                       ║${NC}"
    echo -e "${BLUE}╚════════════════════════════════════════════════════════════════╝${NC}"
    
    [ ! -s "$DB_CLIENTS" ] && { echo -e "${YELLOW}No users${NC}"; read -p "Enter..."; return; }
    
    systemctl is-active --quiet xray || { echo -e "${RED}Xray stopped!${NC}"; read -p "Enter..."; return; }
    
    # Load Historical Stats
    STATS_DB="/etc/xray/stats.db"
    declare -A hist_up
    declare -A hist_down
    
    if [ -f "$STATS_DB" ]; then
        while IFS=';' read -r u h_up h_down; do
            hist_up["$u"]=$h_up
            hist_down["$u"]=$h_down
        done < "$STATS_DB"
    fi

    echo -e "${CYAN}🔄 Fetching data...${NC}"
    echo ""
    
    printf "%-15s %-10s %-12s %-12s %-10s %-12s\n" "User" "Quota" "Upload" "Download" "Usage" "Status"
    echo "----------------------------------------------------------------"
    
    total_up=0
    total_down=0
    online=0
    
    while IFS=';' read -r user quota used exp proto id; do
        [ -z "$user" ] && continue
        
        # Live Stats (Since last reset)
        up=$(timeout 2 xray api stats -server=127.0.0.1:10085 -name "user>>>${user}>>>traffic>>>uplink" 2>/dev/null | grep -o '"value":[0-9]*' | cut -d: -f2)
        down=$(timeout 2 xray api stats -server=127.0.0.1:10085 -name "user>>>${user}>>>traffic>>>downlink" 2>/dev/null | grep -o '"value":[0-9]*' | cut -d: -f2)
        
        [ -z "$up" ] && up=0
        [ -z "$down" ] && down=0
        
        # Historical Stats
        h_up=${hist_up["$user"]:-0}
        h_down=${hist_down["$user"]:-0}
        
        # Combine
        final_up=$((h_up + up))
        final_down=$((h_down + down))
        total=$((final_up + final_down))
        
        # Accumulate Finals for Footer
        total_up=$((total_up + final_up))
        total_down=$((total_down + final_down))
        
        up_hr=$(format_bytes $final_up)
        down_hr=$(format_bytes $final_down)
        
        quota_bytes=$(awk "BEGIN {print $quota * 1073741824}")
        if [ "$quota_bytes" -gt 0 ]; then
             pct=$(awk "BEGIN {printf \"%.1f\", ($total * 100) / $quota_bytes}")
        else
             pct="0.0"
        fi
        
        status=$(get_user_status "$user")
        [ "$status" == "ONLINE" ] && { status="${GREEN}● ON${NC}"; ((online++)); } || status="${RED}○ OFF${NC}"
        
        printf "%-15s %-10s %-12s %-12s %-10s " "$user" "${quota}GB" "$up_hr" "$down_hr" "${pct}%"
        echo -e "$status"
    done < "$DB_CLIENTS"
    
    echo "----------------------------------------------------------------"
    echo ""
    echo -e "${CYAN}Total Upload  : ${WHITE}$(format_bytes $total_up)${NC}"
    echo -e "${CYAN}Total Download: ${WHITE}$(format_bytes $total_down)${NC}"
    echo -e "${CYAN}Online Users  : ${GREEN}$online${NC}"
    echo ""
    
    [ "$auto" -eq 1 ] && { sleep 5; realtime_stats 1; return; }
    
    echo -e "${WHITE}[r] Refresh  [a] Auto-refresh  [Enter] Back${NC}"
    read -p "Choose: " opt
    
    case $opt in
        r|R) realtime_stats 0 ;;
        a|A) realtime_stats 1 ;;
    esac
}

function test_api_connection() {
    show_header
    echo -e "${BLUE}╔════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║          API CONNECTION TEST                          ║${NC}"
    echo -e "${BLUE}╚════════════════════════════════════════════════════════╝${NC}"
    
    echo -e "${CYAN}Testing...${NC}"
    echo ""
    
    echo -n "1. Xray Service : "
    systemctl is-active --quiet xray && echo -e "${GREEN}✓ RUNNING${NC}" || echo -e "${RED}✗ STOPPED${NC}"
    
    echo -n "2. API Port     : "
    netstat -tuln | grep -q ":10085" && echo -e "${GREEN}✓ LISTENING${NC}" || echo -e "${RED}✗ NOT LISTENING${NC}"
    
    echo -n "3. Stats API    : "
    timeout 2 xray api statsquery --server=127.0.0.1:10085 &>/dev/null && echo -e "${GREEN}✓ OK${NC}" || echo -e "${RED}✗ FAILED${NC}"
    
    echo -n "4. Users        : "
    count=$(wc -l < "$DB_CLIENTS" 2>/dev/null)
    echo -e "${GREEN}$count users${NC}"
    
    echo ""
    read -p "Press Enter..."
}
