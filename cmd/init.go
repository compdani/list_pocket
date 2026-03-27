package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	stdfs "io/fs"
	"log"
	"maps"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/Masterminds/sprig/v3"
	"github.com/compdani/list_pocket/internal/auth"
	"github.com/compdani/list_pocket/internal/bounce"
	"github.com/compdani/list_pocket/internal/bounce/mailbox"
	"github.com/compdani/list_pocket/internal/campaignledger"
	"github.com/compdani/list_pocket/internal/captcha"
	"github.com/compdani/list_pocket/internal/core"
	"github.com/compdani/list_pocket/internal/i18n"
	"github.com/compdani/list_pocket/internal/manager"
	"github.com/compdani/list_pocket/internal/media"
	"github.com/compdani/list_pocket/internal/media/providers/filesystem"
	"github.com/compdani/list_pocket/internal/media/providers/s3"
	"github.com/compdani/list_pocket/internal/messenger/email"
	"github.com/compdani/list_pocket/internal/messenger/postback"
	"github.com/compdani/list_pocket/internal/notifs"
	"github.com/compdani/list_pocket/internal/pbdb"
	"github.com/compdani/list_pocket/internal/subimporter"
	"github.com/compdani/list_pocket/internal/workflow"
	"github.com/compdani/list_pocket/models"
	"github.com/jmoiron/sqlx/types"
	koanfmaps "github.com/knadh/koanf/maps"
	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
	"github.com/labstack/echo/v4"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	pbcore "github.com/pocketbase/pocketbase/core"
	flag "github.com/spf13/pflag"
)

const (
	emailMsgr = "email"
)

// UrlConfig contains various URL constants used in the app.
type UrlConfig struct {
	RootURL        string `koanf:"root_url"`
	LogoURL        string `koanf:"logo_url"`
	FaviconURL     string `koanf:"favicon_url"`
	LoginURL       string `koanf:"login_url"`
	UnsubURL       string
	LinkTrackURL   string
	TxLinkTrackURL string
	ViewTrackURL   string
	TxViewTrackURL string
	OptinURL       string
	MessageURL     string
	ArchiveURL     string
}

// Config contains static, constant config values required by arbitrary handlers and functions.
type Config struct {
	SiteName                      string   `koanf:"site_name"`
	FromEmail                     string   `koanf:"from_email"`
	LogVerbose                    bool     `koanf:"log_verbose"`
	NotifyEmails                  []string `koanf:"notify_emails"`
	EnablePublicSubPage           bool     `koanf:"enable_public_subscription_page"`
	EnablePublicArchive           bool     `koanf:"enable_public_archive"`
	EnablePublicArchiveRSSContent bool     `koanf:"enable_public_archive_rss_content"`
	Lang                          string   `koanf:"lang"`
	DBBatchSize                   int      `koanf:"batch_size"`
	Privacy                       struct {
		IndividualTracking bool            `koanf:"individual_tracking"`
		DisableTracking    bool            `koanf:"disable_tracking"`
		AllowPreferences   bool            `koanf:"allow_preferences"`
		AllowBlocklist     bool            `koanf:"allow_blocklist"`
		AllowExport        bool            `koanf:"allow_export"`
		AllowWipe          bool            `koanf:"allow_wipe"`
		RecordOptinIP      bool            `koanf:"record_optin_ip"`
		UnsubHeader        bool            `koanf:"unsubscribe_header"`
		Exportable         map[string]bool `koanf:"-"`
		DomainBlocklist    []string        `koanf:"-"`
		DomainAllowlist    []string        `koanf:"-"`
	} `koanf:"privacy"`
	Security struct {
		Captcha struct {
			Altcha struct {
				Enabled    bool `koanf:"enabled"`
				Complexity int  `koanf:"complexity"`
			} `koanf:"altcha"`
			HCaptcha struct {
				Enabled bool   `koanf:"enabled"`
				Key     string `koanf:"key"`
				Secret  string `koanf:"secret"`
			} `koanf:"hcaptcha"`
		} `koanf:"captcha"`

		CorsOrigins []string `koanf:"cors_origins"`
	} `koanf:"security"`

	Appearance struct {
		AdminCSS  []byte `koanf:"admin.custom_css"`
		AdminJS   []byte `koanf:"admin.custom_js"`
		PublicCSS []byte `koanf:"public.custom_css"`
		PublicJS  []byte `koanf:"public.custom_js"`
	}

	HasLegacyUser bool
	AssetVersion  string

	MediaUpload struct {
		Provider   string
		Extensions []string
	}

	BounceWebhooksEnabled     bool
	BounceSESEnabled          bool
	BounceSendgridEnabled     bool
	BouncePostmarkEnabled     bool
	BounceForwardemailEnabled bool
	BounceBrevoEnabled        bool

	PermissionsRaw json.RawMessage
	Permissions    map[string]struct{}
}

// initFlags initializes the commandline flags into the Koanf instance.
func initFlags(ko *koanf.Koanf) {
	f := flag.NewFlagSet("config", flag.ContinueOnError)
	f.Usage = func() {
		// Register --help handler.
		fmt.Println(f.FlagUsages())
		os.Exit(0)
	}

	// Register the commandline flags.
	f.StringSlice("config", []string{"config.toml"},
		"path to one or more config files (will be merged in order)")
	f.Bool("install", false, "setup database (first time)")
	f.Bool("idempotent", false, "make --install run only if the database isn't already setup")
	f.Bool("upgrade", false, "upgrade database to the current version")
	f.Bool("version", false, "show current version of the build")
	f.Bool("new-config", false, "generate sample config file (at path given in --config)")
	f.String("static-dir", "", "(optional) path to directory with static files")
	f.String("i18n-dir", "", "(optional) path to directory with i18n language files")
	f.Bool("yes", false, "assume 'yes' to prompts during --install/upgrade")
	f.Bool("passive", false, "run in passive mode where campaigns are not processed")
	args := os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "serve", "start":
			args = args[1:]
		}
	}

	if err := f.Parse(args); err != nil {
		lo.Fatalf("error loading flags: %v", err)
	}

	if err := ko.Load(posflag.Provider(f, ".", ko), nil); err != nil {
		lo.Fatalf("error loading config: %v", err)
	}
}

