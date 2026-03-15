package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/knadh/koanf/v2"
	"github.com/knadh/listmonk/internal/pbdb"
	"github.com/knadh/stuffbin"
	"github.com/lib/pq"
)

// migFunc represents a migration function for a particular version.
// fn (generally) executes database migrations and additionally
// takes the filesystem and config objects in case there are additional bits
// of logic to be performed before executing upgrades. fn is idempotent.
type migFunc struct {
	version string
	fn      func(*pbdb.DB, stuffbin.FileSystem, *koanf.Koanf, *log.Logger) error
}

// upgrade upgrades the database to the current version by running SQL migration files
// for all version from the last known version to the current one.
// If record is false, migration versions are not recorded in the DB (used for nightly builds).
func upgrade(db *pbdb.DB, fs stuffbin.FileSystem, prompt bool, record bool) {
	if prompt {
		var ok string
		fmt.Printf("** IMPORTANT: Take a backup of the database before upgrading.\n")
		fmt.Print("continue (y/n)?  ")
		if _, err := fmt.Scanf("%s", &ok); err != nil {
			lo.Fatalf("error reading value from terminal: %v", err)
		}
		if strings.ToLower(ok) != "y" {
			fmt.Println("upgrade cancelled")
			return
		}
	}

	_, toRun, err := getPendingMigrations(db)
	if err != nil {
		lo.Fatalf("error checking migrations: %v", err)
	}

	// No migrations to run.
	if len(toRun) == 0 {
		lo.Printf("no upgrades to run. Database is up to date.")
		return
	}

	// Execute migrations in succession.
	for _, m := range toRun {
		lo.Printf("running migration %s", m.version)
		if err := m.fn(db, fs, ko, lo); err != nil {
			lo.Fatalf("error running migration %s: %v", m.version, err)
		}

		// Record the migration version in the settings table. There was no
		// settings table until v0.7.0, so ignore the no-table errors.
		// For nightly builds, skip recording so migrations re-run on each boot.
		if record {
			if err := recordMigrationVersion(m.version, db); err != nil {
				if isTableNotExistErr(err) {
					continue
				}
				lo.Fatalf("error recording migration version %s: %v", m.version, err)
			}
		}
	}

	lo.Printf("upgrade complete")
}

// checkUpgrade checks if the current database schema matches the expected
// binary version.
func checkUpgrade(db *pbdb.DB) {
	lastVer, toRun, err := getPendingMigrations(db)
	if err != nil {
		lo.Fatalf("error checking migrations: %v", err)
	}

	// No migrations to run.
	if len(toRun) == 0 {
		return
	}

	var vers []string
	for _, m := range toRun {
		vers = append(vers, m.version)
	}

	lo.Fatalf(`there are %d pending database upgrade(s): %v. The last upgrade was %s. Backup the database and run listmonk --upgrade`,
		len(toRun), vers, lastVer)
}

// getPendingMigrations gets the pending migrations by comparing the last
// recorded migration in the DB against all migrations listed in `migrations`.
func getPendingMigrations(db *pbdb.DB) (string, []migFunc, error) {
	lastVer, err := getLastMigrationVersion(db)
	if err != nil {
		return "", nil, err
	}
	return lastVer, nil, nil
}

// getLastMigrationVersion returns the last migration semver recorded in the DB.
// If there isn't any, `v0.0.0` is returned.
func getLastMigrationVersion(db *pbdb.DB) (string, error) {
	var v string
	if err := db.Get(&v, `
		SELECT COALESCE(
			(SELECT value->>-1 FROM settings WHERE key='migrations'),
		'v0.0.0')`); err != nil {
		if isTableNotExistErr(err) {
			return "v0.0.0", nil
		}
		return v, err
	}
	return v, nil
}

// isTableNotExistErr checks if the given error represents a Postgres/pq
// "table does not exist" error.
func isTableNotExistErr(err error) bool {
	if p, ok := err.(*pq.Error); ok {
		// `settings` table does not exist. It was introduced in v0.7.0.
		if p.Code == "42P01" {
			return true
		}
	}
	return false
}
