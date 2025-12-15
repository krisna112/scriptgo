package core

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var (
	DB_CLIENTS  = "/etc/xray/clients.db"
	DB_INBOUNDS = "/etc/xray/inbounds.db"
	CONFIG_XRAY = "/usr/local/etc/xray/config.json"
)

// SetPaths allows overriding paths for testing or different environments
func SetPaths(clients, inbounds, config string) {
	DB_CLIENTS = clients
	DB_INBOUNDS = inbounds
	CONFIG_XRAY = config
}

// LoadClients reads the semicolon-separated clients database
func LoadClients() ([]Client, error) {
	var clients []Client

	file, err := os.Open(DB_CLIENTS)
	if os.IsNotExist(err) {
		return clients, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ";")
		if len(parts) < 6 {
			continue
		}

		quota, _ := strconv.ParseFloat(parts[1], 64)
		used, _ := strconv.ParseFloat(parts[2], 64)

		expTime, err := time.Parse("2006-01-02 15:04:05", parts[3])
		if err != nil {
			// Handle fallback or error
			expTime = time.Now()
		}

		client := Client{
			Username:  parts[0],
			Quota:     quota,
			Used:      used,
			Expiry:    expTime,
			Protocol:  parts[4],
			UUID:      parts[5],
			IsExpired: time.Now().After(expTime),
		}
		clients = append(clients, client)
	}

	return clients, scanner.Err()
}

// SaveClient appends a new client to the database
func SaveClient(c Client) error {
	f, err := os.OpenFile(DB_CLIENTS, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	expStr := c.Expiry.Format("2006-01-02 15:04:05")
	line := fmt.Sprintf("%s;%.2f;%.0f;%s;%s;%s\n", c.Username, c.Quota, c.Used, expStr, c.Protocol, c.UUID)

	_, err = f.WriteString(line)
	return err
}

// UpdateClient strictly updates an existing client line
func UpdateClient(username string, modifier func(*Client)) error {
	clients, err := LoadClients()
	if err != nil {
		return err
	}

	found := false
	for i := range clients {
		if clients[i].Username == username {
			modifier(&clients[i])
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("user not found")
	}

	// Rewrite file
	f, err := os.OpenFile(DB_CLIENTS, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, c := range clients {
		expStr := c.Expiry.Format("2006-01-02 15:04:05")
		line := fmt.Sprintf("%s;%.2f;%.0f;%s;%s;%s\n", c.Username, c.Quota, c.Used, expStr, c.Protocol, c.UUID)
		if _, err := f.WriteString(line); err != nil {
			return err
		}
	}
	return nil
}

// DeleteClient removes a client from the DB
func DeleteClient(username string) error {
	clients, err := LoadClients()
	if err != nil {
		return err
	}

	f, err := os.OpenFile(DB_CLIENTS, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, c := range clients {
		if c.Username == username {
			continue
		}
		expStr := c.Expiry.Format("2006-01-02 15:04:05")
		line := fmt.Sprintf("%s;%.2f;%.0f;%s;%s;%s\n", c.Username, c.Quota, c.Used, expStr, c.Protocol, c.UUID)
		if _, err := f.WriteString(line); err != nil {
			return err
		}
	}
	return nil
}

// GetActiveInbound reads the single active inbound/protocol
func GetActiveInbound() (string, error) {
	content, err := os.ReadFile(DB_INBOUNDS)
	if err != nil {
		return "", err
	}
	// Format: port;protocol
	parts := strings.Split(string(content), ";")
	if len(parts) >= 2 {
		return strings.TrimSpace(parts[1]), nil
	}
	return "", fmt.Errorf("invalid inbound db format")
}

// SyncConfig reads clients.db and updates config.json
func SyncConfig() error {
	clients, err := LoadClients()
	if err != nil {
		return err
	}

	// Read Config
	configFile, err := os.ReadFile(CONFIG_XRAY)
	if err != nil {
		return err
	}

	var conf XrayConfig
	if err := json.Unmarshal(configFile, &conf); err != nil {
		return err
	}

	// Clear clients in inbounds
	for i := range conf.Inbounds {
		if conf.Inbounds[i].Settings.Clients != nil {
			conf.Inbounds[i].Settings.Clients = []XrayClient{}
		}
	}

	// Re-populate
	for _, c := range clients {
		// Clean protocol string (e.g. VLESS-XTLS-EXPIRED -> VLESS-XTLS)
		cleanProto := strings.ReplaceAll(c.Protocol, "-EXPIRED", "")
		cleanProto = strings.ReplaceAll(cleanProto, "-DISABLED", "")

		// Derive tag: VLESS-XTLS -> vless-xtls
		parts := strings.Split(cleanProto, "-")
		tag := strings.ToLower(cleanProto)
		if len(parts) > 1 {
			// If format is PROTO-TRANS e.g. VMESS-WS -> vmess-ws
			// The original python logic was specific, let's try to match tag directly first
		}

		// Python logic:
		// if '-' in clean_proto: p, t = clean_proto.split('-'); tag = f"{p.lower()}-{t.lower()}"
		// else: tag = clean_proto.lower()

		// Find inbound
		for i := range conf.Inbounds {
			if conf.Inbounds[i].Tag == tag {
				xc := XrayClient{Email: c.Username}
				if strings.Contains(strings.ToUpper(cleanProto), "TROJAN") {
					xc.Password = c.UUID
				} else {
					xc.ID = c.UUID
					if strings.Contains(strings.ToUpper(cleanProto), "XTLS") {
						xc.Flow = "xtls-rprx-vision"
					}
				}
				conf.Inbounds[i].Settings.Clients = append(conf.Inbounds[i].Settings.Clients, xc)
			}
		}
	}

	// Write Config
	newConfig, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(CONFIG_XRAY, newConfig, 0644)
}

func RestartXray() error {
	cmd := exec.Command("systemctl", "restart", "xray")
	return cmd.Run()
}

// GetTraffic queries Xray API for a specific user and RESETS the counter
func GetTraffic(email string) (int64, int64, error) {
	// xray api stats --server=127.0.0.1:10085 -name 'user>>>email>>>traffic>>>uplink' -reset

	fetch := func(direction string) int64 {
		name := fmt.Sprintf("user>>>%s>>>traffic>>>%s", email, direction)
		// Added "-reset" to fetch and clear, so we can verify exact accumulation in DB
		cmd := exec.Command("xray", "api", "stats", "--server=127.0.0.1:10085", "-name", name, "-reset")
		out, err := cmd.Output()
		if err != nil {
			return 0
		}
		// Output JSON: {"stat":{"name":"...","value":"1234"}}
		var res struct {
			Stat struct {
				Value string `json:"value"`
			} `json:"stat"`
		}
		if err := json.Unmarshal(out, &res); err != nil {
			return 0
		}
		val, _ := strconv.ParseInt(res.Stat.Value, 10, 64)
		return val
	}

	up := fetch("uplink")
	down := fetch("downlink")
	return up, down, nil
}