// initConfigFiles loads the given config files into the koanf instance.
func initConfigFiles(files []string, ko *koanf.Koanf) {
	for _, f := range files {
		lo.Printf("reading config: %s", f)
		if err := ko.Load(file.Provider(f), toml.Parser()); err != nil {
			if os.IsNotExist(err) {
				lo.Fatal("config file not found. If there isn't one yet, run --new-config to generate one.")
			}
			lo.Fatalf("error loading config from file: %v.", err)
		}
	}
}

// initFileSystem initializes the stuffbin FileSystem to provide
// access to bundled static assets to the app.
func initFS(appDir, frontendDir, staticDir, i18nDir string) stuffbin.FileSystem {
	var (
		// stuffbin real_path:virtual_alias paths to map local assets on disk
		// when there an embedded filestystem is not found.

		// These paths are joined with appDir.
		appFiles = []string{
			"./config.toml.sample:config.toml.sample",
			"./permissions.json:permissions.json",
		}

		frontendFiles = []string{
			// Admin frontend's static assets accessible at /admin/* during runtime.
			// These paths are sourced from frontendDir.
			"./:/admin",
		}

		staticFiles = []string{
			// These paths are joined with staticDir.
			"./email-templates:static/email-templates",
			"./public:/public",
		}

		i18nFiles = []string{
			// These paths are joined with i18nDir.
			"./:/i18n",
		}
	)

	// Get the executable's execPath.
	execPath, err := os.Executable()
	if err != nil {
		lo.Fatalf("error getting executable path: %v", err)
	}

	// Load embedded files in the executable.
	hasEmbed := true
	fs, err := stuffbin.UnStuff(execPath)
	if err != nil {
		hasEmbed = false

		// Running in local mode. Load local assets into
		// the in-memory stuffbin.FileSystem.
		lo.Printf("unable to initialize embedded filesystem (%v). Using local filesystem", err)

		fs, err = stuffbin.NewLocalFS("/")
		if err != nil {
			lo.Fatalf("failed to initialize local file for assets: %v", err)
		}
	}

	// If the embed failed, load app and frontend files from the compile-time paths.
	files := []string{}
	if !hasEmbed {
		files = append(files, joinFSPaths(appDir, appFiles)...)
		files = append(files, joinFSPaths(frontendDir, frontendFiles)...)
	}

	// Irrespective of the embeds, if there are user specified static or i18n paths,
	// load files from there and override default files (embedded or picked up from CWD).
	if !hasEmbed || i18nDir != "" {
		if i18nDir == "" {
			// Default dir in cwd.
			i18nDir = "i18n"
		}
		lo.Printf("loading i18n files from: %v", i18nDir)
		files = append(files, joinFSPaths(i18nDir, i18nFiles)...)
	}

	if !hasEmbed || staticDir != "" {
		if staticDir == "" {
			// Default dir in cwd.
			staticDir = "static"
		} else {
			// There is a custom static directory. Any paths that aren't in it, exclude.
			sf := []string{}
			for _, def := range staticFiles {
				s := strings.Split(def, ":")[0]
				if _, err := os.Stat(path.Join(staticDir, s)); err == nil {
					sf = append(sf, def)
				}
			}
			staticFiles = sf
		}

		lo.Printf("loading static files from: %v", staticDir)
		files = append(files, joinFSPaths(staticDir, staticFiles)...)
	}

	// No additional files to load.
	if len(files) == 0 {
		return fs
	}

	// Load files from disk and overlay into the FS.
	fStatic, err := stuffbin.NewLocalFS("/", files...)
	if err != nil {
		lo.Fatalf("failed reading static files from disk: '%s': %v", staticDir, err)
	}

	if err := fs.Merge(fStatic); err != nil {
		lo.Fatalf("error merging static files: '%s': %v", staticDir, err)
	}

	return fs
}

type stuffbinSubFS struct {
	base stuffbin.FileSystem
	root string
}

func (s stuffbinSubFS) Open(name string) (stdfs.File, error) {
	name = strings.TrimPrefix(filepath.ToSlash(name), "/")
	name = path.Clean(name)
	if name == "." {
		name = ""
	}

	if name == ".." || strings.HasPrefix(name, "../") {
		return nil, os.ErrNotExist
	}

	fullPath := s.root
	if name != "" {
		fullPath = path.Join(s.root, name)
	}

	return s.base.Open(fullPath)
}

// initDB initializes the main DB connection pool from PocketBase.
func initDB() *pbdb.DB {
	db, err := pbdb.NewFromPocketBase(pb)
	if err != nil {
		lo.Fatalf("error initializing SQL adapter from PocketBase: %v", err)
	}

	return db
}

func isSQLiteDB(db *pbdb.DB) bool {
	if db == nil || db.DB == nil {
		return false
	}

	return strings.Contains(strings.ToLower(db.DriverName()), "sqlite")
}

func sqliteTableHasColumn(raw *sql.DB, table, column string) bool {
	if raw == nil {
		return false
	}

	rows, err := raw.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid      int
			name     string
			colType  string
			notNull  int
			defaultV sql.NullString
			primaryK int
		)

		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultV, &primaryK); err != nil {
			continue
		}

		if name == column {
			return true
		}
	}

	return false
}

