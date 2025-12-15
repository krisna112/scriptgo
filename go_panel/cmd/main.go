package main

import (
	"flag"
	"log"
	"sync"

	"github.com/krisna112/scriptxray/go_panel/pkg/bot"
	"github.com/krisna112/scriptxray/go_panel/pkg/cli"
	"github.com/krisna112/scriptxray/go_panel/pkg/core"
	"github.com/krisna112/scriptxray/go_panel/pkg/tasks"
	"github.com/krisna112/scriptxray/go_panel/pkg/web"
)

func main() {
	// Modes
	modeMenu := flag.Bool("menu", false, "Run CLI Menu")
	modeXP := flag.Bool("xp", false, "Run Expiry Check")
	modeQuota := flag.Bool("quota", false, "Run Quota Check")

	// Server Flags
	port := flag.Int("port", 5000, "Web Server Port")
	// Bot config now loaded from file

	// Paths
	dbClients := flag.String("db_clients", "/etc/xray/clients.db", "Path to clients.db")
	dbInbounds := flag.String("db_inbounds", "/etc/xray/inbounds.db", "Path to inbounds.db")
	configXray := flag.String("config_xray", "/usr/local/etc/xray/config.json", "Path to xray config.json")

	flag.Parse()

	// Initialize Core
	core.SetPaths(*dbClients, *dbInbounds, *configXray)

	// Routing
	if *modeMenu {
		cli.RunMenu()
		return
	}
	if *modeXP {
		tasks.RunExpiryCheck()
		return
	}
	if *modeQuota {
		tasks.RunQuotaCheck()
		return
	}

	// Default: Run Server & Bot
	var wg sync.WaitGroup

	// Start Web Server
	wg.Add(1)
	go func() {
		defer wg.Done()
		srv := web.NewServer(*port)
		if err := srv.Start(); err != nil {
			log.Fatalf("Web Server Error: %v", err)
		}
	}()

	// Start Bot (if config exists)
	botCfg, err := core.LoadBotConfig()
	if err == nil && botCfg.BotToken != "" && botCfg.AdminID != 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b, err := bot.NewBot(botCfg.BotToken, botCfg.AdminID)
			if err != nil {
				log.Printf("Bot Init Error: %v", err)
				return
			}
			b.Start()
		}()
	} else {
		log.Println("Bot token missing or config not found. Skipping Bot.")
	}

	wg.Wait()
}
