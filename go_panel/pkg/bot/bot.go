package bot

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/krisna112/scriptxray/go_panel/pkg/core"
)

type BotState int

const (
	Idle BotState = iota
	WaitUsername
	WaitQuota
	WaitDays
	WaitUUIDOption
	WaitUUIDManual
)

type UserSession struct {
	State    BotState
	TempUser core.Client
}

type Bot struct {
	API      *tgbotapi.BotAPI
	Sessions map[int64]*UserSession
	AdminID  int64
}

func NewBot(token string, adminID int64) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}
	return &Bot{
		API:      api,
		Sessions: make(map[int64]*UserSession),
		AdminID:  adminID,
	}, nil
}

func (b *Bot) Start() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := b.API.GetUpdatesChan(u)

	log.Println("Bot Started")

	for update := range updates {
		if update.Message != nil {
			b.handleMessage(update.Message)
		} else if update.CallbackQuery != nil {
			b.handleCallback(update.CallbackQuery)
		}
	}
}

func (b *Bot) getSession(userID int64) *UserSession {
	if _, ok := b.Sessions[userID]; !ok {
		b.Sessions[userID] = &UserSession{State: Idle}
	}
	return b.Sessions[userID]
}

func (b *Bot) handleMessage(msg *tgbotapi.Message) {
	// Auth check
	if msg.From.ID != b.AdminID {
		// Just ignore or reply unauth
		return
	}

	session := b.getSession(msg.From.ID)

	// Commands
	if msg.IsCommand() {
		switch msg.Command() {
		case "start":
			session.State = Idle
			b.sendMenu(msg.Chat.ID)
		case "cancel":
			session.State = Idle
			b.sendMessage(msg.Chat.ID, "Cancelled.")
			b.sendMenu(msg.Chat.ID)
		}
		return
	}

	// State Machine
	switch session.State {
	case WaitUsername:
		if !strings.EqualFold(msg.Text, "") { // Validate alphanumeric if needed
			session.TempUser.Username = msg.Text
			session.State = WaitQuota
			b.sendMessage(msg.Chat.ID, "✅ Enter Quota (GB):")
		}
	case WaitQuota:
		q, err := strconv.ParseFloat(msg.Text, 64)
		if err != nil {
			b.sendMessage(msg.Chat.ID, "❌ Number only!")
			return
		}
		session.TempUser.Quota = q
		session.State = WaitDays
		b.sendMessage(msg.Chat.ID, "✅ Enter Active Days:")
	case WaitDays:
		d, err := strconv.Atoi(msg.Text)
		if err != nil {
			b.sendMessage(msg.Chat.ID, "❌ Number only!")
			return
		}
		session.TempUser.Expiry = time.Now().Add(time.Duration(d) * 24 * time.Hour)

		// Ask for UUID
		kb := tgBotKeyboardUUID()
		msg := tgbotapi.NewMessage(msg.Chat.ID, "🔑 Choose UUID Mode:")
		msg.ReplyMarkup = kb
		b.API.Send(msg)
		session.State = WaitUUIDOption

	case WaitUUIDManual:
		session.TempUser.UUID = msg.Text
		b.finalizeCreateUser(msg.Chat.ID, session)
	}
}

func (b *Bot) handleCallback(cq *tgbotapi.CallbackQuery) {
	b.API.Send(tgbotapi.NewCallback(cq.ID, ""))

	session := b.getSession(cq.From.ID)
	data := cq.Data
	chatID := cq.Message.Chat.ID

	if data == "back" {
		session.State = Idle
		b.sendMenu(chatID)
		return
	}

	switch session.State {
	case Idle:
		if data == "create" {
			session.State = WaitUsername
			b.sendMessage(chatID, "🆕 Enter Username:")
		} else if data == "status" {
			// FIXED: GetActiveInbound returns 3 values (tag, port, error)
			inb, port, err := core.GetActiveInbound()
			if err != nil {
				b.sendMessage(chatID, "Error: "+err.Error())
			} else {
				b.sendMessage(chatID, fmt.Sprintf("System Status:\nInbound: %s\nPort: %d", inb, port))
			}
		}
	case WaitUUIDOption:
		if data == "auto" {
			session.TempUser.UUID = core.GenerateUUID()
			b.finalizeCreateUser(chatID, session)
		} else if data == "manual" {
			session.State = WaitUUIDManual
			b.sendMessage(chatID, "✏️ Enter custom UUID:")
		}
	}
}

func (b *Bot) finalizeCreateUser(chatID int64, session *UserSession) {
	// Finish
	// FIXED: GetActiveInbound returns 3 values
	proto, _, err := core.GetActiveInbound()
	if err != nil {
		b.sendMessage(chatID, "❌ Error getting protocol: "+err.Error())
		session.State = Idle
		b.sendMenu(chatID)
		return
	}
	session.TempUser.Protocol = proto

	err = core.SaveClient(session.TempUser)
	if err != nil {
		b.sendMessage(chatID, "❌ Error saving: "+err.Error())
	} else {
		core.SyncConfig()
		core.RestartXray()
		b.sendMessage(chatID, fmt.Sprintf("✅ User Created: %s\nUUID: %s", session.TempUser.Username, session.TempUser.UUID))
	}
	session.State = Idle
	b.sendMenu(chatID)
}

func (b *Bot) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	b.API.Send(msg)
}

func (b *Bot) sendMenu(chatID int64) {
	msg := tgbotapi.NewMessage(chatID, "Xray Panel Bot")
	msg.ReplyMarkup = tgBotMenu()
	b.API.Send(msg)
}

func tgBotMenu() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Status", "status"),
			tgbotapi.NewInlineKeyboardButtonData("Create User", "create"),
		),
	)
}

func tgBotKeyboardUUID() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Auto", "auto"),
			tgbotapi.NewInlineKeyboardButtonData("Manual", "manual"),
		),
	)
}