func sqliteTimestampColumns(raw *sql.DB, table string) (string, string) {
	createdCol := "created"
	updatedCol := "updated"

	if !sqliteTableHasColumn(raw, table, createdCol) && sqliteTableHasColumn(raw, table, "created_at") {
		createdCol = "created_at"
	}

	if !sqliteTableHasColumn(raw, table, updatedCol) && sqliteTableHasColumn(raw, table, "updated_at") {
		updatedCol = "updated_at"
	}

	return createdCol, updatedCol
}

// initSettings loads settings from the DB into the given Koanf map.
func initSettings(ko *koanf.Koanf) {
	var s types.JSONText

	if pb != nil {
		if out, ok, err := getPBSettings(pb); err != nil {
			lo.Fatalf("error reading settings from PocketBase: %v", err)
		} else if ok {
			if isLegacyPBSettingsBlob(out) {
				b, err := makeDefaultPBSettings(ko)
				if err != nil {
					lo.Fatalf("error marshaling repaired settings: %v", err)
				}
				if err := setPBSettings(pb, b); err != nil {
					lo.Fatalf("error repairing PocketBase settings: %v", err)
				}
				s = b
			} else {
				s = out
			}
		} else {
			// First run: persist app settings defaults into PocketBase settings.
			b, err := makeDefaultPBSettings(ko)
			if err != nil {
				lo.Fatalf("error marshaling default settings: %v", err)
			}
			if err := setPBSettings(pb, b); err != nil {
				lo.Fatalf("error seeding PocketBase settings: %v", err)
			}
			s = b
		}
	} else {
		lo.Fatalf("pocketbase is not initialized")
	}

	// Setting keys are dot separated, eg: app.favicon_url. Unflatten them into
	// nested maps {app: {favicon_url}}.
	var out map[string]any
	if err := json.Unmarshal(s, &out); err != nil {
		lo.Fatalf("error unmarshalling settings from DB: %v", err)
	}
	if err := ko.Load(confmap.Provider(out, "."), nil); err != nil {
		lo.Fatalf("error parsing settings from DB: %v", err)
	}

	if strings.TrimSpace(ko.String("app.lang")) == "" {
		if err := ko.Set("app.lang", "en"); err != nil {
			lo.Fatalf("error setting default app.lang: %v", err)
		}
	}
	if strings.TrimSpace(ko.String("upload.provider")) == "" {
		if err := ko.Set("upload.provider", "filesystem"); err != nil {
			lo.Fatalf("error setting default upload.provider: %v", err)
		}
	}
	if strings.TrimSpace(ko.String("upload.filesystem.upload_uri")) == "" {
		if err := ko.Set("upload.filesystem.upload_uri", "/uploads"); err != nil {
			lo.Fatalf("error setting default upload.filesystem.upload_uri: %v", err)
		}
	}
	if strings.TrimSpace(ko.String("upload.filesystem.upload_path")) == "" {
		if err := ko.Set("upload.filesystem.upload_path", "uploads"); err != nil {
			lo.Fatalf("error setting default upload.filesystem.upload_path: %v", err)
		}
	}
}

func isLegacyPBSettingsBlob(raw []byte) bool {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}

	_, hasSiteName := m["app.site_name"]
	_, hasSMTP := m["smtp"]
	_, hasAppAddress := m["app.address"]
	_, hasDBHost := m["db.host"]

	return !hasSiteName && !hasSMTP && (hasAppAddress || hasDBHost)
}

func makeDefaultPBSettings(ko *koanf.Koanf) ([]byte, error) {
	s := models.Settings{
		AppLang:                    "en",
		UploadProvider:             "filesystem",
		UploadFilesystemUploadURI:  "/uploads",
		UploadFilesystemUploadPath: "uploads",
		UploadExtensions:           []string{},
		AppNotifyEmails:            []string{},
		PrivacyExportable:          []string{},
		DomainBlocklist:            []string{},
		DomainAllowlist:            []string{},
		SecurityCORSOrigins:        []string{},
		SMTP: []models.SMTPSettings{
			{
				Enabled:          true,
				Port:             25,
				AuthProtocol:     "login",
				EmailHeaders:     []map[string]string{},
				FromAddresses:    []string{},
				DefaultFromEmail: "",
				MaxConns:         10,
				MaxMsgRetries:    2,
				IdleTimeout:      "15s",
				WaitTimeout:      "5s",
				TLSType:          "none",
			},
		},
		Messengers: []struct {
			UUID          string `json:"uuid"`
			Enabled       bool   `json:"enabled"`
			Name          string `json:"name"`
			RootURL       string `json:"root_url"`
			Username      string `json:"username"`
			Password      string `json:"password,omitempty"`
			MaxConns      int    `json:"max_conns"`
			Timeout       string `json:"timeout"`
			MaxMsgRetries int    `json:"max_msg_retries"`
		}{},
		BounceActions: map[string]struct {
			Count  int    `json:"count"`
			Action string `json:"action"`
		}{
			"soft":      {Count: 1, Action: "none"},
			"hard":      {Count: 1, Action: "none"},
			"complaint": {Count: 1, Action: "none"},
		},
		BounceBoxes: []struct {
			UUID          string `json:"uuid"`
			Enabled       bool   `json:"enabled"`
			Type          string `json:"type"`
			Host          string `json:"host"`
			Port          int    `json:"port"`
			AuthProtocol  string `json:"auth_protocol"`
			ReturnPath    string `json:"return_path"`
			Username      string `json:"username"`
			Password      string `json:"password,omitempty"`
			TLSEnabled    bool   `json:"tls_enabled"`
			TLSSkipVerify bool   `json:"tls_skip_verify"`
			ScanInterval  string `json:"scan_interval"`
		}{
			{
				Type:         "pop",
				Port:         110,
				AuthProtocol: "userpass",
				ScanInterval: "15m",
			},
		},
	}

	if v := strings.TrimSpace(ko.String("app.lang")); v != "" {
		s.AppLang = v
	}
	if v := strings.TrimSpace(ko.String("upload.provider")); v != "" {
		s.UploadProvider = v
	}
	if v := strings.TrimSpace(ko.String("upload.filesystem.upload_uri")); v != "" {
		s.UploadFilesystemUploadURI = v
	}
	if v := strings.TrimSpace(ko.String("upload.filesystem.upload_path")); v != "" {
		s.UploadFilesystemUploadPath = v
	}

	return json.Marshal(s)
}

