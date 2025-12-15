# handlers.py
from telegram import Update, InlineKeyboardButton, InlineKeyboardMarkup
from telegram.ext import ContextTypes, ConversationHandler
from config import *
from xray_utils import create_user, delete_user, get_users, get_active_inbound, username_exists
from utils import is_admin, format_bytes, generate_config_link, generate_qr_code_image, HAS_QR
import subprocess

# Define States (Pastikan urutan ini sama dengan di config.py jika ada, atau di sini saja)
USERNAME, QUOTA, DAYS, UUID_OPTION, UUID_MANUAL, SELECT_USER_DELETE, CONFIRM_DELETE = range(7)

# --- START MENU ---
async def start(update: Update, context: ContextTypes.DEFAULT_TYPE):
    user_id = update.effective_user.id
    if not is_admin(user_id, ADMIN_ID):
        await update.message.reply_text("⛔ Unauthorized Access!")
        return
        
    kb = [
        [InlineKeyboardButton("📊 System Status", callback_data='status')],
        [InlineKeyboardButton("👥 List Users", callback_data='users')],
        [InlineKeyboardButton("📈 Statistics", callback_data='stats')],
        [
            InlineKeyboardButton("➕ Create User", callback_data='create'),
            InlineKeyboardButton("🗑️ Delete User", callback_data='delete')
        ],
        [InlineKeyboardButton("🔄 Restart Xray", callback_data='restart')],
        [InlineKeyboardButton("ℹ️ Help", callback_data='help')]
    ]
    
    text = f"🤖 *Xray Panel Bot v3.3.5*\n🌐 Domain: `{DOMAIN}`\n🔐 Admin Access Granted"
    
    if update.callback_query:
        await update.callback_query.edit_message_text(text, reply_markup=InlineKeyboardMarkup(kb), parse_mode='Markdown')
    else:
        await update.message.reply_text(text, reply_markup=InlineKeyboardMarkup(kb), parse_mode='Markdown')

async def button_handler(update: Update, context: ContextTypes.DEFAULT_TYPE):
    query = update.callback_query
    await query.answer()
    data = query.data
    
    if data == 'back':
        await start(update, context)
        return ConversationHandler.END
    
    if data == 'status':
        res = subprocess.run(['systemctl', 'is-active', 'xray'], capture_output=True, text=True)
        status = '🟢 RUNNING' if res.stdout.strip() == 'active' else '🔴 STOPPED'
        users = get_users()
        inbound = get_active_inbound()
        ib_text = inbound['full'] if inbound else "Not Configured"
        ram = subprocess.getoutput("free -m | awk 'NR==2{printf \"%.1f%%\", $3*100/$2}'")
        
        text = f"📊 *SYSTEM STATUS*\n{status} *Xray Service*\n📡 Protocol: `{ib_text}`\n👥 Users: `{len(users)}`\n💾 RAM: `{ram}`"
        kb = [[InlineKeyboardButton("🔙 Back", callback_data='back')]]
        await query.edit_message_text(text, reply_markup=InlineKeyboardMarkup(kb), parse_mode='Markdown')
        
    elif data == 'users':
        users = get_users()
        text = f"👥 *USER LIST ({len(users)})*\n\n"
        for u in users[:15]: 
            icon = "✅" if u['days'] > 0 else "❌"
            text += f"{icon} `{u['username']}` | {u['days']}d left\n"
        if len(users) > 15: text += "\n_...and more._"
        kb = [[InlineKeyboardButton("🔙 Back", callback_data='back')]]
        await query.edit_message_text(text, reply_markup=InlineKeyboardMarkup(kb), parse_mode='Markdown')

    elif data == 'stats':
        users = get_users()
        text = "📈 *DATA USAGE*\n\n"
        for u in users[:10]:
            used = format_bytes(u.get('usage', 0))
            text += f"👤 `{u['username']}`: `{used}` / `{u['quota']}GB`\n"
        if not users: text += "No data."
        kb = [[InlineKeyboardButton("🔙 Back", callback_data='back')]]
        await query.edit_message_text(text, reply_markup=InlineKeyboardMarkup(kb), parse_mode='Markdown')
        
    elif data == 'restart':
        await query.edit_message_text("🔄 Restarting Xray...")
        subprocess.run(['systemctl', 'restart', 'xray'])
        kb = [[InlineKeyboardButton("🔙 Back", callback_data='back')]]
        await query.edit_message_text("✅ *Xray Restarted!*", reply_markup=InlineKeyboardMarkup(kb), parse_mode='Markdown')

    elif data == 'help':
        text = "ℹ️ *BOT HELP*\nUse buttons to manage users."
        kb = [[InlineKeyboardButton("🔙 Back", callback_data='back')]]
        await query.edit_message_text(text, reply_markup=InlineKeyboardMarkup(kb), parse_mode='Markdown')

    # ENTRY POINTS FOR CONVERSATION
    elif data == 'create':
        await query.edit_message_text("🆕 *CREATE USER*\nEnter **Username**:", parse_mode='Markdown')
        return USERNAME
        
    elif data == 'delete':
        users = get_users()
        if not users:
            kb = [[InlineKeyboardButton("🔙 Back", callback_data='back')]]
            await query.edit_message_text("📭 No users.", reply_markup=InlineKeyboardMarkup(kb))
            return ConversationHandler.END
        
        kb = []
        for u in users[:10]:
            kb.append([InlineKeyboardButton(f"🗑️ {u['username']}", callback_data=f"del_{u['username']}")])
        kb.append([InlineKeyboardButton("🔙 Back", callback_data='back')])
        await query.edit_message_text("🗑️ Select User to Delete:", reply_markup=InlineKeyboardMarkup(kb))
        return SELECT_USER_DELETE

