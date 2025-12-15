import os
import json
import uuid
import subprocess
import psutil
import time
import base64
import shutil
import zipfile
import io
from datetime import datetime, timedelta
from flask import Flask, render_template, request, redirect, url_for, session, send_from_directory, send_file
import security

app = Flask(__name__)

# --- SECURITY HOOKS ---
@app.after_request
def apply_headers(response):
    return security.add_security_headers(response)

@app.before_request
def security_checks():
    security.rate_limit_check()
    security.waf_check()

@app.route('/robots.txt')
def robots():
    return send_from_directory('static', 'robots.txt')

# --- SESSION CONFIG ---
SECRET_FILE = '/etc/xray/web_secret.key'
if not os.path.exists(SECRET_FILE):
    try:
        with open(SECRET_FILE, 'wb') as f:
            f.write(os.urandom(32))
    except:
        app.secret_key = os.urandom(32)

if os.path.exists(SECRET_FILE):
    with open(SECRET_FILE, 'rb') as f:
        app.secret_key = f.read()
else:
    app.secret_key = b'fallback_key_xray_panel'

# --- CONFIG PATHS ---
DB_CLIENTS = '/etc/xray/clients.db'
DB_INBOUNDS = '/etc/xray/inbounds.db'
CONFIG_XRAY = '/usr/local/etc/xray/config.json'
ADMIN_CONFIG = '/etc/xray/web_admin.json'
DOMAIN_FILE = '/root/domain'
LOG_FILE = '/var/log/xray/access.log'

# --- HELPER FUNCTIONS ---

def get_admin_creds():
    default = {"username": "admin", "password": "admin123"}
    if not os.path.exists(ADMIN_CONFIG):
        return default
    try:
        with open(ADMIN_CONFIG, 'r') as f:
            return json.load(f)
    except:
        return default

def save_admin_creds(u, p):
    with open(ADMIN_CONFIG, 'w') as f:
        json.dump({"username": u, "password": p}, f)

def get_domain():
    try:
        with open(DOMAIN_FILE, 'r') as f:
            return f.read().strip()
    except:
        return "localhost"

def format_bytes(size):
    try:
        power = 2**10
        n = 0
        power_labels = {0 : '', 1: 'K', 2: 'M', 3: 'G', 4: 'T'}
        while size > power:
            size /= power
            n += 1
        return f"{size:.2f} {power_labels[n]}B"
    except:
        return "0 B"

def reload_xray():
    subprocess.run(["systemctl", "restart", "xray"])

def load_xray_config():
    with open(CONFIG_XRAY, 'r') as f:
        return json.load(f)

def save_xray_config(config):
    with open(CONFIG_XRAY, 'w') as f:
        json.dump(config, f, indent=2)

def get_active_inbound():
    try:
        with open(DB_INBOUNDS, 'r') as f:
            return f.read().strip().split(';')[1]
    except:
        return None

def generate_random_id():
    return os.urandom(8).hex()

def get_xray_status():
    try:
        # Check if xray service is active
        subprocess.check_call(["systemctl", "is-active", "--quiet", "xray"])
        return True
    except:
        return False

def get_system_uptime():
    try:
        return subprocess.check_output("uptime -p", shell=True).decode().strip().replace("up ", "")
    except:
        return "Unknown"

def get_install_duration():
    try:
        # Use SECRET_FILE creation time as install time proxy
        if os.path.exists(SECRET_FILE):
            install_time = datetime.fromtimestamp(os.path.getctime(SECRET_FILE))
            diff = datetime.now() - install_time
            days = diff.days
            return f"{days} days"
    except:
        pass
    return "Unknown"

