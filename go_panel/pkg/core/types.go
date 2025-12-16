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
	Log       map[string]string `json:"log"`
	API       *APIConfig        `json:"api,omitempty"`
	Stats     map[string]string `json:"stats,omitempty"` // Perlu ini agar stats aktif
	Policy    *PolicyConfig     `json:"policy,omitempty"`
	Inbounds  []Inbound         `json:"inbounds"`
	Outbounds []Outbound        `json:"outbounds"`
	Routing   *RoutingConfig    `json:"routing,omitempty"`
}

type Inbound struct {
	Tag            string          `json:"tag"`
	Port           int             `json:"port"`
	Protocol       string          `json:"protocol"`
	Settings       InboundSettings `json:"settings"`
	StreamSettings StreamSettings  `json:"streamSettings"`
	Sniffing       *Sniffing       `json:"sniffing,omitempty"`
}

type InboundSettings struct {
	Clients    []XrayClient `json:"clients,omitempty"`
	Decryption string       `json:"decryption,omitempty"`
	Fallbacks  []Fallback   `json:"fallbacks,omitempty"`
	Address    string       `json:"address,omitempty"` // WAJIB ADA untuk dokodemo-door (API)
}

type Fallback struct {
	Dest int `json:"dest"`
	Xver int `json:"xver"`
}

type XrayClient struct {
	ID       string `json:"id,omitempty"`
	Password string `json:"password,omitempty"` // For Trojan
	Email    string `json:"email"`
	Flow     string `json:"flow,omitempty"` // xtls-rprx-vision
	Level    int    `json:"level"`
}

type StreamSettings struct {
	Network      string        `json:"network"`
	Security     string        `json:"security"`
	TLSSettings  *TLSSettings  `json:"tlsSettings,omitempty"`
	WSSettings   *WSSettings   `json:"wsSettings,omitempty"`
	GRPCSettings *GRPCSettings `json:"grpcSettings,omitempty"`
}

type TLSSettings struct {
	Certificates []Certificate `json:"certificates,omitempty"`
	Alpn         []string      `json:"alpn,omitempty"`
}

type Certificate struct {
	CertificateFile string `json:"certificateFile"`
	KeyFile         string `json:"keyFile"`
}

type WSSettings struct {
	Path string `json:"path"`
}

type GRPCSettings struct {
	ServiceName string `json:"serviceName"`
	MultiMode   bool   `json:"multiMode"`
}

type Sniffing struct {
	Enabled      bool     `json:"enabled"`
	DestOverride []string `json:"destOverride"`
}

type APIConfig struct {
	Tag      string   `json:"tag"`
	Services []string `json:"services"`
}

type PolicyConfig struct {
	Levels map[string]LevelPolicy `json:"levels"`
	System SystemPolicy           `json:"system"`
}

type LevelPolicy struct {
	StatsUserUplink   bool `json:"statsUserUplink"`
	StatsUserDownlink bool `json:"statsUserDownlink"`
	Handshake         int  `json:"handshake"`
	ConnIdle          int  `json:"connIdle"`
	UplinkOnly        int  `json:"uplinkOnly"`
	DownlinkOnly      int  `json:"downlinkOnly"`
	BufferSize        int  `json:"bufferSize"`
}

type SystemPolicy struct {
	StatsInboundUplink   bool `json:"statsInboundUplink"`
	StatsInboundDownlink bool `json:"statsInboundDownlink"`
}

type RoutingConfig struct {
	Rules []RoutingRule `json:"rules"`
}

type RoutingRule struct {
	Type        string   `json:"type"`
	InboundTag  []string `json:"inboundTag,omitempty"`
	OutboundTag string   `json:"outboundTag"`
	IP          []string `json:"ip,omitempty"`
}

type Outbound struct {
	Protocol string `json:"protocol"`
	Tag      string `json:"tag"`
}
