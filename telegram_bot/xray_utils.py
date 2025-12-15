# xray_utils.py
import json
import uuid
import subprocess
import os
import logging
from datetime import datetime, timedelta
from config import DB_CLIENTS, DB_INBOUNDS, XRAY_CONFIG

logger = logging.getLogger(__name__)

def get_users():
    users = []
    if not os.path.exists(DB_CLIENTS): return users
    try:
        with open(DB_CLIENTS, 'r') as f:
            for line in f:
                parts = line.strip().split(';')
                if len(parts) >= 4:
                    exp_str = parts[3]
                    try:
                        exp_date = datetime.strptime(exp_str, "%Y-%m-%d %H:%M:%S")
                        days_left = (exp_date - datetime.now()).days
                    except: days_left = 0
                    
                    try: usage = int(parts[2])
                    except: usage = 0

                    users.append({
                        'username': parts[0],
                        'quota': parts[1],
                        'usage': usage,
                        'expired': parts[3],
                        'days': days_left
                    })
    except Exception as e:
        logger.error(f"Error reading users: {e}")
    return users

def username_exists(username):
    users = get_users()
    for u in users:
        if u['username'] == username: return True
    return False

def get_active_inbound():
    if not os.path.exists(DB_INBOUNDS): return None
    try:
        with open(DB_INBOUNDS, 'r') as f:
            line = f.readline().strip()
            if not line: return None
            parts = line.split(';')
            if len(parts) >= 2:
                full_proto = parts[1]
                if '-' in full_proto:
                    proto = full_proto.split('-')[0]
                    trans = full_proto.split('-')[1]
                else:
                    proto = full_proto
                    trans = "TCP"
                return {'full': full_proto, 'protocol': proto, 'transport': trans}
    except: pass
    return None

def create_user(username, quota, days, manual_id=None):
    inbound_info = get_active_inbound()
    if not inbound_info: raise Exception("No active inbound found in DB!")

    target_tag = inbound_info['full'].lower()
    protocol = inbound_info['protocol'].upper()
    
    try:
        with open(XRAY_CONFIG, 'r') as f: config = json.load(f)
    except: raise Exception("Failed to read config.json")

    inbound_idx = -1
    for i, ib in enumerate(config['inbounds']):
        if ib.get('tag', '').lower() == target_tag:
            inbound_idx = i
            break
    
    if inbound_idx == -1: raise Exception(f"Inbound '{target_tag}' not found in config.")

    if protocol == 'TROJAN':
        user_id = manual_id if manual_id else subprocess.getoutput("openssl rand -hex 16")
        new_client = {"password": user_id, "email": username, "level": 0}
    else:
        user_id = manual_id if manual_id else str(uuid.uuid4())
        new_client = {"id": user_id, "email": username, "level": 0}
        if 'flow' in str(config['inbounds'][inbound_idx]['streamSettings']):
             new_client['flow'] = "xtls-rprx-vision"

    if "clients" not in config['inbounds'][inbound_idx]['settings']:
         config['inbounds'][inbound_idx]['settings']['clients'] = []
    config['inbounds'][inbound_idx]['settings']['clients'].append(new_client)
    
    with open(XRAY_CONFIG, 'w') as f: json.dump(config, f, indent=2)

    exp_date = (datetime.now() + timedelta(days=days)).strftime("%Y-%m-%d %H:%M:%S")
    with open(DB_CLIENTS, 'a') as f:
        f.write(f"{username};{quota};0;{exp_date};{inbound_info['full']};{user_id}\n")

    subprocess.run(['systemctl', 'restart', 'xray'])
    return {
        "username": username, "id": user_id, "expired": exp_date,
        "protocol": inbound_info['protocol'], "transport": inbound_info['transport']
    }

def delete_user(username):
    if not os.path.exists(DB_CLIENTS): return False
    # DB Removal
    with open(DB_CLIENTS, 'r') as f: lines = f.readlines()
    found = False
    with open(DB_CLIENTS, 'w') as f:
        for line in lines:
            if not line.startswith(f"{username};"): f.write(line)
            else: found = True
    if not found: return False

    # Config Removal
    try:
        with open(XRAY_CONFIG, 'r') as f: config = json.load(f)
        changed = False
        for ib in config['inbounds']:
            if 'settings' in ib and 'clients' in ib['settings']:
                old_len = len(ib['settings']['clients'])
                ib['settings']['clients'] = [c for c in ib['settings']['clients'] if c.get('email') != username]
                if len(ib['settings']['clients']) < old_len: changed = True
        
        if changed:
            with open(XRAY_CONFIG, 'w') as f: json.dump(config, f, indent=2)
            subprocess.run(['systemctl', 'restart', 'xray'])
            
        if os.path.exists(f"/tmp/xray_traffic_{username}"):
            os.remove(f"/tmp/xray_traffic_{username}")
        return True
    except: return False
