package core

import (
	"bufio"
	"encoding/base64"
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
	CONFIG_BOT  = "/etc/xray/bot.json"
)

type BotConfig struct {
	BotToken string `json:"bot_token"`
	AdminID  int64  `json:"admin_id"`
}

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

// GetActiveInbound reads the single active inbound
func GetActiveInbound() (string, error) {
	content, err := os.ReadFile(DB_INBOUNDS)
	if err != nil {
		return "", err
	}
	parts := strings.Split(string(content), ";")
	if len(parts) >= 2 {
		return strings.TrimSpace(parts[1]), nil
	}
	return "", fmt.Errorf("invalid inbound db format")
}

// AddInbound saves the inbound to DB and Re-Syncs Config
func AddInbound(protocol, transport string, port int) error {
	// Format: active;protocol-transport;port
	line := fmt.Sprintf("active;%s-%s;%d", protocol, transport, port)
	err := os.WriteFile(DB_INBOUNDS, []byte(line), 0644)
	if err != nil {
		return err
	}
	return SyncConfig()
}

// DeleteInbound clears the DB
func DeleteInbound() error {
	return os.WriteFile(DB_INBOUNDS, []byte(""), 0644)
}

// SyncConfig reads clients.db and updates config.json
func SyncConfig() error {
	clients, err := LoadClients()
	if err != nil {
		return err
	}

	// Read Config Logic:
	// We should probably start from a template or just modify the existing one.
	// But `menu.sh` used `jq` to ADD to `.inbounds`.
	// Here we will REBUILD the relevant inbound.

	// 1. Read existing config to keep outbounds/routing
	configFile, err := os.ReadFile(CONFIG_XRAY)
	if err != nil {
		return err
	}
	var conf XrayConfig
	if err := json.Unmarshal(configFile, &conf); err != nil {
		return err // Or create new default? For now assume valid config exists
	}

	// 2. Determine Active Inbound
	activeStr, err := GetActiveInbound()
	// activeStr is "PROTOCOL-TRANSPORT" e.g. "VLESS-XTLS"

	if err == nil && activeStr != "" {
		parts := strings.Split(activeStr, "-")
		if len(parts) == 2 {
			proto := strings.ToLower(parts[0]) // vless
			trans := strings.ToLower(parts[1]) // xtls
			tag := fmt.Sprintf("%s-%s", proto, trans)

			// Create the Inbound Struct
			// Create base settings with Clients
			settings := InboundSettings{
				Clients: []XrayClient{},
			}

			// Protocol Specific Settings
			if proto == "vless" {
				settings.Decryption = "none"
				settings.Fallbacks = []Fallback{
					{Dest: 80, Xver: 0},
				}
				if trans == "xtls" {
					settings.Fallbacks = []Fallback{{Dest: 80, Xver: 1}}
				}
			} else if proto == "trojan" {
				settings.Fallbacks = []Fallback{
					{Dest: 80, Xver: 0},
				}
				// Trojan doesn't use "decryption"
			}
			// VMess defaults (no decryption, no fallbacks usually unless specific setup)

			// Create the Inbound Struct
			newInbound := Inbound{
				Tag:      tag,
				Port:     443,
				Protocol: proto,
				Settings: settings,
				StreamSettings: StreamSettings{
					Security: "tls",
					TLSSettings: &TLSSettings{
						Certificates: []Certificate{
							{CertificateFile: "/etc/xray/xray.crt", KeyFile: "/etc/xray/xray.key"},
						},
					},
				},
				Sniffing: &Sniffing{
					Enabled:      true,
					DestOverride: []string{"http", "tls", "quic", "fakedns"},
				},
			}

			// Transport Specifics
			if trans == "xtls" {
				newInbound.StreamSettings.Network = "tcp"
				newInbound.StreamSettings.TLSSettings.Alpn = []string{"h2", "http/1.1"}
			} else if trans == "ws" {
				newInbound.StreamSettings.Network = "ws"
				newInbound.StreamSettings.WSSettings = &WSSettings{
					Path: fmt.Sprintf("/%s-%s", proto, trans),
				}
				// WS doesn't usually use Fallback inside the streamSettings, but the inbound itself supports it.
			} else if trans == "grpc" {
				newInbound.StreamSettings.Network = "grpc"
				newInbound.StreamSettings.GRPCSettings = &GRPCSettings{
					ServiceName: fmt.Sprintf("%s-%s", proto, trans),
					MultiMode:   true,
				}
			}

			// 3. Populate Clients
			for _, c := range clients {
				// Only add if client protocol matches active inbound
				if c.Protocol == activeStr && !c.IsExpired { // Using IsExpired check to filter
					xc := XrayClient{
						Email: c.Username,
						ID:    c.UUID,
						Level: 0,
					}
					// Special handling
					if proto == "trojan" {
						xc.Password = c.UUID // Trojan uses password field
						xc.ID = ""
					}
					if trans == "xtls" {
						xc.Flow = "xtls-rprx-vision"
					}
					newInbound.Settings.Clients = append(newInbound.Settings.Clients, xc)
				}
			}

			// 4. Update Config.Inbounds
			// Remove any existing inbound with port 443 or same tag to avoid conflict
			var cleanedInbounds []Inbound
			for _, inb := range conf.Inbounds {
				if inb.Port != 443 && inb.Tag != tag {
					cleanedInbounds = append(cleanedInbounds, inb)
				}
			}
			// Add our new one
			cleanedInbounds = append(cleanedInbounds, newInbound)
			conf.Inbounds = cleanedInbounds
		}
	} else {
		// No active inbound? user might have deleted it.
		// Remove port 443 inbounds.
		var cleanedInbounds []Inbound
		for _, inb := range conf.Inbounds {
			if inb.Port != 443 {
				cleanedInbounds = append(cleanedInbounds, inb)
			}
		}
		conf.Inbounds = cleanedInbounds
	}

	// Write Config
	newConfig, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(CONFIG_XRAY, newConfig, 0644)
}

