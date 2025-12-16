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

	conf := XrayConfig{
		Log: map[string]string{
			"access":   "/var/log/xray/access.log",
			"error":    "/var/log/xray/error.log",
			"loglevel": "warning",
		},
		API: &APIConfig{
			Tag:      "api",
			Services: []string{"HandlerService", "LoggerService", "StatsService"},
		},
		Stats: map[string]string{}, 
		Policy: &PolicyConfig{
			Levels: map[string]LevelPolicy{
				"0": {
					StatsUserUplink:   true,
					StatsUserDownlink: true,
					Handshake:         4,
					ConnIdle:          300,
					UplinkOnly:        2,
					DownlinkOnly:      5,
					BufferSize:        4,
				},
			},
			System: SystemPolicy{
				StatsInboundUplink:   true,
				StatsInboundDownlink: true,
			},
		},
		Inbounds: []Inbound{
			{
				Tag:      "api",
				Port:     10085,
				Protocol: "dokodemo-door",
				Settings: InboundSettings{
					Address: "127.0.0.1",
				},
			},
		},
		Outbounds: []Outbound{
			{Protocol: "freedom", Tag: "direct"},
			{Protocol: "blackhole", Tag: "blocked"},
		},
		Routing: &RoutingConfig{
			Rules: []RoutingRule{
				{Type: "field", InboundTag: []string{"api"}, OutboundTag: "api"},
				{Type: "field", IP: []string{"geoip:private"}, OutboundTag: "blocked"},
			},
		},
	}

	activeStr, err := GetActiveInbound()
	if err == nil && activeStr != "" {
		parts := strings.Split(activeStr, "-")
		if len(parts) == 2 {
			proto := strings.ToLower(parts[0]) 
			trans := strings.ToLower(parts[1]) 
			tag := fmt.Sprintf("%s-%s", proto, trans)

			settings := InboundSettings{
				Clients: []XrayClient{},
			}
			
			if proto == "vless" {
				settings.Decryption = "none"
				if trans == "xtls" {
					settings.Fallbacks = []Fallback{{Dest: 80, Xver: 1}}
				} else {
					settings.Fallbacks = []Fallback{{Dest: 80, Xver: 0}}
				}
			}

			userInbound := Inbound{
				Tag:      tag,
				Port:     443,
				Protocol: proto,
				Settings: settings,
				StreamSettings: StreamSettings{
					Network:  "tcp",
					Security: "tls",
					TLSSettings: &TLSSettings{
						Certificates: []Certificate{
							{CertificateFile: "/etc/xray/xray.crt", KeyFile: "/etc/xray/xray.key"},
						},
					},
				},
				Sniffing: &Sniffing{
					Enabled:      true,
					DestOverride: []string{"http", "tls", "quic"},
				},
			}

			if trans == "xtls" {
				userInbound.StreamSettings.Network = "tcp"
				userInbound.StreamSettings.TLSSettings.Alpn = []string{"h2", "http/1.1"}
			} else if trans == "ws" {
				userInbound.StreamSettings.Network = "ws"
				userInbound.StreamSettings.WSSettings = &WSSettings{
					Path: fmt.Sprintf("/%s-%s", proto, trans),
				}
			} else if trans == "grpc" {
				userInbound.StreamSettings.Network = "grpc"
				userInbound.StreamSettings.GRPCSettings = &GRPCSettings{
					ServiceName: fmt.Sprintf("%s-%s", proto, trans),
					MultiMode:   true,
				}
			}

			for _, c := range clients {
				if c.Protocol == activeStr && !c.IsExpired {
					xc := XrayClient{
						Email: c.Username,
						Level: 0,
					}

					if proto == "trojan" {
						xc.Password = c.UUID 
					} else {
						xc.ID = c.UUID
					}

					if trans == "xtls" && proto == "vless" {
						xc.Flow = "xtls-rprx-vision"
					}

					userInbound.Settings.Clients = append(userInbound.Settings.Clients, xc)
				}
			}

			conf.Inbounds = append(conf.Inbounds, userInbound)
		}
	}

	newConfig, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(CONFIG_XRAY, newConfig, 0644)
}

// GenerateLink creates the sharing link for a client
func GenerateLink(c Client, domain string) string {
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
		vmessConfig := map[string]string{
			"v": "2", "ps": c.Username, "add": domain, "port": port, "id": uuid,
			"aid": "0", "scy": "auto", "net": trans, "type": "none", "tls": "tls", "sni": domain,
		}
		if trans == "ws" {
			vmessConfig["path"] = fmt.Sprintf("/%s-%s", proto, trans)
			vmessConfig["host"] = domain
		} else if trans == "grpc" {
			vmessConfig["path"] = fmt.Sprintf("%s-%s", proto, trans)
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
	fetch := func(direction string) int64 {
		name := fmt.Sprintf("user>>>%s>>>traffic>>>%s", email, direction)
		
		// FIX: Menggunakan absolute path untuk xray binary
		// Ini penting agar cronjob tidak error "file not found"
		xrayPath := "/usr/local/bin/xray"
		if _, err := os.Stat(xrayPath); os.IsNotExist(err) {
			xrayPath = "/usr/bin/xray" // Fallback ke path standar lain
		}

		// Added "-reset" to fetch and clear accumulation
		cmd := exec.Command(xrayPath, "api", "stats", "--server=127.0.0.1:10085", "-name", name, "-reset")
		out, err := cmd.Output()
		if err != nil {
			return 0
		}
		
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
	// FIX: Grep regex diperketat (email: %s ) agar tidak salah hitung user dengan nama mirip
	// grep -c "$email" /var/log/xray/access.log
	
	cmdStr := fmt.Sprintf("tail -n 300 /var/log/xray/access.log | grep 'email: %s ' | grep -v 'rejected' | wc -l", email)
	cmd := exec.Command("bash", "-c", cmdStr)
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
