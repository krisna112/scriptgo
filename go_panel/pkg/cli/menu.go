package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
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
		case "x", "X":
			return
		}
	}
}

func clearScreen() {
	fmt.Print("\033[H\033[2J")
}

func printHeader() {
	fmt.Println("==================================================")
	fmt.Println("           XRAY PANEL - GO EDITION")
	fmt.Println("==================================================")

	// Dynamic status: Check socket or process
	// systemctl is-active xray (returns active or inactive)
	// We should trust it if installed.
	out, _ := exec.Command("systemctl", "is-active", "xray").Output()
	status := strings.TrimSpace(string(out))
	color := ""
	if status == "active" {
		status = "RUNNING"
		color = "[32m" // Green
	} else {
		status = "STOPPED"
		color = "[31m" // Red
	}
	fmt.Printf(" Xray Core: \033%s%s\033[0m\n", color, status)

	activeInb, _ := core.GetActiveInbound()
	if activeInb == "" {
		activeInb = "None"
	}
	fmt.Printf(" Inbound:   %s\n", activeInb)
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
		// Calculate used
		usedGB := float64(c.Used) / 1024 / 1024 / 1024
		fmt.Printf("[%d] %-12s | %.2f/%.2f GB | %s | %s\n", i+1, c.Username, usedGB, c.Quota, status, c.Expiry.Format("2006-01-02"))
	}
	fmt.Print("\nPress Enter...")
	bufio.NewReader(os.Stdin).ReadString('\n')
}

func addUser(r *bufio.Reader) {
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

	inbound, _ := core.GetActiveInbound()

	client := core.Client{
		Username: user,
		Quota:    quota,
		Expiry:   time.Now().Add(time.Duration(days) * 24 * time.Hour),
		Protocol: inbound,
		UUID:     core.GenerateRandomID(16), // Using simplistic random for now, ideally UUIDv4
	}

	if err := core.SaveClient(client); err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		core.SyncConfig()
		core.RestartXray()
		fmt.Println("User Created!")
	}
	waitForKey(r)
}

func editUser(r *bufio.Reader) {
	fmt.Print("Username to edit: ")
	user, _ := r.ReadString('\n')
	user = strings.TrimSpace(user)

	clients, _ := core.LoadClients()
	var found *core.Client
	for i := range clients { // Use range with index to get a modifiable reference
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
			found.IsExpired = false // Reactivate if extending
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

	fmt.Print("\nPort (Enter for 443): ")
	portStr, _ := r.ReadString('\n')
	portStr = strings.TrimSpace(portStr)
	port := 443
	if portStr != "" {
		fmt.Sscanf(portStr, "%d", &port)
	}

	fmt.Printf("Creating %s-%s on port %d...\n", protocol, transport, port)

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

	// Simple uptime
	out, _ := exec.Command("uptime", "-p").Output()
	fmt.Printf("Uptime: %s\n", strings.TrimSpace(string(out)))

	// RAM
	out, _ = exec.Command("free", "-h").Output()
	lines := strings.Split(string(out), "\n")
	if len(lines) > 1 {
		fmt.Printf("Memory:\n%s\n", lines[1])
	}

	bufio.NewReader(os.Stdin).ReadString('\n')
}

func manageBot(r *bufio.Reader) {
	fmt.Println("\n--- Bot Management ---")
	fmt.Println(" [1] Start Bot")
	fmt.Println(" [2] Stop Bot")
	fmt.Print("Select: ")
	sel, _ := r.ReadString('\n')
	sel = strings.TrimSpace(sel)

	switch sel {
	case "1":
		core.ManageSystemdService("xray-panel", "restart") // Bot is part of panel now
		fmt.Println("Panel (with Bot) Restarted.")
	case "2":
		fmt.Println("To disable bot, remove token from service file or config.")
	}
	waitForKey(r)
}
