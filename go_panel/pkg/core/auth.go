package core

import (
	"encoding/json"
	"os"
)

var ADMIN_CONFIG = "/etc/xray/web_admin.json"

type AdminCreds struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func GetAdminCreds() AdminCreds {
	defaultCreds := AdminCreds{Username: "admin", Password: "admin123"}

	file, err := os.ReadFile(ADMIN_CONFIG)
	if err != nil {
		return defaultCreds
	}

	var creds AdminCreds
	if err := json.Unmarshal(file, &creds); err != nil {
		return defaultCreds
	}

	return creds
}

func SaveAdminCreds(username, password string) error {
	creds := AdminCreds{Username: username, Password: password}
	data, err := json.Marshal(creds)
	if err != nil {
		return err
	}
	return os.WriteFile(ADMIN_CONFIG, data, 0644)
}
