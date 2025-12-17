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

// Struktur sederhana untuk Inbound di DB
type InboundDet struct {
	Tag       string
	Protocol  string
	Transport string
	Port      int
}

type BotConfig struct {
	BotToken string `json:"bot_token"`
	AdminID  int64  `json:"admin_id"`
}

func SetPaths(clients, inbounds, config string) {
	DB_CLIENTS = clients
	DB_INBOUNDS = inbounds
	CONFIG_XRAY = config
}

// --- CLIENT MANAGER ---

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

// --- MULTI-INBOUND MANAGER ---

// LoadAllInbounds returns all configured inbounds
func LoadAllInbounds() ([]InboundDet, error) {
	var inbounds []InboundDet
	file, err := os.Open(DB_INBOUNDS)
	if os.IsNotExist(err) {
		return inbounds, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ";")
		// Format: active;protocol-trans;port
		if len(parts) >= 3 {
			port, _ := strconv.Atoi(strings.TrimSpace(parts[2]))
			tagFull := strings.TrimSpace(parts[1])
			
			// Split Tag (vless-xtls) -> vless, xtls
			tagParts := strings.Split(tagFull, "-")
			proto := ""
			trans := ""
			if len(tagParts) == 2 {
				proto = tagParts[0]
				trans = tagParts[1]
			}

			if port > 0 && proto != "" {
				inbounds = append(inbounds, InboundDet{
					Tag:       tagFull,
					Protocol:  proto,
					Transport: trans,
					Port:      port,
				})
			}
		}
	}
	return inbounds, scanner.Err()
}

// AddInbound appends new inbound (supports multiple ports)
func AddInbound(protocol, transport string, port int) error {
	// Cek apakah port sudah ada di DB
	currents, _ := LoadAllInbounds()
	for _, cur := range currents {
		if cur.Port == port {
			return fmt.Errorf("port %d already used by %s", port, cur.Tag)
		}
	}

	line := fmt.Sprintf("active;%s-%s;%d\n", protocol, transport, port)
	f, err := os.OpenFile(DB_INBOUNDS, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	
	if _, err := f.WriteString(line); err != nil {
		return err
	}
	return SyncConfig()
}

// DeleteInbound removes specific port
func DeleteInbound(targetPort int) error {
	inbounds, err := LoadAllInbounds()
	if err != nil {
		return err
	}
	
	f, err := os.OpenFile(DB_INBOUNDS, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, inb := range inbounds {
		if inb.Port == targetPort {
			continue // Skip deleted
		}
		line := fmt.Sprintf("active;%s;%d\n", inb.Tag, inb.Port)
		if _, err := f.WriteString(line); err != nil {
			return err
		}
	}
	return nil
}

// GetActiveInbound helper for backward compatibility (returns first found)
func GetActiveInbound() (string, int, error) {
	inbounds, err := LoadAllInbounds()
	if err != nil || len(inbounds) == 0 {
		return "", 0, fmt.Errorf("no inbounds")
	}
	// Return first found
	return inbounds[0].Tag, inbounds[0].Port, nil
}

func SyncConfig() error {
	clients, err := LoadClients()
	if err != nil {
		return err
	}

	inbounds, err := LoadAllInbounds()
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
					Handshake:         10,
					ConnIdle:          1200,
					UplinkOnly:        0,
					DownlinkOnly:      0,
					BufferSize:        512,
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

	// LOOP SEMUA INBOUND DARI DB
	for _, inb := range inbounds {
		proto := inb.Protocol
		trans := inb.Transport
		tag := fmt.Sprintf("%s-%s-%d", proto, trans, inb.Port) // Unik Tag per Port

		settings := InboundSettings{
			Clients: []XrayClient{},
		}
		
		// Logic Fallback Khusus Port 443
		// Ini mencegah Xray error jika diakses via browser biasa
		if inb.Port == 443 {
			if trans == "xtls" {
				settings.Fallbacks = []Fallback{{Dest: 80, Xver: 1}}
			} else {
				settings.Fallbacks = []Fallback{{Dest: 80, Xver: 0}}
			}
		}

		userInbound := Inbound{
			Tag:      tag,
			Port:     inb.Port,
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

		// Tambahkan User yang sesuai dengan Protocol Inbound ini
		for _, c := range clients {
			if c.Protocol == inb.Tag && !c.IsExpired {
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

	newConfig, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(CONFIG_XRAY, newConfig, 0644)
}

func GenerateLink(c Client, domain string) string {
	// Kita cari port dari inbound yang cocok dengan protocol client
	inbounds, _ := LoadAllInbounds()
	var targetPort int
	
	// Default cari port 443 dulu jika ada yang cocok
	for _, inb := range inbounds {
		if inb.Tag == c.Protocol && inb.Port == 443 {
			targetPort = 443
			break
		}
	}
	// Jika tidak ada di 443, ambil port pertama yang cocok dengan protocol
	if targetPort == 0 {
		for _, inb := range inbounds {
			if inb.Tag == c.Protocol {
				targetPort = inb.Port
				break
			}
		}
	}
	
	// Fallback jika tidak ditemukan, default ke 443
	port := "443"
	if targetPort > 0 {
		port = strconv.Itoa(targetPort)
	}

	parts := strings.Split(c.Protocol, "-")
	if len(parts) < 2 {
		return ""
	}
	proto := strings.ToLower(parts[0])
	trans := strings.ToLower(parts[1])
	uuid := c.UUID

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
		} else {
			return fmt.Sprintf("trojan://%s@%s:%s?security=tls&type=tcp&sni=%s&alpn=h2,http/1.1#%s",
				uuid, domain, port, domain, c.Username)
		}
	}
	return ""
}

func RestartXray() error {
	cmd := exec.Command("systemctl", "restart", "xray")
	return cmd.Run()
}

func GetTraffic(email string) (int64, int64, error) {
	fetch := func(direction string) int64 {
		name := fmt.Sprintf("user>>>%s>>>traffic>>>%s", email, direction)
		xrayPath := "/usr/local/bin/xray"
		if _, err := os.Stat(xrayPath); os.IsNotExist(err) {
			xrayPath = "/usr/bin/xray"
		}
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

func IsUserOnline(email string) bool {
	cmdStr := fmt.Sprintf("tail -n 300 /var/log/xray/access.log | grep 'email: %s ' | grep -v 'rejected' | wc -l", email)
	cmd := exec.Command("bash", "-c", cmdStr)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	count, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return count > 0
}

func LoadBotConfig() (BotConfig, error) {
	var cfg BotConfig
	file, err := os.ReadFile(CONFIG_BOT)
	if err != nil {
		return cfg, err
	}
	err = json.Unmarshal(file, &cfg)
	return cfg, err
}

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

func RemoveBotConfig() error {
	return os.Remove(CONFIG_BOT)
}
