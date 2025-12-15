# bot.py
#!/opt/xray-telegram-bot/bin/python3
import logging
from telegram.ext import Application, CommandHandler, CallbackQueryHandler, MessageHandler, ConversationHandler, filters
from config import *
from handlers import *

# Logging Setup
logging.basicConfig(format='%(asctime)s - %(name)s - %(levelname)s - %(message)s', level=logging.INFO)

def main():
    if not BOT_TOKEN:
        print("Error: BOT_TOKEN empty")
        return

    app = Application.builder().token(BOT_TOKEN).build()

    # --- CREATE CONVERSATION ---
    create_conv = ConversationHandler(
        entry_points=[CallbackQueryHandler(button_handler, pattern='^create$')],
        states={
            USERNAME: [MessageHandler(filters.TEXT & ~filters.COMMAND, get_username)],
            QUOTA: [MessageHandler(filters.TEXT & ~filters.COMMAND, get_quota)],
            DAYS: [MessageHandler(filters.TEXT & ~filters.COMMAND, get_days)],
            # FIX: Menambahkan Handler untuk tombol Auto/Manual UUID
            UUID_OPTION: [CallbackQueryHandler(get_uuid_option)], 
            # FIX: Menambahkan Handler untuk Input Manual UUID
            UUID_MANUAL: [MessageHandler(filters.TEXT & ~filters.COMMAND, get_uuid_manual)]
        },
        fallbacks=[CommandHandler('cancel', cancel)]
    )

    # --- DELETE CONVERSATION ---
    delete_conv = ConversationHandler(
        entry_points=[CallbackQueryHandler(button_handler, pattern='^delete$')],
        states={
            SELECT_USER_DELETE: [CallbackQueryHandler(delete_handler, pattern='^del_|back')],
            # FIX: Menambahkan Handler untuk tombol YES/NO
            CONFIRM_DELETE: [CallbackQueryHandler(confirm_delete)]
        },
        fallbacks=[CommandHandler('cancel', cancel)]
    )

    app.add_handler(CommandHandler("start", start))
    app.add_handler(create_conv)
    app.add_handler(delete_conv)
    app.add_handler(CallbackQueryHandler(button_handler))

    print("Bot Started...")
    app.run_polling()

if __name__ == "__main__":
    main()
