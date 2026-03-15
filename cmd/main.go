package main

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "github.com/knadh/listmonk/internal/migrations"

	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/bounce"
	"github.com/knadh/listmonk/internal/buflog"
	"github.com/knadh/listmonk/internal/captcha"
	"github.com/knadh/listmonk/internal/core"
	"github.com/knadh/listmonk/internal/events"
	"github.com/knadh/listmonk/internal/i18n"
	"github.com/knadh/listmonk/internal/manager"
	"github.com/knadh/listmonk/internal/media"
	"github.com/knadh/listmonk/internal/messenger/email"
	"github.com/knadh/listmonk/internal/pbdb"
	"github.com/knadh/listmonk/internal/subimporter"
	"github.com/knadh/listmonk/models"
	"github.com/knadh/paginator"
	"github.com/knadh/stuffbin"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
)

// App contains the "global" shared components, controllers and fields.
type App struct {
	cfg        *Config
	urlCfg     *UrlConfig
	fs         stuffbin.FileSystem
	db         *pbdb.DB
	queries    *models.Queries
	core       *core.Core
	manager    *manager.Manager
	messengers []manager.Messenger
	emailMsgr  manager.Messenger
	importer   *subimporter.Importer
	auth       *auth.Auth
	media      media.Store
	bounce     *bounce.Manager
	captcha    *captcha.Captcha
	i18n       *i18n.I18n
	pg         *paginator.Paginator
	events     *events.Events
	log        *log.Logger
	bufLog     *buflog.BufLog
	pb         *pocketbase.PocketBase
	tpl        *template.Template

	about         about
	fnOptinNotify func(models.Subscriber, []int) (int, error)

	// Channel for passing reload signals.
	chReload chan os.Signal

	// Global variable that stores the state indicating that a restart is required
	// after a settings update.
	needsRestart bool

	// First time installation with no user records in the DB. Needs user setup.
	needsUserSetup bool

	// Global state that stores data on an available remote update.
	update *AppUpdate
	sync.Mutex
}

var (
	// Buffered log writer for storing N lines of log entries for the UI.
	evStream = events.New()
	bufLog   = buflog.New(5000)
	lo       = log.New(io.MultiWriter(os.Stdout, bufLog, evStream.ErrWriter()), "", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)

	ko      = koanf.New(".")
	fs      stuffbin.FileSystem
	db      *pbdb.DB
	queries *models.Queries
	pb      *pocketbase.PocketBase

	// Compile-time variables.
	buildString   string
	versionString string

	// If these are set in build ldflags and static assets (*.sql, config.toml.sample. ./frontend)
	// are not embedded (in make dist), these paths are looked up. The default values before, when not
	// overridden by build flags, are relative to the CWD at runtime.
	appDir      string = "."
	frontendDir string = "frontend/dist"
)

func init() {
	// Initialize commandline flags.
	initFlags(ko)

	// Display version.
	if ko.Bool("version") {
		fmt.Println(buildString)
		os.Exit(0)
	}

	lo.Println(buildString)

	// Generate new config.
	if ko.Bool("new-config") {
		path := ko.Strings("config")[0]
		if err := newConfigFile(path); err != nil {
			lo.Println(err)
			os.Exit(1)
		}
		lo.Printf("generated %s. Edit and run --install", path)
		os.Exit(0)
	}

	// Load config files to pick up the database settings first.
	initConfigFiles(ko.Strings("config"), ko)

	// Load environment variables and merge into the loaded config.
	// LISTMONK_foo__bar -> foo.bar (double underscore becomes dot for nested config)
	// LISTMONK_static_dir -> static-dir (top-level keys with underscore become hyphen for CLI flags)
	if err := ko.Load(env.Provider("LISTMONK_", ".", func(s string) string {
		key := strings.ToLower(strings.TrimPrefix(s, "LISTMONK_"))
		key = strings.ReplaceAll(key, "__", ".")
		// Only convert underscore to hyphen for top-level keys (CLI flags like static-dir, i18n-dir)
		// Nested config keys (containing dots) keep underscores (e.g., db.ssl_mode)
		if !strings.Contains(key, ".") {
			key = strings.ReplaceAll(key, "_", "-")
		}
		return key
	}), nil); err != nil {
		lo.Fatalf("error loading config from env: %v", err)
	}

	// Initialize PocketBase.
	pb = initPocketBase()
	migratecmd.MustRegister(pb, pb.RootCmd, migratecmd.Config{
		// enable auto creation of migration files when making collection changes in the Dashboard
		// (the IsProbablyGoRun check is to enable it only during development)
		Automigrate: false, // disable automigrate to prevent accidental migration file creation in production
	})

	db = initDB()

	// Initialize the embedded filesystem with static assets.
	fs = initFS(appDir, frontendDir, ko.String("static-dir"), ko.String("i18n-dir"))

	// Installer mode? This runs before the SQL queries are loaded and prepared
	// as the installer needs to work on an empty DB.
	if ko.Bool("install") {
		lo.Printf("PocketBase mode: app migrations are applied automatically on startup; skipping --install")
		os.Exit(0)
	}

	if ko.Bool("upgrade") {
		lo.Printf("PocketBase mode: app migrations are applied automatically on startup; skipping --upgrade")
		os.Exit(0)
	}

	// Read the SQL queries from the queries file.
	qMap := readQueries(queryFilePath, fs)

	// Load settings from DB.
	if q, ok := qMap["get-settings"]; ok {
		initSettings(q.Query, db, ko)
	}

	// Prepare queries.
	queries = prepareQueries(qMap, db, ko)
}

