package main

import (
	"crypto/md5"
	"database/sql"
	"encoding/json"
	"fmt"
	stdfs "io/fs"
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/compdani/list_pocket/internal/assets"
	"github.com/compdani/list_pocket/internal/auth"
	"github.com/compdani/list_pocket/internal/captcha"
	"github.com/compdani/list_pocket/internal/config"
	"github.com/compdani/list_pocket/internal/core"
	"github.com/compdani/list_pocket/internal/i18n"
	"github.com/compdani/list_pocket/internal/manager"
	"github.com/compdani/list_pocket/internal/media"
	"github.com/compdani/list_pocket/internal/notifs"
	"github.com/compdani/list_pocket/internal/pbdb"
	"github.com/compdani/list_pocket/internal/subimporter"
	"github.com/compdani/list_pocket/models"
	"github.com/jmoiron/sqlx/types"
	"github.com/pocketbase/pocketbase"
)

const (
	emailMsgr = "email"
)

// UrlConfig contains various URL constants used in the app.
type UrlConfig struct {
	RootURL        string `config:"root_url"`
	LogoURL        string `config:"logo_url"`
	FaviconURL     string `config:"favicon_url"`
	LoginURL       string `config:"login_url"`
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
	SiteName                      string   `config:"site_name"`
	FromEmail                     string   `config:"from_email"`
	LogVerbose                    bool     `config:"log_verbose"`
	NotifyEmails                  []string `config:"notify_emails"`
	EnablePublicSubPage           bool     `config:"enable_public_subscription_page"`
	EnablePublicArchive           bool     `config:"enable_public_archive"`
	EnablePublicArchiveRSSContent bool     `config:"enable_public_archive_rss_content"`
	Lang                          string   `config:"lang"`
	DBBatchSize                   int      `config:"batch_size"`
	Privacy                       struct {
		IndividualTracking bool            `config:"individual_tracking"`
		DisableTracking    bool            `config:"disable_tracking"`
		AllowPreferences   bool            `config:"allow_preferences"`
		AllowBlocklist     bool            `config:"allow_blocklist"`
		AllowExport        bool            `config:"allow_export"`
		AllowWipe          bool            `config:"allow_wipe"`
		RecordOptinIP      bool            `config:"record_optin_ip"`
		UnsubHeader        bool            `config:"unsubscribe_header"`
		Exportable         map[string]bool `config:"-"`
		DomainBlocklist    []string        `config:"-"`
		DomainAllowlist    []string        `config:"-"`
	} `config:"privacy"`
	Security struct {
		Captcha struct {
			Altcha struct {
				Enabled    bool `config:"enabled"`
				Complexity int  `config:"complexity"`
			} `config:"altcha"`
			HCaptcha struct {
				Enabled bool   `config:"enabled"`
				Key     string `config:"key"`
				Secret  string `config:"secret"`
			} `config:"hcaptcha"`
		} `config:"captcha"`

		CorsOrigins []string `config:"cors_origins"`
	} `config:"security"`

	Appearance struct {
		AdminCSS  []byte `config:"admin.custom_css"`
		AdminJS   []byte `config:"admin.custom_js"`
		PublicCSS []byte `config:"public.custom_css"`
		PublicJS  []byte `config:"public.custom_js"`
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

// initFlags initializes the commandline flags into the config instance.

// initDB initializes the main DB connection pool from PocketBase.
func initDB() *pbdb.DB {
	db, err := pbdb.NewFromPocketBase(pb)
	if err != nil {
		lo.Fatalf("error initializing SQL adapter from PocketBase: %v", err)
	}

	return db
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
func initUrlConfig(ko *config.Conf) *UrlConfig {
	root := strings.TrimSuffix(ko.String("app.root_url"), "/")

	return &UrlConfig{
		RootURL:    root,
		LogoURL:    ko.String("app.logo_url"),
		FaviconURL: ko.String("app.favicon_url"),
		LoginURL:   path.Join(uriAdmin, "/login"),

		// Static URLS (record ids: campaigns.id, subscribers.id, links.id).
		UnsubURL: fmt.Sprintf("%s/s/%%s/%%s", root),

		OptinURL: fmt.Sprintf("%s/o/%%s?%%s", root),

		LinkTrackURL: fmt.Sprintf("%s/l/%%s/%%s/%%s", root),

		// url.com/tx/link/{link_uuid}/{message_uuid}
		TxLinkTrackURL: fmt.Sprintf("%s/tx/link/%%s/%%s", root),

		MessageURL: fmt.Sprintf("%s/m/%%s/%%s", root),

		// url.com/archive
		ArchiveURL: root + "/archive",

		ViewTrackURL: fmt.Sprintf("%s/v/%%s/%%s/px.png", root),

		// url.com/tx/{message_uuid}/px.png
		TxViewTrackURL: fmt.Sprintf("%s/tx/%%s/px.png", root),
	}
}

// initConstConfig initializes the app's global constants from config.
func initConstConfig(ko *config.Conf) *Config {
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

	if err := ko.UnmarshalFlat("appearance", &c.Appearance); err != nil {
		lo.Fatalf("error loading app.appearance config: %v", err)
	}

	c.Lang = ko.String("app.lang")
	c.Privacy.Exportable = config.LookupStrings(ko.Strings("privacy.exportable"))
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

	pm, err := assets.ReadFile(fs, "/permissions.json")
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
func initI18n(lang string, fsys stdfs.FS) *i18n.I18n {
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
func initCore(fnNotify func(sub models.Subscriber, listIDs []int) (int, error), db *pbdb.DB, i *i18n.I18n, ko *config.Conf) *core.Core {
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
func initCampaignManager(msgrs []manager.Messenger, db *pbdb.DB, u *UrlConfig, co *core.Core, md media.Store, i *i18n.I18n, ko *config.Conf) *manager.Manager {
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
func initImporter(db *pbdb.DB, core *core.Core, i *i18n.I18n, ko *config.Conf) *subimporter.Importer {
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

// registerServeRoutes registers listpocket HTTP routes on a PocketBase ServeEvent.
func initCaptcha() *captcha.Captcha {
	var opt captcha.Opt
	if err := ko.Unmarshal("security.captcha", &opt); err != nil {
		lo.Fatalf("error loading captcha config: %v", err)
	}

	return captcha.New(opt)
}

// initCron initializes cron jobs for slow query cache refresh and database vacuum.
func initAuth(co *core.Core, pb *pocketbase.PocketBase, ko *config.Conf) *auth.Auth {
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