func initPocketBase() *pocketbase.PocketBase {
	pb := pocketbase.NewWithConfig(pocketbase.Config{
		HideStartBanner: true,
		DefaultDataDir:  "pb_data",
	})

	if err := pb.Bootstrap(); err != nil {
		lo.Fatalf("error bootstrapping pocketbase: %v", err)
	}

	if err := pb.RunAppMigrations(); err != nil {
		lo.Fatalf("error running pocketbase app migrations: %v", err)
	}

	return pb
}

func getPBSettings(pb *pocketbase.PocketBase) (types.JSONText, bool, error) {
	var row struct {
		Value []byte `db:"value"`
	}

	for _, table := range []string{"listpocket_settings", "listmonk_settings"} {
		err := pb.DB().NewQuery("SELECT value FROM " + table + " LIMIT 1").One(&row)
		if err == nil {
			return row.Value, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			continue
		}
	}

	return nil, false, nil
}

func setPBSettings(pb *pocketbase.PocketBase, value []byte) error {
	var (
		collection *pbcore.Collection
		err        error
	)

	for _, name := range []string{"listpocket_settings", "listmonk_settings"} {
		collection, err = pb.FindCollectionByNameOrId(name)
		if err == nil {
			break
		}
	}
	if err != nil {
		return err
	}

	// Check if a settings record already exists
	var existingRecord *pbcore.Record
	records, err := pb.FindRecordsByFilter(collection, "", "", 1, 0)
	if err == nil && len(records) > 0 {
		existingRecord = records[0]
	}

	if existingRecord != nil {
		// Update existing record
		existingRecord.Set("value", string(value))
		return pb.Save(existingRecord)
	} else {
		// Create new record
		record := pbcore.NewRecord(collection)
		record.Set("value", string(value))
		return pb.Save(record)
	}
}

func patchPBSettings(pb *pocketbase.PocketBase, key string, value json.RawMessage) error {
	raw, ok, err := getPBSettings(pb)
	if err != nil {
		return err
	}
	if !ok {
		raw = []byte("{}")
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}

	var out any
	if err := json.Unmarshal(value, &out); err != nil {
		return err
	}
	m[key] = out

	b, err := json.Marshal(m)
	if err != nil {
		return err
	}

	return setPBSettings(pb, b)
}

func initUrlConfig(ko *koanf.Koanf) *UrlConfig {
	root := strings.TrimSuffix(ko.String("app.root_url"), "/")

	return &UrlConfig{
		RootURL:    root,
		LogoURL:    ko.String("app.logo_url"),
		FaviconURL: ko.String("app.favicon_url"),
		LoginURL:   path.Join(uriAdmin, "/login"),

		// Static URLS.
		// url.com/subscription/{campaign_uuid}/{subscriber_uuid}
		UnsubURL: fmt.Sprintf("%s/subscription/%%s/%%s", root),

		// url.com/subscription/optin/{subscriber_uuid}
		OptinURL: fmt.Sprintf("%s/subscription/optin/%%s?%%s", root),

		// url.com/link/{campaign_uuid}/{subscriber_uuid}/{link_uuid}
		LinkTrackURL: fmt.Sprintf("%s/link/%%s/%%s/%%s", root),

		// url.com/tx/link/{link_uuid}/{message_uuid}
		TxLinkTrackURL: fmt.Sprintf("%s/tx/link/%%s/%%s", root),

		// url.com/link/{campaign_uuid}/{subscriber_uuid}
		MessageURL: fmt.Sprintf("%s/campaign/%%s/%%s", root),

		// url.com/archive
		ArchiveURL: root + "/archive",

		// url.com/campaign/{campaign_uuid}/{subscriber_uuid}/px.png
		ViewTrackURL: fmt.Sprintf("%s/campaign/%%s/%%s/px.png", root),

		// url.com/tx/{message_uuid}/px.png
		TxViewTrackURL: fmt.Sprintf("%s/tx/%%s/px.png", root),
	}
}

// initConstConfig initializes the app's global constants from the given koanf instance.
func initConstConfig(ko *koanf.Koanf) *Config {
	// Read constants.
	var c Config
	if err := ko.Unmarshal("app", &c); err != nil {
		lo.Fatalf("error loading app config: %v", err)
	}
	if err := ko.Unmarshal("privacy", &c.Privacy); err != nil {
		lo.Fatalf("error loading app.privacy config: %v", err)
	}
	if err := ko.Unmarshal("security", &c.Security); err != nil {
		lo.Fatalf("error loading app.security config: %v", err)
	}

	if err := ko.UnmarshalWithConf("appearance", &c.Appearance, koanf.UnmarshalConf{FlatPaths: true}); err != nil {
		lo.Fatalf("error loading app.appearance config: %v", err)
	}

	c.Lang = ko.String("app.lang")
	c.Privacy.Exportable = koanfmaps.StringSliceToLookupMap(ko.Strings("privacy.exportable"))
	c.MediaUpload.Provider = ko.String("upload.provider")
	c.MediaUpload.Extensions = ko.Strings("upload.extensions")
	c.Privacy.DomainBlocklist = ko.Strings("privacy.domain_blocklist")
	c.Privacy.DomainAllowlist = ko.Strings("privacy.domain_allowlist")

	c.BounceWebhooksEnabled = ko.Bool("bounce.webhooks_enabled")
	c.BounceSESEnabled = ko.Bool("bounce.ses_enabled")
	c.BounceSendgridEnabled = ko.Bool("bounce.sendgrid_enabled")
	c.BouncePostmarkEnabled = ko.Bool("bounce.postmark.enabled")
	c.BounceForwardemailEnabled = ko.Bool("bounce.forwardemail.enabled")
	c.BounceBrevoEnabled = ko.Bool("bounce.brevo.enabled")
	c.HasLegacyUser = ko.Exists("app.admin_username") || ko.Exists("app.admin_password")

	b := md5.Sum([]byte(time.Now().String()))
	c.AssetVersion = fmt.Sprintf("%x", b)[0:10]

	pm, err := fs.Read("/permissions.json")
	if err != nil {
		lo.Fatalf("error reading permissions file: %v", err)
	}
	c.PermissionsRaw = pm

	// Make a lookup map of permissions.
	permGroups := []struct {
		Group       string   `json:"group"`
		Permissions []string `json:"permissions"`
	}{}
	if err := json.Unmarshal(pm, &permGroups); err != nil {
		lo.Fatalf("error loading permissions file: %v", err)
	}

	c.Permissions = map[string]struct{}{}
	for _, group := range permGroups {
		for _, g := range group.Permissions {
			c.Permissions[g] = struct{}{}
		}
	}

	return &c
}

