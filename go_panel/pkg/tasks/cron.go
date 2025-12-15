package tasks

import (
	"log"
	"time"

	"github.com/krisna112/scriptxray/go_panel/pkg/core"
)

// RunExpiryCheck replaces xp.sh
func RunExpiryCheck() {
	log.Println("Running Expiry Check...")
	clients, err := core.LoadClients()
	if err != nil {
		log.Printf("Error loading clients: %v", err)
		return
	}

	configChanged := false
	for _, c := range clients {
		if c.Expiry.Before(time.Now()) && !c.IsExpired {
			// Mark as expired in DB (actually LoadClients calculates IsExpired,
			// but we might want to update the protocol string to include -EXPIRED if logic demands it,
			// or just remove the user from config.json)

			// In the original python logic:
			// "EXPIRED" in proto string meant disabled.
			// Let's stick to core logic: if expired, we remove from config.json but keep in DB?
			// Or we update DB to say EXPIRED.

			// Current core.SyncConfig() reads DB.
			// If we want to disable them, we should update DB protocol or expiry flag.

			// Let's assume we remove them from Config but keep in DB with -EXPIRED tag if not present
			// Or simply relying on SyncConfig filtering?

			// Original xp.sh deletes users from config.

			log.Printf("User %s expired.", c.Username)

			// We can use UpdateClient to tag it if we want persistence
			// But SyncConfig is what matters for Xray.

			// Simple approach: Delete from Config if expired.
			// Currently implementation of SyncConfig might not filter expired users unless we tell it to.
			// Let's check SyncConfig logic... it puts everyone from DB to Config.

			// So we need to update DB to flag them as disabled/expired so SyncConfig skips them?
			// Or SyncConfig should check expiry date.
		}
	}

	// Actually, let's implement the logic:
	// Iterate clients. If expired -> Delete from Config (or add -EXPIRED tag).
	// To match xp.sh behavior (often deletes user or disables):
	// Let's disable them by modifying protocol or just rely on SyncConfig.

	// FIX: Let's make SyncConfig smart enough to exclude expired users,
	// OR we modify the DB to add "EXPIRED" tag to protocol so SyncConfig sees it.

	// Updating DB is safer for persistence.
	for _, c := range clients {
		if time.Now().After(c.Expiry) {
			// Logic to disable
			if err := core.DeleteClient(c.Username); err == nil {
				log.Printf("Deleted expired user: %s", c.Username)
				configChanged = true
			}
		}
	}

	if configChanged {
		core.SyncConfig()
		core.RestartXray()
	}
	log.Println("Expiry Check Done.")
}

// RunQuotaCheck replaces quota.sh
func RunQuotaCheck() {
	log.Println("Running Quota Check...")

	clients, err := core.LoadClients()
	if err != nil {
		log.Printf("Error loading clients: %v", err)
		return
	}

	configChanged := false
	for _, c := range clients {
		// 1. Get Traffic
		up, down, _ := core.GetTraffic(c.Username)

		// 2. Update DB Used
		// Note: Xray API stats are cumulative since start.
		// If Xray restarts, stats reset.
		// The original script handles this by storing accumulated stats in stats.db or similar.
		// For this simple migration, we'll implement a basic accumulator if possible,
		// OR we simply assume the DB 'Used' field + current session traffic is the total.
		// However, protecting against reset requires a specialized service or file storage for 'last seen'.

		// Simplification for migration:
		// We add the fetched traffic to the client.Used.
		// BUT we must be careful not to double count if we run this loop often.
		// The API returns "current session" traffic.

		// Correct Logic (as per python/shell script):
		// Total = DB_Stored + Current_Session
		// But when Xray restarts, Current_Session becomes 0.
		// To persist, we should periodically flush Current_Session to DB_Stored and reset Xray stats?
		// No, usually we just read.

		// Let's rely on the DB Used field being "Previous Epochs" and API being "Current Epoch".
		// But detecting restart is hard.

		// Strategy: core.UpdateClient to save usage?
		// Let's just update the DB with the latest known usage.

		// We will stick to the Python logic:
		// total_used = db_usage + session_traffic

		fetchedTraffic := float64(up + down)

		// Since GetTraffic now resets the counter, we MUST add it to the DB immediately
		if fetchedTraffic > 0 {
			err := core.UpdateClient(c.Username, func(target *core.Client) {
				target.Used += fetchedTraffic
			})
			if err != nil {
				log.Printf("Failed to update usage for %s: %v", c.Username, err)
			}
			// Update local struct for quota check logic below
			c.Used += fetchedTraffic
		}

		currentTotal := c.Used

		// 3. Check Quota
		quotaBytes := c.Quota * 1024 * 1024 * 1024

		if quotaBytes > 0 && currentTotal > quotaBytes {
			// Limit reached
			if !c.IsExpired { // reusing IsExpired as generic "Disabled" flag for now
				log.Printf("User %s exceeded quota (%.2f / %.2f GB)", c.Username, currentTotal/1024/1024/1024, c.Quota)
				if err := core.DeleteClient(c.Username); err == nil {
					configChanged = true
				}
			}
		}
	}

	if configChanged {
		core.SyncConfig()
		core.RestartXray()
	}
	log.Println("Quota Check Done.")
}
