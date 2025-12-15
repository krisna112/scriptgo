package core

import "time"

// Client represents a user in the system (from clients.db)
type Client struct {
	Username  string    `json:"username"`
	Quota     float64   `json:"quota"` // GB
	Used      float64   `json:"used"`  // Bytes
	Expiry    time.Time `json:"expiry"`
	Protocol  string    `json:"protocol"` // e.g., VLESS-XTLS
	UUID      string    `json:"uuid"`
	IsExpired bool      `json:"is_expired"`
	IsOnline  bool      `json:"is_online"`
}

// XrayConfig matches the structure of config.json
type XrayConfig struct {
	Inbounds []Inbound `json:"inbounds"`
	// Add other fields if needed, but we mostly touch inbounds
}

type Inbound struct {
	Tag            string          `json:"tag"`
	Port           int             `json:"port"`
	Protocol       string          `json:"protocol"`
	Settings       InboundSettings `json:"settings"`
	StreamSettings StreamSettings  `json:"streamSettings"`
}

type InboundSettings struct {
	Clients []XrayClient `json:"clients,omitempty"`
}

type XrayClient struct {
	ID       string `json:"id,omitempty"`
	Password string `json:"password,omitempty"` // For Trojan
	Email    string `json:"email"`
	Flow     string `json:"flow,omitempty"` // xtls-rprx-vision
	Level    int    `json:"level"`
}

type StreamSettings struct {
	Network  string `json:"network"`
	Security string `json:"security"`
}
