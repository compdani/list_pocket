package workflow

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
)

type Config struct {
	FrontendDir string
}

func Register(pb *pocketbase.PocketBase, cfg Config) {
	pb.OnRecordAfterUpdateSuccess("subscribers").BindFunc(func(e *core.RecordEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		return handleSubscriberTagWorkflowTriggers(e.App, e.Record)
	})

	pb.OnServe().Bind(&hook.Handler[*core.ServeEvent]{
		Func: func(e *core.ServeEvent) error {
			startRunWorker(e.App)

			authGroup := e.Router.Group("").Bind(apis.RequireAuth())
			authGroup.GET("/api/control-plane/dashboard", dashboardHandler)
			authGroup.POST("/api/control-plane/workflows/{id}/save", saveWorkflowHandler)
			authGroup.POST("/api/control-plane/workflows/{id}/validate", validateWorkflowHandler)
			authGroup.POST("/api/control-plane/workflows/{id}/publish", publishWorkflowHandler)
			authGroup.POST("/api/control-plane/workflows/{id}/run", runWorkflowHandler)
			authGroup.POST("/api/control-plane/workflows/{id}/webhook-capture", armWebhookCaptureHandler)
			authGroup.GET("/api/control-plane/webhook-captures/{sessionId}", getWebhookCaptureHandler)
			authGroup.GET("/api/control-plane/runs/{id}", getRunDetailHandler)

			e.Router.POST("/api/hooks/{hookPath...}", webhookTriggerHandler)
			registerFrontendRoutes(e, cfg.FrontendDir)
			return e.Next()
		},
		Priority: 998,
	})
}

func dashboardHandler(re *core.RequestEvent) error {
	payload, err := buildDashboardPayload(re.App, re.Request.URL.Query().Get("workflowId"))
	if err != nil {
		return re.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return re.JSON(http.StatusOK, payload)
}

func registerFrontendRoutes(e *core.ServeEvent, frontendDir string) {
	if frontendDir == "" {
		return
	}
	frontendDir = filepath.Clean(frontendDir)
	indexPath := filepath.Join(frontendDir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		return
	}

	e.Router.GET("/workflow/assets/{path...}", apis.Static(os.DirFS(filepath.Join(frontendDir, "assets")), false))
	e.Router.GET("/workflow", func(re *core.RequestEvent) error {
		http.ServeFile(re.Response, re.Request, indexPath)
		return nil
	})
	e.Router.GET("/workflow/{path...}", func(re *core.RequestEvent) error {
		requestPath := filepath.Clean(re.Request.PathValue("path"))
		if requestPath == "." || requestPath == "" {
			http.ServeFile(re.Response, re.Request, indexPath)
			return nil
		}

		target := filepath.Join(frontendDir, requestPath)
		if info, err := os.Stat(target); err == nil && !info.IsDir() {
			http.ServeFile(re.Response, re.Request, target)
			return nil
		}

		http.ServeFile(re.Response, re.Request, indexPath)
		return nil
	})
}