func main() {
	var (
		// Initialize static global config.
		cfg = initConstConfig(ko)

		// Initialize static URL config.
		urlCfg = initUrlConfig(ko)

		// Initialize i18n language map.
		i18n = initI18n(ko.MustString("app.lang"), fs)

		// Initialize the media store.
		media = initMediaStore(ko)

		fbOptinNotify = makeOptinNotifyHook(ko.Bool("privacy.unsubscribe_header"), urlCfg, queries, i18n)

		// Crud core.
		core = initCore(fbOptinNotify, queries, db, i18n, ko)

		// Auth module.
		authn = initAuth(core, pb, ko)

		// Initialize all messengers, SMTP and postback.
		msgrs = append(initSMTPMessengers(), initPostbackMessengers(ko)...)

		// Campaign manager.
		mgr = initCampaignManager(msgrs, queries, db, urlCfg, core, media, i18n, ko)

		// Bulk importer.
		importer = initImporter(queries, db, core, i18n, ko)

		// Initialize the webhook/POP3 bounce processor.
		bounce *bounce.Manager

		emailMsgr *email.Emailer

		chReload = make(chan os.Signal, 1)
	)

	// Initialize the bounce manager that processes bounces from webhooks and
	// POP3 mailbox scanning.
	if ko.Bool("bounce.enabled") {
		bounce = initBounceManager(core.RecordBounce, queries.RecordBounce, lo, ko)
	}

	// Assign the default `email` messenger to the app.
	for _, m := range msgrs {
		if m.Name() == "email" {
			emailMsgr = m.(*email.Emailer)
		}
	}

	// Initialize the global admin/sub e-mail notifier.
	initNotifs(fs, i18n, emailMsgr, urlCfg, ko)

	// Initialize and cache tx templates in memory.
	initTxTemplates(mgr, core)

	// Initialize the bounce manager that processes bounces from webhooks and
	// POP3 mailbox scanning.
	if ko.Bool("bounce.enabled") {
		go bounce.Run()
	}

	// Start cronjobs.
	initCron(core, db)

	// Start the campaign manager workers. The campaign batches (fetch from DB, push out
	// messages) get processed at the specified interval.
	if ko.Bool("passive") {
		lo.Println("running in passive mode. won't process campaigns.")
	} else {
		go mgr.Run()
	}

	// Sync user/auth records and determine first-time setup state.
	hasUser, err := cacheUsers(core, authn)
	if err != nil {
		lo.Fatalf("error caching users: %v", err)
	}
	// =========================================================================
	// Initialize the App{} with all the global shared components, controllers and fields.
	app := &App{
		cfg:        cfg,
		urlCfg:     urlCfg,
		fs:         fs,
		db:         db,
		queries:    queries,
		core:       core,
		manager:    mgr,
		messengers: msgrs,
		emailMsgr:  emailMsgr,
		importer:   importer,
		auth:       authn,
		media:      media,
		bounce:     bounce,
		captcha:    initCaptcha(),
		i18n:       i18n,
		log:        lo,
		events:     evStream,
		bufLog:     bufLog,

		pg: paginator.New(paginator.Opt{
			DefaultPerPage: 20,
			MaxPerPage:     50,
			NumPageNums:    10,
			PageParam:      "page",
			PerPageParam:   "per_page",
			AllowAll:       true,
		}),

		fnOptinNotify: fbOptinNotify,
		about:         initAbout(queries, db),
		chReload:      chReload,

		// First time installation with no user records in the DB.
		needsUserSetup: !hasUser,
	}

	// Star the update checker.
	if ko.Bool("app.check_updates") {
		go app.checkUpdates(versionString, time.Hour*24)
	}

	// Start the app server.
	srv := initHTTPServer(cfg, urlCfg, i18n, fs, app)

	// =========================================================================
	// Wait for the reload signal with a callback to gracefully shut down resources.
	// The `wait` channel is passed to awaitReload to wait for the callback to finish
	// within N seconds, or do a force reload.
	signal.Notify(chReload, syscall.SIGHUP)

	closerWait := make(chan bool)
	<-awaitReload(chReload, closerWait, func() {
		// Stop the HTTP server.
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
		if app.pb != nil {
			_ = app.pb.ResetBootstrapState()
		}

		// Close the campaign manager.
		mgr.Close()

		// Close the DB pool.
		db.Close()

		// Close the messenger pool.
		for _, m := range app.messengers {
			m.Close()
		}

		// Signal the close.
		closerWait <- true
	})
}
