# config.py
import os
import logging

# Bot Configuration
BOT_TOKEN = os.environ.get('TELEGRAM_BOT_TOKEN', '')
ADMIN_ID = os.environ.get('TELEGRAM_ADMIN_ID', '')

# Xray Configuration
# FIXED: Variable name standardized to XRAY_CONFIG
XRAY_CONFIG = '/usr/local/etc/xray/config.json' 
DB_CLIENTS = '/etc/xray/clients.db'
DB_INBOUNDS = '/etc/xray/inbounds.db'

# Domain File
DOMAIN_FILE = '/root/domain'
if os.path.exists(DOMAIN_FILE):
    with open(DOMAIN_FILE, 'r') as f:
        DOMAIN = f.read().strip()
else:
    DOMAIN = 'localhost'

# Conversation States
(USERNAME, QUOTA, DAYS, UUID_OPTION, UUID_MANUAL, SELECT_USER_DELETE, CONFIRM_DELETE) = range(7)

# Logging Setup
logging.basicConfig(
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
    level=logging.INFO
)
logger = logging.getLogger(__name__)