# --- LINK GENERATOR ---
def generate_link(user, uuid, full_proto):
    domain = get_domain()
    try:
        clean_proto = full_proto.replace("-EXPIRED", "").replace("-DISABLED", "").upper()
        
        if '-' in clean_proto:
            proto, transport = clean_proto.split('-')
        else:
            proto = clean_proto
            transport = "TCP"
            
    except:
        return "#"

    if proto == "VLESS":
        if "XTLS" in transport:
            return f"vless://{uuid}@{domain}:443?security=tls&encryption=none&flow=xtls-rprx-vision&type=tcp&sni={domain}&alpn=h2,http/1.1#{user}"
        elif "WS" in transport:
            return f"vless://{uuid}@{domain}:443?security=tls&encryption=none&type=ws&path=%2Fvless-ws&host={domain}&sni={domain}&alpn=h2,http/1.1#{user}"
        elif "GRPC" in transport:
            return f"vless://{uuid}@{domain}:443?security=tls&encryption=none&type=grpc&serviceName=vless-grpc&mode=multi&sni={domain}&alpn=h2#{user}"
            
    elif proto == "VMESS":
        vmess_data = {
            "v": "2", "ps": user, "add": domain, "port": "443", "id": uuid, "aid": "0",
            "scy": "auto", 
            "net": "ws" if "WS" in transport else "grpc",
            "type": "none", "host": domain, "tls": "tls", "sni": domain,
            "alpn": "h2,http/1.1" if "WS" in transport else "h2"
        }
        if "WS" in transport: vmess_data["path"] = "/vmess-ws"
        else: vmess_data["path"] = "vmess-grpc"
        
        b64_link = base64.b64encode(json.dumps(vmess_data).encode()).decode()
        return f"vmess://{b64_link}"
        
    elif proto == "TROJAN":
        if "WS" in transport:
            return f"trojan://{uuid}@{domain}:443?security=tls&type=ws&path=%2Ftrojan-ws&host={domain}&sni={domain}&alpn=h2,http/1.1#{user}"
        elif "GRPC" in transport:
            return f"trojan://{uuid}@{domain}:443?security=tls&type=grpc&serviceName=trojan-grpc&mode=multi&sni={domain}&alpn=h2#{user}"
            
    return "#"

# --- CORE LOGIC ---

def get_realtime_traffic(email):
    try:
        cmd_up = f"xray api stats --server=127.0.0.1:10085 -name 'user>>>{email}>>>traffic>>>uplink'"
        res_up = subprocess.check_output(cmd_up, shell=True, stderr=subprocess.DEVNULL).decode()
        up = int(json.loads(res_up)['stat']['value'])
        
        cmd_down = f"xray api stats --server=127.0.0.1:10085 -name 'user>>>{email}>>>traffic>>>downlink'"
        res_down = subprocess.check_output(cmd_down, shell=True, stderr=subprocess.DEVNULL).decode()
        down = int(json.loads(res_down)['stat']['value'])
        return up + down
    except:
        return 0

def get_active_users_from_log():
    active_users = set()
    if not os.path.exists(LOG_FILE):
        return active_users
        
    try:
        # Read last 2000 lines efficiently
        cmd = f"tail -n 2000 {LOG_FILE}"
        output = subprocess.check_output(cmd, shell=True, stderr=subprocess.DEVNULL).decode('utf-8', errors='ignore')
        
        now = datetime.now()
        threshold = timedelta(seconds=10) # 10 Seconds Timeout
        
        # Regex to capture timestamp and email
        # Log format: 2023/12/11 15:40:00 ... email: user@example.com
        import re
        pattern = re.compile(r'(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}).*email:\s+(\S+)')
        
        for line in reversed(output.splitlines()):
            match = pattern.search(line)
            if match:
                ts_str, email = match.groups()
                try:
                    log_time = datetime.strptime(ts_str, "%Y/%m/%d %H:%M:%S")
                    if now - log_time < threshold:
                        active_users.add(email)
                    else:
                        # Since we read in reverse, once we hit older logs, we can stop for optimization
                        # BUT logs might be slightly out of order if heavily concurrent, so maybe check a bit more
                        # For 10s window, it's very short. Safe to break if > 1 min diff maybe?
                        # Let's just process the 2000 lines, it's fast enough.
                        pass
                except:
                    continue
    except:
        pass
        
    return active_users

