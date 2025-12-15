package core

import (
	"os"
	"os/exec"
	"strings"
)

// EnableBBR writes BBR config to sysctl.conf and applies it
func EnableBBR() error {
	content := `# Optimized by ScriptPanelVPS
net.core.default_qdisc=fq
net.ipv4.tcp_congestion_control=bbr
`
	file := "/etc/sysctl.conf"
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		return err
	}

	return exec.Command("sysctl", "-p").Run()
}

// UpdateXrayCore runs the official install script
func UpdateXrayCore() error {
	cmd := exec.Command("bash", "-c", "curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh | bash -s -- install")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ManageSystemdService controls services (start/stop/restart/enable/disable)
func ManageSystemdService(service, action string) error {
	// Valid actions: start, stop, restart, enable, disable
	return exec.Command("systemctl", action, service).Run()
}

// CheckServiceStatus checks if a service is active
func IsServiceRunning(service string) bool {
	err := exec.Command("systemctl", "is-active", "--quiet", service).Run()
	return err == nil
}

// GetHostname returns system hostname
func GetHostname() string {
	out, err := exec.Command("hostname").Output()
	if err != nil {
		return "Unknown"
	}
	return strings.TrimSpace(string(out))
}
