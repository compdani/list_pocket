package main

import (
	"context"
	"os"
	"syscall"
	"time"

	"github.com/compdani/list_pocket/internal/campaignledger"
	"github.com/compdani/list_pocket/internal/core"
	"github.com/compdani/list_pocket/internal/manager"
	"github.com/compdani/list_pocket/internal/pbdb"
	"github.com/pocketbase/pocketbase"
)

func initCron(pb *pocketbase.PocketBase, co *core.Core, db *pbdb.DB, mgr *manager.Manager, jobs *backgroundCallbacks) {
	if pb != nil && mgr != nil && !ko.Bool("passive") {
		if err := jobs.Add(pb, "campaign-scheduler", "* * * * *", func() {
			mgr.ScanCampaignsOnce()
		}); err != nil {
			lo.Printf("error initializing campaign scheduler cron: %v", err)
		} else {
			lo.Println("campaign scheduler cron enabled at interval: * * * * *")
		}
	}

	// Slow query cache cron job.
	if ko.Bool("app.cache_slow_queries") {
		intval := ko.String("app.cache_slow_queries_interval")
		if intval == "" {
			lo.Println("error: invalid cron interval string for slow query cache")
		} else {
			err := jobs.Add(pb, "slow-query-cache-refresh", intval, func() {
				lo.Println("refreshing slow query cache")
				if err := co.RefreshMatViews(true); err != nil {
					lo.Printf("error refreshing slow query cache: %v", err)
					return
				}
				lo.Println("done refreshing slow query cache")
			})
			if err != nil {
				lo.Printf("error initializing slow cache query cron: %v", err)
			} else {
				lo.Printf("IMPORTANT: database slow query caching is enabled. Aggregate numbers and stats will not be realtime. Interval: %s", intval)
			}
		}
	}

	// Database vacuum cron job.
	if ko.Bool("maintenance.db.vacuum") {
		intval := ko.String("maintenance.db.vacuum_cron_interval")
		if intval == "" {
			lo.Println("error: invalid cron interval string for database vacuum")
		} else {
			err := jobs.Add(pb, "database-vacuum", intval, func() {
				RunDBVacuum(db, lo)
			})
			if err != nil {
				lo.Printf("error initializing database vacuum cron: %v", err)
			} else {
				lo.Printf("database VACUUM cron enabled at interval: %s", intval)
			}
		}
	}

	// Campaign send ledger cleanup cron job.
	if pb != nil && db != nil {
		const (
			ledgerCleanupCron = "15 3 * * *"
			ledgerRetention   = 14
		)
		err := jobs.Add(pb, "campaign-ledger-cleanup", ledgerCleanupCron, func() {
			cutoff := time.Now().UTC().AddDate(0, 0, -ledgerRetention)
			deleted, reconciled, err := campaignledger.CleanupSentOlderThan(db, cutoff)
			if err != nil {
				lo.Printf("error cleaning campaign ledger (cutoff=%s): %v", cutoff.Format(time.RFC3339), err)
				return
			}
			if deleted > 0 || reconciled > 0 {
				lo.Printf("campaign ledger cleanup: deleted=%d reconciled_campaigns=%d cutoff=%s",
					deleted, reconciled, cutoff.Format(time.RFC3339))
			}
		})
		if err != nil {
			lo.Printf("error initializing campaign ledger cleanup cron: %v", err)
		} else {
			lo.Printf("campaign ledger cleanup cron enabled at interval: %s (retention=%d days)",
				ledgerCleanupCron, ledgerRetention)
		}
	}

	// Weekly spam inbox cleanup cron job — deletes spam/confirmed_spam emails older than 7 days.
	if err := jobs.Add(pb, "spam-inbox-cleanup", "0 2 * * 0", func() {
		ctx := context.Background()
		deleted, err := co.DeleteSpamInboundEmails(ctx)
		if err != nil {
			lo.Printf("spam inbox cleanup cron: error: %v", err)
		} else {
			lo.Printf("spam inbox cleanup cron: deleted %d spam email(s)", deleted)
		}
	}); err != nil {
		lo.Printf("error initializing spam inbox cleanup cron: %v", err)
	} else {
		lo.Println("spam inbox cleanup cron enabled at interval: 0 2 * * 0")
	}

}

// startSIGHUPReload watches for SIGHUP and respawns the process after cleanup.
// Settings changes and /mailapi/admin/reload send on the same channel.
func startSIGHUPReload(sigChan chan os.Signal, closer func()) {
	go func() {
		// Only one reload can run. Never replace the process while cleanup is
		// still using its resources, even when draining takes longer than usual.
		if _, ok := <-sigChan; !ok {
			return
		}
		lo.Println("reloading on signal ...")
		reloadAfterCleanup(closer, func() {
			if err := syscall.Exec(os.Args[0], os.Args, os.Environ()); err != nil {
				lo.Fatalf("error spawning process: %v", err)
			}
		})
	}()
}

// reloadAfterCleanup preserves the resource ownership boundary: replacement
// must wait until all cleanup has completed.
// A stuck cleanup (for example a blocked provider send) must not hang the
// reload forever, so replacement proceeds after reloadCleanupTimeout.
const reloadCleanupTimeout = 2 * time.Minute

func reloadAfterCleanup(closer, replace func()) {
	done := make(chan struct{})
	go func() { defer close(done); closer() }()
	select {
	case <-done:
	case <-time.After(reloadCleanupTimeout):
	}
	replace()
}

// initTplFuncs returns a generic template func map with custom template
// functions and sprig template functions.
