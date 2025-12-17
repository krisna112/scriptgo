package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/krisna112/scriptxray/go_panel/pkg/core"
)

func RunMenu() {
	reader := bufio.NewReader(os.Stdin)

	for {
		clearScreen()
		printHeader()
		fmt.Println(" [1]  List Users (Details & QR)")
		fmt.Println(" [2]  Add User")
		fmt.Println(" [3]  Delete User")
		fmt.Println(" [4]  Edit User")
		fmt.Println(" ")
		fmt.Println(" [5]  Add/Replace Inbound")
		fmt.Println(" [6]  Delete Inbound")
		fmt.Println(" ")
		fmt.Println(" [7]  System Status & Info")
		fmt.Println(" [8]  Restart Services")
		fmt.Println(" [9]  Update Xray Core")
		fmt.Println(" [10] Enable BBR (Speed Up)")
		fmt.Println(" [11] Bot Config (Start/Stop)")
		fmt.Println(" [12] Update Script (Force)")
		fmt.Println(" [13] User Monitor (Traffic/Status)")
		fmt.Println(" [14] Debug Center (Logs & Error)") // MENU BARU
		fmt.Println(" ")
		fmt.Println(" [x]  Exit")
		fmt.Print("\n Select Option: ")

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		switch input {
		case "1":
			listUsers()
		case "2":
			addUser(reader)
		case "3":
			deleteUser(reader)
		case "4":
			editUser(reader)
		case "5":
			addInbound(reader)
		case "6":
			if err := core.DeleteInbound(); err == nil {
				core.SyncConfig()
				core.RestartXray()
				fmt.Println("Inbound Deleted!")
			}
			waitForKey(reader)
		case "7":
			printSystemStatus()
		case "8":
			fmt.Println("Restarting...")
			core.RestartXray()
			core.ManageSystemdService("xray-panel", "restart")
			waitForKey(reader)
		case "9":
			fmt.Println("Updating Xray Core...")
			if err := core.UpdateXrayCore(); err != nil {
				fmt.Printf("Error: %v\n", err)
			} else {
				fmt.Println("Xray Updated!")
				core.RestartXray()
			}
			waitForKey(reader)
		case "10":
			fmt.Println("Enabling BBR...")
			if err := core.EnableBBR(); err != nil {
				fmt.Printf("Error: %v\n", err)
			} else {
				fmt.Println("BBR Enabled!")
			}
			waitForKey(reader)
		case "11":
			manageBot(reader)
		case "12":
			updateScript(reader)
		case "13":
			monitorUsers(reader)
		case "14":
			debugMenu(reader) // PANGGIL FUNGSI DEBUG
		case "x", "X":
			return
		}
	}
}

// --- FUNGSI DEBUG BARU ---
func debugMenu(r *bufio.Reader) {
	for {
		clearScreen()
		fmt.Println("==================================================")
		fmt.Println("             DEBUG CENTER (LOGS)                  ")
		fmt.Println("==================================================")
		fmt.Println(" [1] View Xray Error Log (Last 50 lines)")
		fmt.Println(" [2] View Xray Access Log (Last 50 lines)")
		fmt.Println(" [3] View Panel & Bot Log (Systemd Journal)")
		fmt.Println(" [4] Check Service Status (Detailed)")
		fmt.Println(" [5] Test Config Syntax (xray run -test)")
		fmt.Println(" ")
		fmt.Println(" [x] Back to Main Menu")
		fmt.Print("\n Select Log: ")

		input, _ := r.ReadString('\n')
		input = strings.TrimSpace(input)

		switch input {
		case "1":
			fmt.Println("\n--- Xray Error Log ---")
			runCommand("tail", "-n", "50", "/var/log/xray/error.log")
			waitForKey(r)
		case "2":
			fmt.Println("\n--- Xray Access Log ---")
			runCommand("tail", "-n", "50", "/var/log/xray/access.log")
			waitForKey(r)
		case "3":
			fmt.Println("\n--- Panel & Bot Log ---")
			runCommand("journalctl", "-u", "xray-panel", "-n", "50", "--no-pager")
			waitForKey(r)
		case "4":
			fmt.Println("\n--- Service Status ---")
			runCommand("systemctl", "status", "xray", "xray-panel", "--no-pager")
			waitForKey(r)
		case "5":
			fmt.Println("\n--- Config Syntax Check ---")
			binPath := "/usr/local/bin/xray"
			if _, err := os.Stat(binPath); os.IsNotExist(err) {
				binPath = "/usr/bin/xray"
			}
			runCommand(binPath, "run", "-test", "-confdir", "/usr/local/etc/xray")
			waitForKey(r)
		case "x", "X":
			return
		}
	}
}

