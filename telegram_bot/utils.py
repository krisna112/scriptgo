# utils.py
import io
import base64
import json
import logging

logger = logging.getLogger(__name__)

try:
    import qrcode
    from PIL import Image
    HAS_QR = True
except ImportError:
    HAS_QR = False
    logger.warning("qrcode/PIL not installed. QR codes disabled.")

def is_admin(user_id, admin_id):
    """Check if user is admin"""
    if not admin_id:
        return False
    return str(user_id) == str(admin_id)

def format_bytes(bytes_val):
    if not isinstance(bytes_val, (int, float)): return "0 B"
    if bytes_val < 1024: return f"{bytes_val} B"
    elif bytes_val < 1048576: return f"{bytes_val/1024:.2f} KB"
    elif bytes_val < 1073741824: return f"{bytes_val/1048576:.2f} MB"
    else: return f"{bytes_val/1073741824:.2f} GB"

def generate_config_link(username, protocol, transport, id_pass, domain):
    """Generates V2Ray/Xray configuration URI"""
    proto = protocol.upper()
    trans = transport.upper() if transport else 'TCP'
    
    if proto == 'VLESS':
        if trans == 'XTLS':
            return f"vless://{id_pass}@{domain}:443?security=tls&encryption=none&flow=xtls-rprx-vision&type=tcp&sni={domain}&alpn=h2,http/1.1#{username}"
        elif trans == 'WS':
            return f"vless://{id_pass}@{domain}:443?security=tls&encryption=none&type=ws&path=%2Fvless-ws&host={domain}&sni={domain}&alpn=h2,http/1.1#{username}"
        else: # gRPC
            return f"vless://{id_pass}@{domain}:443?security=tls&encryption=none&type=grpc&serviceName=vless-grpc&mode=multi&sni={domain}&alpn=h2#{username}"
            
    elif proto == 'VMESS':
        conf = {
            "v": "2", "ps": username, "add": domain, "port": "443",
            "id": id_pass, "aid": "0", "scy": "auto", 
            "net": "ws" if trans=='WS' else "grpc",
            "type": "none", "host": domain, 
            "path": "/vmess-ws" if trans=='WS' else "vmess-grpc",
            "tls": "tls", "sni": domain, 
            "alpn": "h2,http/1.1" if trans=='WS' else "h2"
        }
        return "vmess://" + base64.b64encode(json.dumps(conf).encode()).decode()
        
    elif proto == 'TROJAN':
        if trans == 'WS':
            return f"trojan://{id_pass}@{domain}:443?security=tls&type=ws&path=%2Ftrojan-ws&host={domain}&sni={domain}&alpn=h2,http/1.1#{username}"
        else: # gRPC
            return f"trojan://{id_pass}@{domain}:443?security=tls&type=grpc&serviceName=trojan-grpc&mode=multi&sni={domain}&alpn=h2#{username}"
            
    return f"Error: Unknown Protocol ({proto}-{trans})"

def generate_qr_code_image(data, username):
    """Generates QR Code image in memory"""
    if not HAS_QR: return None
    try:
        qr = qrcode.QRCode(border=4, box_size=10)
        qr.add_data(data)
        qr.make(fit=True)
        img = qr.make_image(fill_color="black", back_color="white").convert('RGB')
        bio = io.BytesIO()
        bio.name = f"{username}.png"
        img.save(bio, 'PNG')
        bio.seek(0)
        return bio
    except Exception as e:
        logger.error(f"QR Generation Error: {e}")
        return None