# --- CREATE USER FLOW ---
async def get_username(update: Update, context: ContextTypes.DEFAULT_TYPE):
    user = update.message.text.strip()
    if not user.isalnum():
        await update.message.reply_text("❌ Alphanumeric only!")
        return USERNAME
    if username_exists(user):
        await update.message.reply_text("❌ User exists! Try again:")
        return USERNAME
    context.user_data['u'] = user
    await update.message.reply_text(f"✅ User: {user}\n📦 Enter **Quota (GB)**:")
    return QUOTA

async def get_quota(update: Update, context: ContextTypes.DEFAULT_TYPE):
    try:
        q = float(update.message.text.strip())
        context.user_data['q'] = q
        await update.message.reply_text(f"✅ Quota: {q}GB\n📅 Enter **Active Days**:")
        return DAYS
    except:
        await update.message.reply_text("❌ Numbers only!")
        return QUOTA

async def get_days(update: Update, context: ContextTypes.DEFAULT_TYPE):
    try:
        d = int(update.message.text.strip())
        context.user_data['d'] = d
        
        kb = [[
            InlineKeyboardButton("🤖 Auto UUID", callback_data='auto'),
            InlineKeyboardButton("✏️ Manual UUID", callback_data='manual')
        ]]
        await update.message.reply_text("🔑 **Choose UUID/Password:**", reply_markup=InlineKeyboardMarkup(kb), parse_mode='Markdown')
        return UUID_OPTION
    except:
        await update.message.reply_text("❌ Numbers only!")
        return DAYS

async def get_uuid_option(update: Update, context: ContextTypes.DEFAULT_TYPE):
    query = update.callback_query
    await query.answer()
    
    if query.data == 'auto':
        return await perform_create(query, context, None)
    else: # manual
        await query.edit_message_text("✏️ Enter custom **UUID** or **Password**:", parse_mode='Markdown')
        return UUID_MANUAL

async def get_uuid_manual(update: Update, context: ContextTypes.DEFAULT_TYPE):
    manual_id = update.message.text.strip()
    return await perform_create(update, context, manual_id)

async def perform_create(obj, context, manual_id):
    u = context.user_data['u']
    q = context.user_data['q']
    d = context.user_data['d']
    
    if hasattr(obj, 'message'):
        reply_method = obj.message.reply_text
        reply_photo = obj.message.reply_photo
    else:
        reply_method = obj.edit_message_text
        reply_photo = obj.message.reply_photo

    try:
        res = create_user(u, q, d, manual_id)
        link = generate_config_link(u, res['protocol'], res['transport'], res['id'], DOMAIN)
        
        msg = (f"✅ *USER CREATED*\n\n"
               f"👤 User: `{u}`\n"
               f"📦 Quota: `{q} GB`\n"
               f"📅 Expired: `{res['expired']}`\n"
               f"🔗 Link:\n`{link}`")
        
        await reply_method(msg, parse_mode='Markdown')
        
        qr = generate_qr_code_image(link, u)
        if qr: await reply_photo(photo=qr, caption=f"📱 QR Code: {u}")
            
    except Exception as e:
        await reply_method(f"❌ Error: {str(e)}", parse_mode='Markdown')
        
    return ConversationHandler.END

# --- DELETE USER FLOW ---
async def delete_handler(update: Update, context: ContextTypes.DEFAULT_TYPE):
    query = update.callback_query
    await query.answer()
    data = query.data
    
    if data == 'back':
        await start(update, context)
        return ConversationHandler.END
        
    user = data.replace('del_', '')
    context.user_data['delete_user'] = user
    
    kb = [
        [InlineKeyboardButton("✅ YES", callback_data='confirm')],
        [InlineKeyboardButton("❌ NO", callback_data='cancel')]
    ]
    await query.edit_message_text(f"⚠️ Delete user `{user}`?", reply_markup=InlineKeyboardMarkup(kb), parse_mode='Markdown')
    return CONFIRM_DELETE

async def confirm_delete(update: Update, context: ContextTypes.DEFAULT_TYPE):
    query = update.callback_query
    await query.answer()
    
    if query.data == 'confirm':
        user = context.user_data['delete_user']
        if delete_user(user):
            await query.edit_message_text(f"✅ `{user}` Deleted!", parse_mode='Markdown')
        else:
            await query.edit_message_text("❌ Failed to delete.")
    else:
        await query.edit_message_text("❌ Cancelled.")
        
    return ConversationHandler.END

async def cancel(update: Update, context: ContextTypes.DEFAULT_TYPE):
    await update.message.reply_text("🚫 Cancelled.")
    return ConversationHandler.END