func runCommand(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("Command error: %v\n", err)
	}
}
// -------------------------

func monitorUsers(r *bufio.Reader) {
	for {
		clearScreen()
		fmt.Println("==================================================")
		fmt.Println("             LIVE USER MONITOR                    ")
		fmt.Println("==================================================")
		fmt.Printf("%-12s | %-10s | %-10s \n", "Username", "Usage/Quota", "Status")
		fmt.Println("--------------------------------------------------")

		clients, _ := core.LoadClients()
		for _, c := range clients {
			usedGB := float64(c.Used) / 1024 / 1024 / 1024
			usageStr := fmt.Sprintf("%.2f/%.2f", usedGB, c.Quota)

			status := "OFFLINE"
			color := ""

			if c.IsExpired {
				status = "EXPIRED"
				color = "\033[31m" // Red
			} else if c.Used >= (c.Quota * 1024 * 1024 * 1024) {
				status = "LIMIT"
				color = "\033[31m" // Red
			} else {
				if core.IsUserOnline(c.Username) {
					status = "ONLINE"
					color = "\033[32m" // Green
				}
			}

			fmt.Printf("%-12s | %-10s | %s%s\033[0m\n", c.Username, usageStr, color, status)
		}
		fmt.Println("==================================================")
		fmt.Println(" [Enter] Refresh  [x] Back to Menu")
		fmt.Print(" Select: ")

		input, _ := r.ReadString('\n')
		input = strings.TrimSpace(input)
		if strings.ToLower(input) == "x" {
			return
		}
	}
}

func updateScript(r *bufio.Reader) {
	fmt.Println("\n--- Update Script (Force) ---")
	fmt.Println("This will re-download the latest source code and reinstall.")
	fmt.Print("Continue? (y/n): ")
	confirm, _ := r.ReadString('\n')
	if strings.TrimSpace(strings.ToLower(confirm)) != "y" {
		return
	}

	fmt.Println("🚀 Updating...")

	cmdStr := `
	cd /root
	rm -rf scriptvpsgo_update
	GIT_TERMINAL_PROMPT=0 git clone https://github.com/krisna112/scriptgo.git scriptvpsgo_update
	cd scriptvpsgo_update
	chmod +x setup_go.sh
	./setup_go.sh
	`

	cmd := exec.Command("bash", "-c", cmdStr)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Printf("❌ Update Failed: %v\n", err)
	} else {
		fmt.Println("✅ Update & Re-install Complete!")
		fmt.Println("Please restart the menu command.")
		os.Exit(0)
	}
	waitForKey(r)
}

func clearScreen() {
	fmt.Print("\033[H\033[2J")
}

func printHeader() {
	fmt.Println("==================================================")
	fmt.Println("           XRAY PANEL - GO EDITION")
	fmt.Println("==================================================")

	out, _ := exec.Command("systemctl", "is-active", "xray").Output()
	xrayStatus := strings.TrimSpace(string(out))
	xrayColor := "[31m" // Red
	if xrayStatus == "active" {
		xrayStatus = "RUNNING"
		xrayColor = "[32m" // Green
	} else {
		xrayStatus = "STOPPED"
	}

	outPanel, _ := exec.Command("systemctl", "is-active", "xray-panel").Output()
	panelStatus := strings.TrimSpace(string(outPanel))
	panelColor := "[31m"
	if panelStatus == "active" {
		panelStatus = "RUNNING"
		panelColor = "[32m"
	} else {
		panelStatus = "STOPPED"
	}

	botStatus := "STOPPED"
	botColor := "[31m"
	if panelStatus == "RUNNING" {
		botCfg, err := core.LoadBotConfig()
		if err == nil && botCfg.BotToken != "" {
			botStatus = "RUNNING"
			botColor = "[32m"
		}
	}

	fmt.Printf(" Xray Core:   \033%s%s\033[0m\n", xrayColor, xrayStatus)
	fmt.Printf(" Web Panel:   \033%s%s\033[0m\n", panelColor, panelStatus)
	fmt.Printf(" Telegram Bot:\033%s%s\033[0m\n", botColor, botStatus)

	// FIX: Handle 3 return values (string, int, error)
	activeInb, port, _ := core.GetActiveInbound()
	if activeInb == "" {
		fmt.Printf(" Inbound:     None\n")
	} else {
		fmt.Printf(" Inbound:     %s (Port: %d)\n", activeInb, port)
	}
	fmt.Println("==================================================")
}