// initI18n initializes a new i18n instance with the selected language map
// loaded from the filesystem. English is a loaded first as the default map
// and then the selected language is loaded on top of it so that if there are
// missing translations in it, the default English translations show up.
func initI18n(lang string, fs stuffbin.FileSystem) *i18n.I18n {
	i, ok, err := getI18nLang(lang, fs)
	if err != nil {
		if ok {
			lo.Println(err)
		} else {
			lo.Fatal(err)
		}
	}
	return i
}

// initCore initializes the CRUD DB core .
func initCore(fnNotify func(sub models.Subscriber, listIDs []int) (int, error), db *pbdb.DB, i *i18n.I18n, ko *koanf.Koanf) *core.Core {
	opt := &core.Opt{
		Constants: core.Constants{
			SendOptinConfirmation: ko.Bool("app.send_optin_confirmation"),
			CacheSlowQueries:      ko.Bool("app.cache_slow_queries"),
		},
		DB:   db,
		I18n: i,
		Log:  lo,
	}

	if pb != nil {
		opt.GetSettings = func() (types.JSONText, error) {
			v, _, err := getPBSettings(pb)
			return v, err
		}
		opt.SetSettings = func(v types.JSONText) error {
			return setPBSettings(pb, v)
		}
		opt.SetSettingsByKey = func(key string, value json.RawMessage) error {
			return patchPBSettings(pb, key, value)
		}
	}

	// Load bounce config.
	if err := ko.Unmarshal("bounce.actions", &opt.Constants.BounceActions); err != nil {
		lo.Fatalf("error unmarshalling bounce config: %v", err)
	}

	// Initialize the CRUD core.
	return core.New(opt, &core.Hooks{
		SendOptinConfirmation: fnNotify,
	})
}

// initCampaignManager initializes the campaign manager.
func initCampaignManager(msgrs []manager.Messenger, db *pbdb.DB, u *UrlConfig, co *core.Core, md media.Store, i *i18n.I18n, ko *koanf.Koanf) *manager.Manager {
	if ko.Bool("passive") {
		lo.Println("running in passive mode. won't process campaigns.")
	}

	mgr := manager.New(manager.Config{
		BatchSize:             ko.Int("app.batch_size"),
		Concurrency:           ko.Int("app.concurrency"),
		MessageRate:           ko.Int("app.message_rate"),
		LogVerbose:            ko.Bool("app.log_verbose"),
		MaxSendErrors:         ko.Int("app.max_send_errors"),
		FromEmail:             ko.String("app.from_email"),
		IndividualTracking:    ko.Bool("privacy.individual_tracking"),
		DisableTracking:       ko.Bool("privacy.disable_tracking"),
		UnsubURL:              u.UnsubURL,
		OptinURL:              u.OptinURL,
		LinkTrackURL:          u.LinkTrackURL,
		TxLinkTrackURL:        u.TxLinkTrackURL,
		ViewTrackURL:          u.ViewTrackURL,
		TxViewTrackURL:        u.TxViewTrackURL,
		MessageURL:            u.MessageURL,
		ArchiveURL:            u.ArchiveURL,
		RootURL:               u.RootURL,
		UnsubHeader:           ko.Bool("privacy.unsubscribe_header"),
		SlidingWindow:         ko.Bool("app.message_sliding_window"),
		SlidingWindowDuration: ko.Duration("app.message_sliding_window_duration"),
		SlidingWindowRate:     ko.Int("app.message_sliding_window_rate"),
		ScanInterval:          time.Second * 5,
		ScanCampaigns:         false,
	}, newManagerStore(db, co, md, lo, evStream, ko.Bool("app.log_verbose")), i, lo)

	// Attach all messengers to the campaign manager.
	for _, m := range msgrs {
		mgr.AddMessenger(m)
	}

	return mgr
}

// initTxTemplates initializes and compiles the transactional templates and caches them in-memory.
func initTxTemplates(m *manager.Manager, co *core.Core) {
	tpls, err := co.GetTemplates(models.TemplateTypeTx, false)
	if err != nil {
		lo.Printf("skipping transactional template cache initialization: %v", err)
		return
	}

	for _, t := range tpls {
		tpl := t
		if err := tpl.Compile(m.GenericTemplateFuncs()); err != nil {
			lo.Printf("error compiling transactional template %d: %v", tpl.ID, err)
			continue
		}
		m.CacheTpl(tpl.ID, &tpl)
	}
}

