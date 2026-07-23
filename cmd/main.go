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

	_ "github.com/compdani/list_pocket/internal/migrations"

	"github.com/compdani/list_pocket/internal/auth"
	"github.com/compdani/list_pocket/internal/bounce"
	"github.com/compdani/list_pocket/internal/buflog"
	"github.com/compdani/list_pocket/internal/campaignledger"
	"github.com/compdani/list_pocket/internal/captcha"
	"github.com/compdani/list_pocket/internal/core"
	"github.com/compdani/list_pocket/internal/events"
	"github.com/compdani/list_pocket/internal/i18n"
	"github.com/compdani/list_pocket/internal/manager"
	"github.com/compdani/list_pocket/internal/media"
	"github.com/compdani/list_pocket/internal/messenger/email"
	"github.com/compdani/list_pocket/internal/messenger/quo"
	"github.com/compdani/list_pocket/internal/pbdb"
	"github.com/compdani/list_pocket/internal/subimporter"
	"github.com/compdani/list_pocket/internal/workflow"
	"github.com/compdani/list_pocket/models"
	"github.com/joho/godotenv"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/paginator"
	"github.com/knadh/stuffbin"
	"github.com/pocketbase/pocketbase"
	pbcore "github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
)

// App contains the "global" shared components, controllers and fields.
type App struct {
	cfg        *Config
	urlCfg     *UrlConfig
	fs         stuffbin.FileSystem
	db         *pbdb.DB
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
	aiBuilder  *aiBuilderService
	log        *log.Logger
	bufLog     *buflog.BufLog
	pb         *pocketbase.PocketBase
	tpl        *template.Template
	renderer   *tplRenderer

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

	ko = koanf.New(".")
	fs stuffbin.FileSystem
	db *pbdb.DB
	pb *pocketbase.PocketBase

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
	// Skip application bootstrapping during `go test` for the cmd package.
	// The test binary name ends with ".test".
	if strings.HasSuffix(os.Args[0], ".test") {
		return
	}

	// In local/dev mode, load environment variables from .env.
	tempIsProd := os.Getenv("is_prod")
	if strings.ToLower(strings.TrimSpace(tempIsProd)) != "true" {
		if err := godotenv.Load(); err != nil {
			lo.Fatal("Error loading .env file")
		}
	}

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
		lo.Printf("generated %s. Edit the file and run the app (migrations apply on serve).", path)
		os.Exit(0)
	}

	// Load config files to pick up the database settings first.
	initConfigFiles(ko.Strings("config"), ko)

	// Load environment variables and merge into the loaded config.
	// LISTPOCKET_foo__bar -> foo.bar (double underscore becomes dot for nested config)
	// LISTPOCKET_static_dir -> static-dir (top-level keys with underscore become hyphen for CLI flags)
	if err := ko.Load(env.Provider("LISTPOCKET_", ".", func(s string) string {
		key := strings.ToLower(strings.TrimPrefix(s, "LISTPOCKET_"))
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

	// Deprecated listmonk installer flags: migrations run via PocketBase serve/startup.
	if ko.Bool("install") {
		lo.Printf("--install is deprecated: app migrations run automatically on serve/startup")
		os.Exit(0)
	}
	if ko.Bool("upgrade") {
		lo.Printf("--upgrade is deprecated: app migrations run automatically on serve/startup")
		os.Exit(0)
	}

	// Create PocketBase (bootstrap happens in pb.Start()).
	pb = initPocketBase()
	migratecmd.MustRegister(pb, pb.RootCmd, migratecmd.Config{
		Automigrate: false, // disable automigrate to prevent accidental migration file creation in production
	})

	// Initialize the embedded filesystem with static assets (no DB required).
	fs = initFS(appDir, frontendDir, ko.String("static-dir"), ko.String("i18n-dir"))
}

func main() {
	var (
		app   *App
		appMu sync.Mutex
	)

	pb.OnBootstrap().BindFunc(func(e *pbcore.BootstrapEvent) error {
		if err := e.Next(); err != nil {
			return err
		}

		// App migrations must run before settings/DB-dependent wiring.
		// apis.Serve also runs migrations; a second pass is a no-op for applied ones.
		if err := pb.RunAppMigrations(); err != nil {
			return fmt.Errorf("run app migrations: %w", err)
		}

		db = initDB()
		initSettings(ko)
		return nil
	})

	campaignledger.RegisterHooks(pb)

	workflow.Register(pb, workflow.Config{
		FrontendDir: "../frontend/dist",
		SendTransactional: func(ctx context.Context, req workflow.ExecutorTransactionalEmailRequest) (workflow.ExecutorTransactionalEmailResult, error) {
			_ = ctx
			appMu.Lock()
			a := app
			appMu.Unlock()
			if a == nil {
				return workflow.ExecutorTransactionalEmailResult{}, fmt.Errorf("app not ready")
			}
			record, err := a.newTransactionalSender().Send(txRequestFromWorkflow(req))
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

	chReload := make(chan os.Signal, 1)
	signal.Notify(chReload, syscall.SIGHUP)
	startSIGHUPReload(chReload, func() {
		appMu.Lock()
		a := app
		appMu.Unlock()
		shutdownApp(a)
		if pb != nil {
			_ = pb.ResetBootstrapState()
		}
		if db != nil {
			_ = db.Close()
		}
	})

	pb.OnServe().BindFunc(func(se *pbcore.ServeEvent) error {
		appMu.Lock()
		if app == nil {
			app = wireApp(chReload)
		}
		a := app
		appMu.Unlock()

		registerServeRoutes(se, a)
		return se.Next()
	})

	pb.OnTerminate().BindFunc(func(e *pbcore.TerminateEvent) error {
		appMu.Lock()
		a := app
		appMu.Unlock()
		shutdownApp(a)
		return e.Next()
	})

	ensureDefaultServeArgs()

	if err := pb.Start(); err != nil {
		lo.Fatalf("error starting pocketbase: %v", err)
	}
}

// wireApp constructs the application services after PocketBase bootstrap/settings load.
func wireApp(chReload chan os.Signal) *App {
	cfg := initConstConfig(ko)
	urlCfg := initUrlConfig(ko)
	i18n := initI18n(ko.MustString("app.lang"), fs)
	mediaStore := initMediaStore(ko)
	fbOptinNotify := makeOptinNotifyHook(ko.Bool("privacy.unsubscribe_header"), urlCfg, db, i18n)
	coreSvc := initCore(fbOptinNotify, db, i18n, ko)
	authn := initAuth(coreSvc, pb, ko)
	msgrs := append(initSMTPMessengers(), initPostbackMessengers(ko)...)
	mgr := initCampaignManager(msgrs, db, urlCfg, coreSvc, mediaStore, i18n, ko)
	importer := initImporter(db, coreSvc, i18n, ko)

	var bounceMgr *bounce.Manager
	var emailMsgr *email.Emailer

	mgr.SetSMSRateLimits(func() models.TextMessagingSendLimits {
		return loadTextMessagingSettingsFromPB(pb).SendLimits
	})
	tm := loadTextMessagingSettingsFromPB(pb)
	if p := tm.QuoProvider(); p != nil && p.Enabled && strings.TrimSpace(p.APIKey) != "" {
		qm := quo.NewMessenger(func() models.TextMessagingSettings {
			return loadTextMessagingSettingsFromPB(pb)
		})
		if err := mgr.AddMessenger(qm); err != nil {
			lo.Printf("register quo messenger: %v", err)
		} else {
			msgrs = append(msgrs, qm)
		}
	}

	if ko.Bool("bounce.enabled") {
		bounceMgr = initBounceManager(coreSvc.RecordBounce, lo, ko)
	}

	for _, m := range msgrs {
		if m.Name() == "email" {
			emailMsgr = m.(*email.Emailer)
		}
	}

	initNotifs(fs, i18n, emailMsgr, urlCfg, ko)
	initTxTemplates(mgr, coreSvc)

	if ko.Bool("bounce.enabled") && bounceMgr != nil {
		go bounceMgr.Run()
	}

	initCron(pb, coreSvc, db, mgr)

	if ko.Bool("passive") {
		lo.Println("running in passive mode. won't process campaigns or workflow runs.")
	} else {
		workflow.StartRunWorker(pb)
		go mgr.Run()
	}

	if err := ensureSuperAdminRolePermissions(coreSvc, cfg); err != nil {
		lo.Fatalf("error ensuring super admin permissions: %v", err)
	}

	hasUser, err := refreshAuthCache(authn)
	if err != nil {
		lo.Fatalf("error caching users: %v", err)
	}

	tpl := parsePublicTemplates(i18n, urlCfg, fs)
	renderer := newTplRenderer(tpl, cfg, urlCfg, i18n)

	app := &App{
		cfg:        cfg,
		urlCfg:     urlCfg,
		fs:         fs,
		db:         db,
		core:       coreSvc,
		manager:    mgr,
		messengers: msgrs,
		emailMsgr:  emailMsgr,
		importer:   importer,
		auth:       authn,
		media:      mediaStore,
		bounce:     bounceMgr,
		captcha:    initCaptcha(),
		i18n:       i18n,
		log:        lo,
		events:     evStream,
		aiBuilder:  newAIBuilderService(newAIBuilderProvider(), lo),
		bufLog:     bufLog,
		pb:         pb,
		tpl:        tpl,
		renderer:   renderer,

		pg: paginator.New(paginator.Opt{
			DefaultPerPage: 20,
			MaxPerPage:     50,
			NumPageNums:    10,
			PageParam:      "page",
			PerPageParam:   "per_page",
			AllowAll:       true,
		}),

		fnOptinNotify: fbOptinNotify,
		about:         initAbout(db),
		chReload:      chReload,

		needsUserSetup: !hasUser,
	}
	evStream.SetPublishHook(app.publishRealtimeEvent)

	if ko.Bool("app.check_updates") {
		go app.checkUpdates(versionString, time.Hour*24)
	}

	return app
}

func shutdownApp(app *App) {
	if app == nil {
		return
	}
	if app.manager != nil {
		app.manager.Close()
	}
	for _, m := range app.messengers {
		m.Close()
	}
}

func ensureSuperAdminRolePermissions(core *core.Core, cfg *Config) error {
	roles, err := core.GetRoles()
	if err != nil {
		return err
	}

	perms := make([]string, 0, len(cfg.Permissions))
	for p := range cfg.Permissions {
		perms = append(perms, p)
	}

	for _, role := range roles {
		if role.ID != auth.SuperAdminRoleID && (!role.Name.Valid || role.Name.String != "Super Admin") {
			continue
		}

		if sameStringSetMain([]string(role.Permissions), perms) {
			return nil
		}

		role.Permissions = perms
		_, err := core.UpdateUserRole(role.RecordID, role)
		return err
	}

	return nil
}

func sameStringSetMain(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	if len(a) == 0 {
		return true
	}

	seen := make(map[string]int, len(a))
	for _, v := range a {
		seen[v]++
	}

	for _, v := range b {
		n, ok := seen[v]
		if !ok || n == 0 {
			return false
		}
		seen[v]--
	}

	return true
}
