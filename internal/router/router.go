package router

import (
	"os"
	"path/filepath"

	"movieagent/internal/app/gateway/routers"
	"movieagent/internal/config"
	"movieagent/internal/handler"
	"movieagent/internal/middleware"
	svcconfig "movieagent/internal/service/config"
	svcvoice "movieagent/internal/service/voice"

	"github.com/gin-gonic/gin"
)

func New(cfg config.Config) *gin.Engine {
	gin.SetMode(cfg.Mode)

	r := gin.New()
	r.Use(middleware.Recovery(), middleware.RequestLogger())

	root, _ := os.Getwd()
	webDir := filepath.Join(root, "web")
	assetsYAML := cfg.ModelsConfigPath
	if filepath.Base(assetsYAML) == "models.yaml" {
		assetsYAML = filepath.Join(filepath.Dir(assetsYAML), "assets", "assets.yaml")
	} else {
		assetsYAML = filepath.Join(root, "internal", "service", "models", "assets", "assets.yaml")
	}

	health := handler.NewHealthHandler(cfg.AppName)
	studio := handler.NewStudioHandler(root, assetsYAML)
	stt := svcvoice.New(svcvoice.Options{
		ProjectRoot: root,
		Python:      cfg.VoicePython,
		Model:       cfg.WhisperModel,
	})
	voice := handler.NewVoiceHandler(stt)

	api := r.Group("/api/v1")
	{
		api.GET("/health", health.Ping)
		api.GET("/ping", health.Ping)
		api.GET("/agents", studio.ListAgents)
		api.GET("/brands", studio.ListBrands)
		api.POST("/assets/upload", studio.UploadAsset)
		api.POST("/tasks", studio.CreateTask)
		api.POST("/sessions", studio.CreateRobotSession)
		api.GET("/sessions/:id", studio.GetRobotSession)
		api.POST("/sessions/:id/messages", studio.PromptRobotSession)
		api.GET("/sandbox/file", studio.ServeSandboxFile)
		api.GET("/voice/status", voice.Status)
		api.POST("/voice/transcribe", voice.Transcribe)
		api.GET("/voice/transcripts", voice.ListTranscripts)
		api.DELETE("/voice/transcripts", voice.ClearTranscripts)
	}

	r.Static("/static", webDir)
	r.Static("/media/assets", filepath.Join(root, "assets"))
	r.GET("/", func(c *gin.Context) {
		c.File(filepath.Join(webDir, "index.html"))
	})
	r.GET("/voice", func(c *gin.Context) {
		c.File(filepath.Join(webDir, "voice.html"))
	})

	catalog, err := svcconfig.NewPaths(nil)
	if err != nil {
		panic(err)
	}
	(&routers.Handler{Catalog: catalog}).Register(r)

	return r
}