// initImporter initializes the bulk subscriber importer.
func initImporter(db *pbdb.DB, core *core.Core, i *i18n.I18n, ko *koanf.Koanf) *subimporter.Importer {
	_, listUpdatedCol := sqliteTimestampColumns(db.DB.DB, "lists")

	updateListDateStmt, err := db.DB.DB.Prepare(`
UPDATE lists
SET ` + listUpdatedCol + `=(strftime('%Y-%m-%d %H:%M:%fZ'))
WHERE id IN (SELECT value FROM json_each(?1));
`)
	if err != nil {
		lo.Printf("disabling importer: unable to prepare importer queries: %v", err)
		return nil
	}

	return subimporter.New(
		subimporter.Options{
			DomainBlocklist:    ko.Strings("privacy.domain_blocklist"),
			DomainAllowlist:    ko.Strings("privacy.domain_allowlist"),
			UpsertStmt:         nil,
			BlocklistStmt:      nil,
			UpdateListDateStmt: updateListDateStmt,
			ResolveListIDs: func(listIDs []int) ([]string, error) {
				return core.SQLiteListRecordIDs(listIDs, nil)
			},

			// Hook for triggering admin notifications and refreshing stats materialized
			// views after a successful import.
			PostCB: func(subject string, data any) error {
				// Refresh cached subscriber counts and stats.
				core.RefreshMatViews(true)

				// Send admin notification.
				notifs.NotifySystem(subject, notifs.TplImport, data, nil)
				return nil
			},
		}, db.DB.DB, i)
}

// initSMTPMessenger initializes the combined and individual SMTP messengers.
func initSMTPMessengers() []manager.Messenger {
	var (
		servers = []email.Server{}
		out     = []manager.Messenger{}
	)

	// Load the config for multiple SMTP servers.
	for _, item := range ko.Slices("smtp") {
		if !item.Bool("enabled") {
			continue
		}

		// Read the SMTP config.
		var s email.Server
		if err := item.UnmarshalWithConf("", &s, koanf.UnmarshalConf{Tag: "json"}); err != nil {
			lo.Fatalf("error reading SMTP config: %v", err)
		}

		servers = append(servers, s)
		lo.Printf("initialized email (SMTP) messenger: %s@%s", item.String("username"), item.String("host"))

		// If the server has a name, initialize it as a standalone e-mail messenger
		// allowing campaigns to select individual SMTPs. In the UI and config, it'll appear as `email / $name`.
		if s.Name != "" {
			msgr, err := email.New(s.Name, s)
			if err != nil {
				lo.Fatalf("error initializing e-mail messenger: %v", err)
			}
			out = append(out, msgr)
		}
	}

	// Initialize the 'email' messenger with all SMTP servers.
	msgr, err := email.New(email.MessengerName, servers...)
	if err != nil {
		lo.Fatalf("error initializing e-mail messenger: %v", err)
	}

	// If it's just one server, return the default "email" messenger.
	if len(servers) == 1 {
		return []manager.Messenger{msgr}
	}

	// If there are multiple servers, prepend the group "email" to be the first one.
	out = append([]manager.Messenger{msgr}, out...)

	return out
}

// initPostbackMessengers initializes and returns all the enabled
// HTTP postback messenger backends.
func initPostbackMessengers(ko *koanf.Koanf) []manager.Messenger {
	items := ko.Slices("messengers")
	if len(items) == 0 {
		return nil
	}

	var out []manager.Messenger
	for _, item := range items {
		if !item.Bool("enabled") {
			continue
		}

		// Read the Postback server config.
		var (
			name = item.String("name")
			o    postback.Options
		)
		if err := item.UnmarshalWithConf("", &o, koanf.UnmarshalConf{Tag: "json"}); err != nil {
			lo.Fatalf("error reading Postback config: %v", err)
		}

		// Initialize the Messenger.
		p, err := postback.New(o)
		if err != nil {
			lo.Fatalf("error initializing Postback messenger %s: %v", name, err)
		}
		out = append(out, p)

		lo.Printf("loaded Postback messenger: %s", name)
	}

	return out
}

// initMediaStore initializes Upload manager with a custom backend.
func initMediaStore(ko *koanf.Koanf) media.Store {
	switch provider := ko.String("upload.provider"); provider {
	case "s3":
		var o s3.Opt
		ko.Unmarshal("upload.s3", &o)
		o.RootURL = ko.String("app.root_url")

		up, err := s3.NewS3Store(o)
		if err != nil {
			lo.Fatalf("error initializing s3 upload provider %s", err)
		}
		lo.Println("media upload provider: s3")
		return up

	case "filesystem":
		var o filesystem.Opts

		ko.Unmarshal("upload.filesystem", &o)
		o.RootURL = ko.String("app.root_url")
		o.UploadPath = filepath.Clean(o.UploadPath)
		o.UploadURI = filepath.Clean(o.UploadURI)
		up, err := filesystem.New(o)
		if err != nil {
			lo.Fatalf("error initializing filesystem upload provider %s", err)
		}
		lo.Println("media upload provider: filesystem")
		return up

	default:
		lo.Fatalf("unknown provider. select filesystem or s3")
	}
	return nil
}

// initNotifs initializes the notifier with the system e-mail templates.
func initNotifs(fs stuffbin.FileSystem, i *i18n.I18n, em *email.Emailer, u *UrlConfig, ko *koanf.Koanf) {
	tpls, err := stuffbin.ParseTemplatesGlob(initTplFuncs(i, u), fs, "/static/email-templates/*.html")
	if err != nil {
		lo.Fatalf("error parsing e-mail notif templates: %v", err)
	}

	// Read the notification templates.
	html, err := fs.Read("/static/email-templates/base.html")
	if err != nil {
		lo.Fatalf("error reading static/email-templates/base.html: %v", err)
	}

	// Determine whether the notification templates are HTML or plaintext.
	// Copy the first few (arbitrary) bytes of the template and check if has the <!doctype html> tag.
	ln := min(len(html), 256)
	h := make([]byte, ln)
	copy(h, html[0:ln])

	contentType := models.CampaignContentTypeHTML
	if !bytes.Contains(bytes.ToLower(h), []byte("<!doctype html")) {
		contentType = models.CampaignContentTypePlain
		lo.Println("system e-mail templates are plaintext")
	}

	notifs.Initialize(notifs.Opt{
		FromEmail:    ko.String("app.from_email"),
		SystemEmails: ko.Strings("app.notify_emails"),
		ContentType:  contentType,
	}, tpls, em, lo)
}