func waitForKey(r *bufio.Reader) {
	fmt.Print("\nPress Enter to continue...")
	r.ReadString('\n')
}

func listUsers() {
	clients, _ := core.LoadClients()
	fmt.Println("\n--- User List ---")
	for i, c := range clients {
		status := "OK"
		if c.IsExpired {
			status = "EXPIRED"
		}
		usedGB := float64(c.Used) / 1024 / 1024 / 1024
		fmt.Printf("[%d] %-12s | %.2f/%.2f GB | %s | %s\n", i+1, c.Username, usedGB, c.Quota, status, c.Expiry.Format("2006-01-02"))
	}
	fmt.Print("\nPress Enter...")
	bufio.NewReader(os.Stdin).ReadString('\n')
}

func addUser(r *bufio.Reader) {
	// FIX: Handle 3 return values
	inbound, _, err := core.GetActiveInbound()
	if err != nil || inbound == "" {
		fmt.Println("❌ Error: No active inbound found!")
		fmt.Println("Please create an inbound first (Option 5).")
		waitForKey(r)
		return
	}

	fmt.Printf("Adding user to Inbound: %s\n", inbound)
	fmt.Print("Username: ")
	user, _ := r.ReadString('\n')
	user = strings.TrimSpace(user)

	fmt.Print("Quota (GB): ")
	qStr, _ := r.ReadString('\n')
	var quota float64
	fmt.Sscanf(strings.TrimSpace(qStr), "%f", &quota)

	fmt.Print("Days: ")
	dStr, _ := r.ReadString('\n')
	var days int
	fmt.Sscanf(strings.TrimSpace(dStr), "%d", &days)

	client := core.Client{
		Username: user,
		Quota:    quota,
		Expiry:   time.Now().Add(time.Duration(days) * 24 * time.Hour),
		Protocol: inbound,
		UUID:     core.GenerateUUID(),
	}

	if err := core.SaveClient(client); err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		core.SyncConfig()
		core.RestartXray()
		fmt.Println("\n✅ User Created!")

		domainBytes, _ := os.ReadFile("/root/domain")
		domain := strings.TrimSpace(string(domainBytes))
		if domain == "" {
			domain = core.GetHostname()
		}

		link := core.GenerateLink(client, domain)
		fmt.Println("\n🔗 Xray Link:")
		fmt.Println(link)

		if link != "" {
			fmt.Println("\n📱 QR Code:")
			cmd := exec.Command("qrencode", "-t", "ANSIUTF8", link)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Run()
		}
	}
	waitForKey(r)
}

func editUser(r *bufio.Reader) {
	fmt.Print("Username to edit: ")
	user, _ := r.ReadString('\n')
	user = strings.TrimSpace(user)

	clients, _ := core.LoadClients()
	var found *core.Client
	for i := range clients {
		if clients[i].Username == user {
			found = &clients[i]
			break
		}
	}

	if found == nil {
		fmt.Println("User not found!")
		waitForKey(r)
		return
	}

	fmt.Printf("Current Quota: %.2f GB. New Quota (Enter to keep): ", found.Quota)
	qStr, _ := r.ReadString('\n')
	qStr = strings.TrimSpace(qStr)
	if qStr != "" {
		fmt.Sscanf(qStr, "%f", &found.Quota)
	}

	fmt.Printf("Current Expiry: %s. Add Days (e.g. 30, or 0 to keep): ", found.Expiry.Format("2006-01-02"))
	dStr, _ := r.ReadString('\n')
	dStr = strings.TrimSpace(dStr)
	var days int
	if dStr != "" {
		fmt.Sscanf(dStr, "%d", &days)
		if days > 0 {
			found.Expiry = found.Expiry.Add(time.Duration(days) * 24 * time.Hour)
			found.IsExpired = false
		}
	}

	err := core.UpdateClient(user, func(target *core.Client) {
		target.Quota = found.Quota
		target.Expiry = found.Expiry
		target.IsExpired = found.IsExpired
	})

	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		core.SyncConfig()
		core.RestartXray()
		fmt.Println("User Updated!")
	}
	waitForKey(r)
}