def load_clients_enhanced():
    clients = []
    if not os.path.exists(DB_CLIENTS):
        return clients
    
    active_users_log = get_active_users_from_log()
    
    with open(DB_CLIENTS, 'r') as f:
        for line in f:
            try:
                parts = line.strip().split(';')
                if len(parts) < 6: continue
                
                email = parts[0]
                raw_proto = parts[4]
                try: db_usage = int(parts[2])
                except: db_usage = 0
                
                # Still get traffic for total usage display
                session_traffic = get_realtime_traffic(email)
                link = generate_link(email, parts[5], raw_proto)
                
                is_disabled = "DISABLED" in raw_proto or "EXPIRED" in raw_proto
                
                # Hitung Persentase
                quota_gb = float(parts[1])
                total_used = db_usage + session_traffic
                quota_bytes = quota_gb * 1073741824
                
                percent = 0
                if quota_bytes > 0:
                    percent = (total_used / quota_bytes) * 100
                percent = round(percent, 2)
 
                # Hitung Hari
                days_left = 0
                is_expired = False
                try:
                    exp_dt = datetime.strptime(parts[3], "%Y-%m-%d %H:%M:%S")
                    diff = exp_dt - datetime.now()
                    total_seconds = diff.total_seconds()
                    
                    if total_seconds > 0:
                        days_left = int((total_seconds + 86399) // 86400)
                    else:
                        days_left = -1
                        
                    is_expired = days_left < 0
                except:
                    pass
 
                clients.append({
                    'user': email,
                    'quota': quota_gb,
                    'used': total_used,
                    'used_fmt': format_bytes(total_used),
                    'percent': percent,
                    'exp': parts[3],
                    'days': days_left,
                    'is_expired': is_expired,
                    'proto': raw_proto,
                    'uuid': parts[5],
                    'is_online': email in active_users_log, # Changed to use log
                    'is_disabled': is_disabled,
                    'progress_class': 'bg-red-500' if percent > 90 else 'bg-emerald-500',
                    'width_style': f"width: {percent}%;",
                    'link': link
                })
            except:
                continue
    return clients

def sync_config_from_db():
    try:
        if not os.path.exists(DB_CLIENTS): return False
        
        conf = load_xray_config()
        
        # 1. Clear existing clients in all inbounds to avoid duplicates
        for inb in conf['inbounds']:
            if 'settings' in inb and 'clients' in inb['settings']:
                inb['settings']['clients'] = []
                
        # 2. Re-populate from DB
        with open(DB_CLIENTS, 'r') as f:
            for line in f:
                try:
                    parts = line.strip().split(';')
                    if len(parts) < 6: continue
                    
                    user = parts[0]
                    # exp = parts[3] # Not strictly needed for config, Xray handles authentication
                    proto_raw = parts[4] # e.g. VLESS-XTLS
                    uuid = parts[5]
                    
                    # Clean tags
                    clean_proto = proto_raw.replace("-EXPIRED", "").replace("-DISABLED", "")
                    
                    # Determine inbound tag
                    if '-' in clean_proto:
                        p, t = clean_proto.split('-')
                        tag = f"{p.lower()}-{t.lower()}"
                    else:
                        tag = clean_proto.lower()
                        
                    # Find matching inbound and add user
                    for inb in conf['inbounds']:
                        if inb.get('tag') == tag:
                            new_client = {"email": user}
                            if "TROJAN" in clean_proto.upper():
                                new_client["password"] = uuid
                            else:
                                new_client["id"] = uuid
                                if "XTLS" in clean_proto.upper():
                                    new_client["flow"] = "xtls-rprx-vision"
                                    
                            inb['settings'].setdefault("clients", []).append(new_client)
                except:
                    continue
                    
        save_xray_config(conf)
        return True
    except Exception as e:
        print(f"Sync Error: {e}")
        return False

# --- ROUTES ---

@app.before_request
def require_login():
    if request.endpoint not in ['login', 'static'] and not session.get('logged_in'):
        return redirect(url_for('login'))

@app.route('/')
def index():
    users = load_clients_enhanced()
    users = load_clients_enhanced()
    try:
        cpu = psutil.cpu_percent()
        ram = psutil.virtual_memory().percent
    except:
        cpu = 0
        ram = 0
        
    total_usage = sum(u['used'] for u in users)
    
    return render_template('dashboard.html', users=users, info={
        'cpu': cpu, 
        'ram': ram, 
        'users': len(users),
        'online': sum(1 for u in users if u['is_online']),
        'xray_status': get_xray_status(),
        'uptime': get_system_uptime(),
        'install_duration': get_install_duration(),
        'total_usage': format_bytes(total_usage),
        'cpu_style': f"width: {cpu}%;"
    })

@app.route('/login', methods=['GET', 'POST'])
def login():
    if session.get('logged_in'):
        return redirect(url_for('index'))
    
    creds = get_admin_creds()
    if request.method == 'POST':
        if request.form['username'] == creds['username'] and request.form['password'] == creds['password']:
            session['logged_in'] = True
            return redirect(url_for('index'))
        else:
            return render_template('login.html', error="Invalid Credentials")
    return render_template('login.html')

@app.route('/logout')
def logout():
    session.clear()
    return redirect(url_for('login'))

@app.route('/settings', methods=['GET', 'POST'])
def settings():
    creds = get_admin_creds()
    msg = None
    if request.method == 'POST':
        save_admin_creds(request.form['username'], request.form['password'])
        creds = get_admin_creds()
        msg = "Settings updated!"
    if request.method == 'POST':
        save_admin_creds(request.form['username'], request.form['password'])
        creds = get_admin_creds()
        msg = "Settings updated!"
    return render_template('settings.html', creds=creds, success=msg)

@app.route('/restart_xray', methods=['POST'])
def restart_service():
    try:
        reload_xray()
        return render_template('settings.html', creds=get_admin_creds(), success="Xray Service Restarted Successfully!")
    except Exception as e:
        return render_template('settings.html', creds=get_admin_creds(), error=f"Restart Failed: {str(e)}")

@app.route('/backup')
def backup_data():
    # In-memory zip creation
    memory_file = io.BytesIO()
    with zipfile.ZipFile(memory_file, 'w') as zf:
        if os.path.exists(DB_CLIENTS):
            zf.write(DB_CLIENTS, 'clients.db')
        if os.path.exists(DB_INBOUNDS):
            zf.write(DB_INBOUNDS, 'inbounds.db')
            
    memory_file.seek(0)
    
    timestamp = datetime.now().strftime("%Y-%m-%d_%H-%M")
    return send_file(memory_file, download_name=f"xray_backup_{timestamp}.zip", as_attachment=True)

@app.route('/restore', methods=['POST'])
def restore_data():
    if 'backup_file' not in request.files:
        return redirect(url_for('settings'))
    
    file = request.files['backup_file']
    if file.filename == '':
        return redirect(url_for('settings'))
        
    if file:
        try:
            # Save temporary
            temp_path = '/tmp/restore.zip'
            file.save(temp_path)
            
            with zipfile.ZipFile(temp_path, 'r') as zf:
                # Extract specific files to /etc/xray/
                # We enforce names to prevent directory traversal or junk files
                for filename in ['clients.db', 'inbounds.db']:
                    if filename in zf.namelist():
                        source = zf.open(filename)
                        target = open(os.path.join('/etc/xray', filename), "wb")
                        with source, target:
                            shutil.copyfileobj(source, target)
                            
            # Sync Config from DB
            sync_config_from_db()

            # Reload System
            reload_xray()
            return render_template('settings.html', creds=get_admin_creds(), success="Restore Successful! Reboot Xray...")
        except Exception as e:
            return render_template('settings.html', creds=get_admin_creds(), error=f"Restore Failed: {str(e)}")
            
    return redirect(url_for('settings'))

@app.route('/add', methods=['GET', 'POST'])
def add_user():
    inbound_type = get_active_inbound()
    if not inbound_type:
        return "Error: No inbound configured!"

    if request.method == 'POST':
        username = request.form['username']
        quota = request.form['quota']
        expiry_mode = request.form.get('expiry_mode', 'days')
        
        if expiry_mode == 'days':
            days = int(request.form.get('days', 30))
            exp_date = (datetime.now() + timedelta(days=days)).strftime("%Y-%m-%d %H:%M:%S")
        else:
            date_input = request.form.get('date_input')
            try:
                # Expecting format YYYY-MM-DDTHH:MM from HTML datetime-local
                dt = datetime.strptime(date_input, "%Y-%m-%dT%H:%M")
                exp_date = dt.strftime("%Y-%m-%d %H:%M:%S")
            except:
                return "Invalid Date Format!"

        mode = request.form.get('uuid_mode')
        custom = request.form.get('custom_uuid')
        
        if mode == 'manual' and custom:
            new_uuid = custom
        else:
            new_uuid = generate_random_id()

        clients = load_clients_enhanced()
        if any(c['user'] == username for c in clients):
            return "User exists!"
        
        with open(DB_CLIENTS, 'a') as f:
            f.write(f"{username};{quota};0;{exp_date};{inbound_type};{new_uuid}\n")
        
        conf = load_xray_config()
        target_tag = inbound_type.lower()
        found = False
        for inb in conf['inbounds']:
            if inb.get('tag') == target_tag:
                client = {"email": username}
                if "TROJAN" in inbound_type:
                    client["password"] = new_uuid
                else: 
                    client["id"] = new_uuid
                    if "xtls" in target_tag:
                        client["flow"] = "xtls-rprx-vision"
                
                inb['settings'].setdefault("clients", []).append(client)
                found = True
                break
        
        if found:
            save_xray_config(conf)
            reload_xray()
            return redirect(url_for('index'))
        
    return render_template('form.html', action="Add", inbound=inbound_type)

@app.route('/edit/<username>', methods=['GET', 'POST'])
def edit_user(username):
    if not os.path.exists(DB_CLIENTS): return redirect(url_for('index'))
    
    with open(DB_CLIENTS, 'r') as f: lines = f.readlines()
    
    target_idx = -1
    for i, line in enumerate(lines):
        try:
            parts = line.strip().split(';')
            if len(parts) >= 6 and parts[0] == username:
                target_idx = i
                break
        except:
            continue
            
    if target_idx == -1: return redirect(url_for('index'))
    
    parts = lines[target_idx].strip().split(';')
    user_data = {
        'user': parts[0],
        'quota': parts[1],
        'used': parts[2],
        'exp': parts[3],
        'proto': parts[4],
        'uuid': parts[5]
    }
    
    try:
        # Format for datetime-local input (YYYY-MM-DDTHH:MM)
        dt_obj = datetime.strptime(parts[3], "%Y-%m-%d %H:%M:%S")
        user_data['exp_input'] = dt_obj.strftime("%Y-%m-%dT%H:%M")
    except:
        user_data['exp_input'] = ""

    if request.method == 'POST':
        new_quota = request.form['quota']
        new_uuid = request.form.get('uuid', user_data['uuid'])
        add_days = request.form.get('add_days')
        
        new_exp = user_data['exp']
        clean_proto = user_data['proto']

        expiry_mode = request.form.get('expiry_mode', 'extend')
        
        if expiry_mode == 'extend':
            if add_days and int(add_days) > 0:
                try:
                    curr = datetime.strptime(user_data['exp'], "%Y-%m-%d %H:%M:%S")
                    base = datetime.now() if curr < datetime.now() else curr
                    new_exp = (base + timedelta(days=int(add_days))).strftime("%Y-%m-%d %H:%M:%S")
                    clean_proto = user_data['proto'].replace("-EXPIRED", "").replace("-DISABLED", "")
                except: pass
        elif expiry_mode == 'date':
            date_input = request.form.get('date_input')
            if date_input:
                try:
                    dt = datetime.strptime(date_input, "%Y-%m-%dT%H:%M")
                    new_exp = dt.strftime("%Y-%m-%d %H:%M:%S")
                    if dt > datetime.now():
                        clean_proto = user_data['proto'].replace("-EXPIRED", "").replace("-DISABLED", "")
                except: pass
            
        lines[target_idx] = f"{user_data['user']};{new_quota};{user_data['used']};{new_exp};{clean_proto};{new_uuid}\n"
        with open(DB_CLIENTS, 'w') as f: f.writelines(lines)
        
        conf = load_xray_config()
        config_updated = False
        target_tag = clean_proto.split('-')[0].lower() + '-' + clean_proto.split('-')[1].lower() if '-' in clean_proto else clean_proto.lower()
        
        user_found = False
        for inb in conf['inbounds']:
            if inb.get('tag') == target_tag:
                if 'settings' in inb and 'clients' in inb['settings']:
                    for c in inb['settings']['clients']:
                        if c.get('email') == username:
                            if 'id' in c: c['id'] = new_uuid
                            if 'password' in c: c['password'] = new_uuid
                            user_found = True
                            config_updated = True
                
                if not user_found:
                    new_cl = {"email": username}
                    if "TROJAN" in clean_proto.upper(): new_cl["password"] = new_uuid
                    else: 
                        new_cl["id"] = new_uuid
                        if "XTLS" in clean_proto.upper(): new_cl["flow"] = "xtls-rprx-vision"
                    inb['settings'].setdefault("clients", []).append(new_cl)
                    config_updated = True

        if config_updated:
            save_xray_config(conf)
            reload_xray()
            
        return redirect(url_for('index'))

    return render_template('form.html', action="Edit", user=user_data)

@app.route('/delete/<username>')
def delete_user(username):
    if os.path.exists(DB_CLIENTS):
        with open(DB_CLIENTS, 'r') as f: lines = f.readlines()
        with open(DB_CLIENTS, 'w') as f:
            for line in lines:
                if line.strip().split(';')[0] != username: f.write(line)

    conf = load_xray_config()
    changed = False
    for inb in conf['inbounds']:
        if 'settings' in inb and 'clients' in inb['settings']:
            orig = len(inb['settings']['clients'])
            inb['settings']['clients'] = [c for c in inb['settings']['clients'] if c.get('email') != username]
            if len(inb['settings']['clients']) < orig: changed = True
            
    if changed: save_xray_config(conf); reload_xray()
    return redirect(url_for('index'))

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=5000)