// initBounceManager initializes the bounce manager that scans mailboxes and listens to webhooks
// for incoming bounce events.
func initBounceManager(cb func(models.Bounce) error, lo *log.Logger, ko *koanf.Koanf) *bounce.Manager {
	opt := bounce.Opt{
		WebhooksEnabled: ko.Bool("bounce.webhooks_enabled"),
		SESEnabled:      ko.Bool("bounce.ses_enabled"),
		SendgridEnabled: ko.Bool("bounce.sendgrid_enabled"),
		SendgridKey:     ko.String("bounce.sendgrid_key"),
		Postmark: struct {
			Enabled  bool
			Username string
			Password string
		}{
			ko.Bool("bounce.postmark.enabled"),
			ko.String("bounce.postmark.username"),
			ko.String("bounce.postmark.password"),
		},
		ForwardEmail: struct {
			Enabled bool
			Key     string
		}{
			ko.Bool("bounce.forwardemail.enabled"),
			ko.String("bounce.forwardemail.key"),
		},
		BrevoEnabled:   ko.Bool("bounce.brevo.enabled"),
		BrevoToken:     ko.String("bounce.brevo.token"),
		RecordBounceCB: cb,
	}

	// For now, only one mailbox is supported.
	for _, b := range ko.Slices("bounce.mailboxes") {
		if !b.Bool("enabled") {
			continue
		}

		var boxOpt mailbox.Opt
		if err := b.UnmarshalWithConf("", &boxOpt, koanf.UnmarshalConf{Tag: "json"}); err != nil {
			lo.Fatalf("error reading bounce mailbox config: %v", err)
		}

		opt.MailboxType = b.String("type")
		opt.MailboxEnabled = true
		opt.Mailbox = boxOpt
		break
	}

	// Initialize the bounce manager.
	b, err := bounce.New(opt, lo)
	if err != nil {
		lo.Fatalf("error initializing bounce manager: %v", err)
	}

	return b
}

// initAbout initializes the app's /about API endpoint with the app and system info.
func initAbout(db *pbdb.DB) about {
	var (
		mem runtime.MemStats
	)

	// Memory / alloc stats.
	runtime.ReadMemStats(&mem)

	info := types.JSONText(`{}`)
	if err := db.QueryRow(`SELECT JSON_OBJECT('version', SQLITE_VERSION(), 'size_mb', NULL) AS info`).Scan(&info); err != nil {
		lo.Printf("WARNING: error getting database version: %v", err)
	}

	hostname, err := os.Hostname()
	if err != nil {
		lo.Printf("WARNING: error getting hostname: %v", err)
	}

	return about{
		Version:   versionString,
		Build:     buildString,
		GoArch:    runtime.GOARCH,
		GoVersion: runtime.Version(),
		Database:  info,
		System: aboutSystem{
			NumCPU: runtime.NumCPU(),
		},
		Host: aboutHost{
			OS:       runtime.GOOS,
			Machine:  runtime.GOARCH,
			Hostname: hostname,
		},
	}

}

// initHTTPServer sets up and runs the app's main HTTP server.
func initHTTPServer(cfg *Config, urlCfg *UrlConfig, i *i18n.I18n, fs stuffbin.FileSystem, app *App) *echo.Echo {
	// Parse templates for public pages.
	tpl, err := stuffbin.ParseTemplatesGlob(initTplFuncs(i, urlCfg), fs, "/public/templates/*.html")
	if err != nil {
		lo.Fatalf("error parsing public templates: %v", err)
	}

	app.tpl = tpl

	campaignledger.RegisterHooks(pb)

	workflow.Register(pb, workflow.Config{
		FrontendDir: "../frontend/dist",
		SendTransactional: func(ctx context.Context, req workflow.ExecutorTransactionalEmailRequest) (workflow.ExecutorTransactionalEmailResult, error) {
			record, err := app.newTransactionalSender().Send(txRequestFromWorkflow(req))
			if err != nil {
				return workflow.ExecutorTransactionalEmailResult{}, err
			}
			return workflow.ExecutorTransactionalEmailResult{
				RecordID:        record.RecordID,
				UUID:            record.UUID,
				SubscriberID:    record.SubscriberID,
				SubscriberEmail: record.SubscriberEmail,
				TemplateID:      record.TemplateID,
				TemplateName:    record.TemplateName,
				Status:          record.Status,
				Subject:         record.Subject,
			}, nil
		},
	})

	// Register all routes using PocketBase's router.
	pb.OnServe().BindFunc(func(se *pbcore.ServeEvent) error {
		// Inject app into request context middleware.
		se.Router.BindFunc(func(e *pbcore.RequestEvent) error {
			e.Set("app", app)
			return e.Next()
		})

		registerDocsRoutes(se, ko)

		// Public (subscriber) facing static files.
		se.Router.GET("/public/static/{path...}", apis.Static(stuffbinSubFS{base: fs, root: "/public/static"}, false))

		// Admin (frontend) facing static files.
		se.Router.GET("/admin/static/{path...}", apis.Static(stuffbinSubFS{base: fs, root: "/admin/static"}, false))

		// Public (subscriber) facing media upload files.
		var (
			uploadProvider = ko.String("upload.provider")
			uploadFsURI    = ko.String("upload.filesystem.upload_uri")
			publicURL      = ko.String("upload.s3.public_url")
		)
		switch {
		case uploadProvider == "filesystem" && uploadFsURI != "":
			staticPath := ko.String("upload.filesystem.upload_path")
			se.Router.GET(path.Join(uploadFsURI, "{filepath...}"), func(e *pbcore.RequestEvent) error {
				http.StripPrefix(uploadFsURI, http.FileServer(http.Dir(staticPath))).ServeHTTP(e.Response, e.Request)
				return nil
			})
		case uploadProvider == "s3" && strings.HasPrefix(publicURL, "/"):
			se.Router.GET(path.Join(publicURL, "{filepath}"), wrapEcho(app, tpl, cfg, urlCfg, []string{"filepath"}, app.ServeS3Media))
		}

		// Register all HTTP handlers.
		registerHandlers(se.Router, app, tpl, cfg, urlCfg)

		return se.Next()
	})

	go func() {
		if err := apis.Serve(pb, apis.ServeConfig{HttpAddr: ko.String("app.address")}); err != nil {
			if errors.Is(err, http.ErrServerClosed) || errors.Is(err, context.Canceled) {
				lo.Println("HTTP server shut down")
			} else {
				lo.Fatalf("error starting pocketbase HTTP server: %v", err)
			}
		}
	}()

	app.pb = pb

	// Return a dummy Echo instance for compatibility with shutdown code.
	return echo.New()
}