func addInbound(r *bufio.Reader) {
	fmt.Println("\n--- Add/Replace Inbound ---")
	fmt.Println("Select Protocol:")
	fmt.Println("1. VLESS")
	fmt.Println("2. VMESS")
	fmt.Println("3. TROJAN")
	fmt.Print("Choice (default 1): ")
	protoStr, _ := r.ReadString('\n')
	protoStr = strings.TrimSpace(protoStr)

	protocol := "vless"
	switch protoStr {
	case "2":
		protocol = "vmess"
	case "3":
		protocol = "trojan"
	}

	fmt.Println("\nSelect Transport:")
	fmt.Println("1. XTLS-Vision (TCP)")
	fmt.Println("2. WebSocket (WS)")
	fmt.Println("3. gRPC")
	fmt.Print("Choice (default 1): ")
	transStr, _ := r.ReadString('\n')
	transStr = strings.TrimSpace(transStr)

	transport := "xtls"
	switch transStr {
	case "2":
		transport = "ws"
	case "3":
		transport = "grpc"
	}

	if protocol == "vmess" && transport == "xtls" {
		fmt.Println("Warning: VMess + XTLS is not recommended/standard. Switching to WS.")
		transport = "ws"
	}

	// FIX: INPUT PORT MANUAL AGAR TIDAK BENTROK
	fmt.Print("\nEnter Port (Default 443): ")
	portStr, _ := r.ReadString('\n')
	portStr = strings.TrimSpace(portStr)
	port := 443
	if portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err == nil {
			port = p
		}
	}

	fmt.Printf("\nCreating %s-%s on Port %d...\n", protocol, transport, port)

	err := core.AddInbound(protocol, transport, port)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Println("Inbound Created & Xray Restarted!")
		core.RestartXray()
	}
	waitForKey(r)
}

func deleteUser(r *bufio.Reader) {
	fmt.Print("Username to delete: ")
	user, _ := r.ReadString('\n')
	user = strings.TrimSpace(user)

	if err := core.DeleteClient(user); err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		core.SyncConfig()
		core.RestartXray()
		fmt.Println("User Deleted!")
	}
	waitForKey(r)
}

func printSystemStatus() {
	fmt.Println("\n--- System Status ---")
	fmt.Printf("Hostname: %s\n", core.GetHostname())

	out, _ := exec.Command("uptime", "-p").Output()
	fmt.Printf("Uptime: %s\n", strings.TrimSpace(string(out)))

	out, _ = exec.Command("free", "-h").Output()
	lines := strings.Split(string(out), "\n")
	if len(lines) > 1 {
		fmt.Printf("Memory:\n%s\n", lines[1])
	}

	bufio.NewReader(os.Stdin).ReadString('\n')
}

func manageBot(r *bufio.Reader) {
	fmt.Println("\n--- Bot Management ---")
	fmt.Println(" [1] Start Bot (Set Token & ID)")
	fmt.Println(" [2] Stop Bot (Disable)")
	fmt.Print("Select: ")
	sel, _ := r.ReadString('\n')
	sel = strings.TrimSpace(sel)

	switch sel {
	case "1":
		fmt.Print("Enter Bot Token: ")
		token, _ := r.ReadString('\n')
		token = strings.TrimSpace(token)

		fmt.Print("Enter Admin ID (Numeric): ")
		idStr, _ := r.ReadString('\n')
		idStr = strings.TrimSpace(idStr)
		adminID, _ := strconv.ParseInt(idStr, 10, 64)

		if token == "" || adminID == 0 {
			fmt.Println("❌ Invalid Input!")
			return
		}

		if err := core.SaveBotConfig(token, adminID); err != nil {
			fmt.Printf("❌ Failed to save config: %v\n", err)
		} else {
			fmt.Println("✅ Config Saved! Restarting Panel to activate Bot...")
			core.ManageSystemdService("xray-panel", "restart")
		}

	case "2":
		if err := core.RemoveBotConfig(); err != nil {
			fmt.Println("⚠️  Bot is already disabled or file missing.")
		}
		fmt.Println("✅ Bot Config Removed. Restarting Panel...")
		core.ManageSystemdService("xray-panel", "restart")
	}
	waitForKey(r)
}
