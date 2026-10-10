package router

import (
	"os"
	"path/filepath"

	"movieagent/internal/app/gateway/routers"
	"movieagent/internal/config"
	"movieagent/internal/handler"
	"movieagent/internal/middleware"
	svcconfig "movieagent/internal/service/config"
	"movieagent/internal/service/dispatch"
	svcvoice "movieagent/internal/service/voice"

	"github.com/gin-gonic/gin"
)

func New(cfg config.Config) *gin.Engine {
	gin.SetMode(cfg.Mode)

	r := gin.New()
	r.Use(middleware.Recovery(), middleware.CORS(), middleware.RequestLogger())

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
	chat := handler.NewChatHandler(cfg.DeepSeekKey, cfg.DeepSeekBase, cfg.DeepSeekModel)
	chat.ProjectRoot = root
	chat.Dispatch = dispatch.New(dispatch.Config{
		MaxTasks: cfg.DispatchMaxTasks,
		MaxLLM:   cfg.DispatchMaxLLM,
		MaxTool:  cfg.DispatchMaxTool,
		MaxSkill: cfg.DispatchMaxSkill,
	})
	images := handler.NewImageHandler(root)
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
		api.POST("/chat", chat.Chat)
		api.GET("/chat/tasks/:id/events", chat.TaskEvents)
		api.POST("/chat/tasks/:id/pause", chat.PauseTask)
		api.POST("/chat/tasks/:id/resume", chat.ResumeTask)
		api.GET("/chat/tasks/:id/checkpoint", chat.GetCheckpoint)
		api.GET("/chat/tasks/:id/runtime", chat.GetRuntimeSnapshot)
		api.POST("/assets/upload", studio.UploadAsset)
		api.POST("/tasks", studio.CreateTask)
		api.POST("/sessions", studio.CreateRobotSession)
		api.GET("/sessions/:id", studio.GetRobotSession)
		api.POST("/sessions/:id/messages", studio.PromptRobotSession)
		api.GET("/sandbox/file", studio.ServeSandboxFile)
		api.POST("/images/generate", images.Generate)
		api.GET("/voice/status", voice.Status)
		api.POST("/voice/transcribe", voice.Transcribe)
		api.GET("/voice/transcripts", voice.ListTranscripts)
		api.DELETE("/voice/transcripts", voice.ClearTranscripts)
	}

	r.Static("/media/assets", filepath.Join(root, "assets"))
	r.Static("/media/generated", filepath.Join(root, "tmp", "generated"))

	// Official assistant-ui Next.js app (static export → web/chat-ui/out).
	chatOut := filepath.Join(webDir, "chat-ui", "out")
	r.Static("/_next", filepath.Join(chatOut, "_next"))
	serveChat := func(c *gin.Context) {
		index := filepath.Join(chatOut, "index.html")
		if _, err := os.Stat(index); err != nil {
			c.String(503, "chat-ui not built: cd web/chat-ui && npm run build")
			return
		}
		c.File(index)
	}
	r.GET("/", serveChat)
	r.GET("/voice", serveChat)
	r.GET("/index.html", serveChat)

	catalog, err := svcconfig.NewPaths(nil)
	if err != nil {
		panic(err)
	}
	(&routers.Handler{Catalog: catalog}).Register(r)

	return r
}