// initCaptcha initializes the captcha service.
func initCaptcha() *captcha.Captcha {
	var opt captcha.Opt
	if err := ko.Unmarshal("security.captcha", &opt); err != nil {
		lo.Fatalf("error loading captcha config: %v", err)
	}

	return captcha.New(opt)
}

// initCron initializes cron jobs for slow query cache refresh and database vacuum.
func initCron(pb *pocketbase.PocketBase, co *core.Core, db *pbdb.DB, mgr *manager.Manager) {
	if pb != nil && mgr != nil && !ko.Bool("passive") {
		if err := pb.Cron().Add("campaign-scheduler", "* * * * *", func() {
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
			err := pb.Cron().Add("slow-query-cache-refresh", intval, func() {
				lo.Println("refreshing slow query cache")
				_ = co.RefreshMatViews(true)
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
			err := pb.Cron().Add("database-vacuum", intval, func() {
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
		err := pb.Cron().Add("campaign-ledger-cleanup", ledgerCleanupCron, func() {
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

}

// awaitReload waits for a SIGHUP signal to reload the app. Every setting change on the UI causes a reload.
func awaitReload(sigChan chan os.Signal, closerWait chan bool, closer func()) chan bool {
	// The blocking signal handler that main() waits on.
	out := make(chan bool)

	// Respawn a new process and exit the running one.
	respawn := func() {
		if err := syscall.Exec(os.Args[0], os.Args, os.Environ()); err != nil {
			lo.Fatalf("error spawning process: %v", err)
		}
		os.Exit(0)
	}

	// Listen for reload signal.
	go func() {
		for range sigChan {
			lo.Println("reloading on signal ...")

			go closer()
			select {
			case <-closerWait:
				// Wait for the closer to finish.
				respawn()
			case <-time.After(time.Second * 3):
				// Or timeout and force close.
				respawn()
			}
		}
	}()

	return out
}

// initTplFuncs returns a generic template func map with custom template
// functions and sprig template functions.
func initTplFuncs(i *i18n.I18n, u *UrlConfig) template.FuncMap {
	funcs := template.FuncMap{
		"TrackView": func(...any) template.HTML {
			return template.HTML("")
		},
		"UnsubscribeURL": func(...any) string {
			return ""
		},
		"ManageURL": func(...any) string {
			return ""
		},
		"OptinURL": func(...any) string {
			return ""
		},
		"MessageURL": func(...any) string {
			return ""
		},
		"ArchiveURL": func(...any) string {
			return ""
		},
		"RootURL": func() string {
			return u.RootURL
		},
		"LogoURL": func() string {
			return u.LogoURL
		},
		"Date": func(layout string) string {
			if layout == "" {
				layout = time.ANSIC
			}
			return time.Now().Format(layout)
		},
		"L": func() *i18n.I18n {
			return i
		},
		"Safe": func(safeHTML string) template.HTML {
			return template.HTML(safeHTML)
		},
	}

	// Copy spring functions.
	sprigFuncs := sprig.GenericFuncMap()
	delete(sprigFuncs, "env")
	delete(sprigFuncs, "expandenv")
	delete(sprigFuncs, "getHostByName")

	maps.Copy(funcs, sprigFuncs)

	return funcs
}

// initAuth initializes the auth module.
func initAuth(co *core.Core, pb *pocketbase.PocketBase, ko *koanf.Koanf) *auth.Auth {
	callbacks := &auth.Callbacks{
		GetUser:           func(recordID string) (auth.User, error) { return co.GetUser(recordID, "", "") },
		GetUsers:          co.GetUsers,
		GetUserByUsername: func(username string) (auth.User, error) { return co.GetUser("", username, "") },
	}

	a, err := auth.New(auth.Config{AuthCollection: "users"}, nil, pb, callbacks, lo)
	if err != nil {
		lo.Fatalf("error initializing auth module: %v", err)
	}

	// If the legacy username+password is set in the TOML file, warn users about migration.
	username := ko.String("app.admin_username")
	password := ko.String("app.admin_password")
	if len(username) > 2 && len(password) > 6 {
		lo.Println(`WARNING: Remove the admin_username and admin_password fields from the TOML configuration file. If you are using APIs, create and use new credentials. Users are now managed via the Admin -> Settings -> Users dashboard.`)
	}

	return a
}

// joinFSPaths joins the given paths with the root path and returns the full paths.
func joinFSPaths(root string, paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		// real_path:stuffbin_alias
		f := strings.Split(p, ":")

		out = append(out, path.Join(root, f[0])+":"+f[1])
	}

	return out
}
