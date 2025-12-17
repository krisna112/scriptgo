package web

import (
	"net/http"
	"strconv"
	"time"

	"github.com/krisna112/scriptxray/go_panel/pkg/core"
)

// Dashboard Data
type DashboardData struct {
	Users           []core.Client
	CPU             float64
	RAM             float64
	TotalUsage      string
	OnlineCount     int
	XrayStatus      bool
	InstallDuration string
}

func DashboardHandler(w http.ResponseWriter, r *http.Request) {
	clients, _ := core.LoadClients()

	var totalBytes float64
	online := 0
	
	for i := range clients {
		totalBytes += clients[i].Used
		if core.IsUserOnline(clients[i].Username) {
			clients[i].IsOnline = true
			online++
		}
	}

	data := DashboardData{
		Users:           clients,
		CPU:             0.0, 
		RAM:             0.0, 
		TotalUsage:      core.FormatBytes(totalBytes),
		OnlineCount:     online,
		XrayStatus:      core.IsServiceRunning("xray"),
		InstallDuration: "Unknown",
	}

	Render(w, "dashboard.html", data)
}

func LoginHandler(w http.ResponseWriter, r *http.Request) {
	Render(w, "login.html", nil)
}

func LoginPostHandler(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")

	creds := core.GetAdminCreds()

	if username == creds.Username && password == creds.Password {
		token := CreateSession()
		http.SetCookie(w, &http.Cookie{
			Name:  "session_token",
			Value: token,
			Path:  "/",
		})
		http.Redirect(w, r, "/", http.StatusFound)
	} else {
		Render(w, "login.html", map[string]string{"error": "Invalid Credentials"})
	}
}

func LogoutHandler(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:   "session_token",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.Redirect(w, r, "/login", http.StatusFound)
}

func AddUserFormHandler(w http.ResponseWriter, r *http.Request) {
	inbound, _, _ := core.GetActiveInbound() // Ignore port and error for display only logic if needed, or pass port too
	Render(w, "form.html", map[string]interface{}{
		"action":  "Add",
		"inbound": inbound,
	})
}

func AddUserPostHandler(w http.ResponseWriter, r *http.Request) {
	inb, _, err := core.GetActiveInbound()
	if err != nil || inb == "" {
		http.Error(w, "CRITICAL ERROR: No Active Inbound created yet! Please create Inbound via CLI Menu first.", http.StatusPreconditionRequired)
		return
	}

	username := r.FormValue("username")
	quotaStr := r.FormValue("quota")
	daysStr := r.FormValue("days")

	quota, _ := strconv.ParseFloat(quotaStr, 64)
	days, _ := strconv.Atoi(daysStr)

	newClient := core.Client{
		Username: username,
		Quota:    quota,
		Used:     0,
		Expiry:   time.Now().Add(time.Duration(days) * 24 * time.Hour),
		Protocol: inb,
		UUID:     core.GenerateUUID(),
	}

	if err := core.SaveClient(newClient); err != nil {
		http.Error(w, "Failed to save: "+err.Error(), http.StatusInternalServerError)
		return
	}

	core.SyncConfig()
	core.RestartXray()

	http.Redirect(w, r, "/", http.StatusFound)
}

func DeleteUserHandler(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")

	core.DeleteClient(username)
	core.SyncConfig()
	core.RestartXray()

	http.Redirect(w, r, "/", http.StatusFound)
}

func SettingsHandler(w http.ResponseWriter, r *http.Request) {
	creds := core.GetAdminCreds()
	msg := ""
	if r.Method == "POST" {
		user := r.FormValue("username")
		pass := r.FormValue("password")
		core.SaveAdminCreds(user, pass)
		msg = "Saved!"
		creds = core.AdminCreds{Username: user, Password: pass}
	}

	Render(w, "settings.html", map[string]interface{}{
		"creds":   creds,
		"success": msg,
	})
}