// GenerateLink creates the sharing link for a client
func GenerateLink(c Client, domain string) string {
	// Format: Protocol-Transport e.g. "VLESS-XTLS"
	parts := strings.Split(c.Protocol, "-")
	if len(parts) < 2 {
		return ""
	}
	proto := strings.ToLower(parts[0])
	trans := strings.ToLower(parts[1])
	uuid := c.UUID
	port := "443"

	if proto == "vless" {
		if trans == "xtls" {
			return fmt.Sprintf("vless://%s@%s:%s?security=tls&encryption=none&flow=xtls-rprx-vision&type=tcp&sni=%s&alpn=h2,http/1.1#%s",
				uuid, domain, port, domain, c.Username)
		} else if trans == "ws" {
			path := fmt.Sprintf("/%s-%s", proto, trans)
			return fmt.Sprintf("vless://%s@%s:%s?security=tls&encryption=none&type=ws&path=%s&host=%s&sni=%s&alpn=h2,http/1.1#%s",
				uuid, domain, port, path, domain, domain, c.Username)
		} else if trans == "grpc" {
			service := fmt.Sprintf("%s-%s", proto, trans)
			return fmt.Sprintf("vless://%s@%s:%s?security=tls&encryption=none&type=grpc&serviceName=%s&mode=multi&sni=%s&alpn=h2#%s",
				uuid, domain, port, service, domain, c.Username)
		}
	} else if proto == "vmess" {
		// VMess uses JSON base64
		vmessConfig := map[string]string{
			"v": "2", "ps": c.Username, "add": domain, "port": port, "id": uuid,
			"aid": "0", "scy": "auto", "net": trans, "type": "none", "tls": "tls", "sni": domain,
		}
		if trans == "ws" {
			vmessConfig["path"] = fmt.Sprintf("/%s-%s", proto, trans)
			vmessConfig["host"] = domain
		} else if trans == "grpc" {
			vmessConfig["path"] = fmt.Sprintf("%s-%s", proto, trans) // ServiceName
		}
		jsonBytes, _ := json.Marshal(vmessConfig)
		b64 := base64.StdEncoding.EncodeToString(jsonBytes)
		return fmt.Sprintf("vmess://%s", b64)
	} else if proto == "trojan" {
		if trans == "ws" {
			path := fmt.Sprintf("/%s-%s", proto, trans)
			return fmt.Sprintf("trojan://%s@%s:%s?security=tls&type=ws&path=%s&host=%s&sni=%s&alpn=h2,http/1.1#%s",
				uuid, domain, port, path, domain, domain, c.Username)
		} else if trans == "grpc" {
			service := fmt.Sprintf("%s-%s", proto, trans)
			return fmt.Sprintf("trojan://%s@%s:%s?security=tls&type=grpc&serviceName=%s&mode=multi&sni=%s&alpn=h2#%s",
				uuid, domain, port, service, domain, c.Username)
		}
	}
	return ""
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

// IsUserOnline checks the access log for recent activity
func IsUserOnline(email string) bool {
	// Grep /var/log/xray/access.log for email in last 30s
	// This is expensive if log is huge. We assume log rotation or tailored grep.
	// grep -c "$email" /var/log/xray/access.log | tail -n 50 (last lines)
	// We'll read the file directly or use exec (easier for grep).

	cmd := exec.Command("bash", "-c", fmt.Sprintf("tail -n 300 /var/log/xray/access.log | grep '%s' | grep -v 'rejected' | wc -l", email))
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	count, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return count > 0
}

// LoadBotConfig reads the bot configuration
func LoadBotConfig() (BotConfig, error) {
	var cfg BotConfig
	file, err := os.ReadFile(CONFIG_BOT)
	if err != nil {
		return cfg, err
	}
	err = json.Unmarshal(file, &cfg)
	return cfg, err
}

// SaveBotConfig writes the bot configuration
func SaveBotConfig(token string, adminID int64) error {
	cfg := BotConfig{
		BotToken: token,
		AdminID:  adminID,
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(CONFIG_BOT, data, 0644)
}

// RemoveBotConfig deletes the bot config file
func RemoveBotConfig() error {
	return os.Remove(CONFIG_BOT)
}
