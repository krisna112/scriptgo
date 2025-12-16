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

	// Calc stats
	var totalBytes float64
	online := 0
	
	// FIX: Loop updated untuk cek status online real-time
	for i := range clients {
		totalBytes += clients[i].Used
		
		// Cek status online manual disini karena LoadClients tidak melakukannya
		if core.IsUserOnline(clients[i].Username) {
			clients[i].IsOnline = true // Update struct agar di HTML terlihat hijau
			online++
		}
	}

	data := DashboardData{
		Users:           clients,
		CPU:             core.GetSystemCPU(), // Pastikan fungsi ini ada/mock
		RAM:             core.GetSystemRAM(), // Pastikan fungsi ini ada/mock
		TotalUsage:      core.FormatBytes(totalBytes),
		OnlineCount:     online,
		XrayStatus:      core.IsServiceRunning("xray"), // Gunakan fungsi helper
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
	inbound, _ := core.GetActiveInbound()
	Render(w, "form.html", map[string]interface{}{
		"action":  "Add",
		"inbound": inbound,
	})
}

func AddUserPostHandler(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	quotaStr := r.FormValue("quota")
	daysStr := r.FormValue("days")
	// uuidMode := r.FormValue("uuid_mode")
	// customUuid := r.FormValue("custom_uuid")

	quota, _ := strconv.ParseFloat(quotaStr, 64)
	days, _ := strconv.Atoi(daysStr)

	// Create Client
	newClient := core.Client{
		Username: username,
		Quota:    quota,
		Used:     0,
		Expiry:   time.Now().Add(time.Duration(days) * 24 * time.Hour),
		Protocol: "VLESS-XTLS", // Needs to be dynamic based on inbound
		UUID:     core.GenerateUUID(),
	}

	// Get Inbound Protocol
	inb, _ := core.GetActiveInbound()
	if inb != "" {
		newClient.Protocol = inb
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
	// r.PathValue only in Go 1.22+
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
