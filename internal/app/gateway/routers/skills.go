package routers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"

	"movieagent/internal/app/gateway/skillexport"
	svcconfig "movieagent/internal/service/config"
	svcskills "movieagent/internal/service/skills"

	"github.com/gin-gonic/gin"
)

const (
	adminRequiredDetail = "Admin privileges required to manage skills"
	skillMDFile         = "SKILL.md"

	maxSkillArchiveBytes             int64 = 1024 * 1024 * 1024
	maxSkillArchiveMultipartOverhead int64 = 1024 * 1024
	uploadCopyChunkBytes                   = 1024 * 1024
)

var (
	threadIDPattern    = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
	revisionPattern    = regexp.MustCompile(`^[a-f0-9]{64}$`)
	errArchiveTooLarge = errors.New("skill archive too large")
)

type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string {
	return e.Message
}

type InvalidInputError struct {
	Message string
}

func (e *InvalidInputError) Error() string {
	return e.Message
}

type AlreadyExistsError struct {
	Message string
}

func (e *AlreadyExistsError) Error() string {
	return e.Message
}

type SecurityScanError struct {
	Message   string
	SkillName *string
	Findings  []map[string]any
}

func (e *SecurityScanError) Error() string {
	return e.Message
}

type StaticScanBlockedError struct {
	Message   string
	SkillName *string
	Findings  []map[string]any
}

func (e *StaticScanBlockedError) Error() string {
	return e.Message
}

type StaticScannerError struct {
	Message string
}

func (e *StaticScannerError) Error() string {
	return e.Message
}

type StatusError struct {
	Status int
	Detail any
}

func (e *StatusError) Error() string {
	return fmt.Sprint(e.Detail)
}

type Skill struct {
	Name        string
	Description string
	License     string
	Category    string
	Enabled     bool
}

type SkillResponse struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	License     *string `json:"license"`
	Category    string  `json:"category"`
	Enabled     bool    `json:"enabled"`
	Editable    bool    `json:"editable"`
}

func (s Skill) Response() SkillResponse {
	license := s.License
	return SkillResponse{
		Name:        s.Name,
		Description: s.Description,
		License:     &license,
		Category:    s.Category,
		Enabled:     s.Enabled,
	}
}

type SkillListResponse struct {
	Skills []SkillResponse `json:"skills"`
}

type SkillInstallResponse struct {
	Success   bool   `json:"success"`
	SkillName string `json:"skill_name"`
	Message   string `json:"message"`
}

type SkillReloadResponse struct {
	Success bool   `json:"success"`
	Scope   string `json:"scope"`
	Message string `json:"message"`
}

type CustomSkillContentResponse struct {
	SkillResponse
	Content string `json:"content"`
}

type CustomSkillHistoryResponse struct {
	History []map[string]any `json:"history"`
}

type SkillStorage interface {
	LoadSkills(enbaledOnly bool) ([]Skill, error)
	InstallFromArchive(archivePath string) (SkillInstallResponse, error)
	ReadCustomSkill(name string) (string, error)
	WriteCustomSkill(name, relativePath string, content string) error
	AppendHistory(name, record map[string]any) error
	DeleteCustomSkill(name string, historyMeta map[string]any) error
	CustomSkillExists(name string) (bool, error)
	HistoryExists(name string) (bool, error)
	ReadHistory(name string) ([]map[string]any, error)
	EnsureCustomSkillEditable(name string) error
	ValidateSkillMarkdown(name, content string) error
	ReadCustomSkillMarkdown(name string) (*string, error)
	UserScoped() bool
	SetSkillEnabled(name string, enabled bool) error
}

type PathResolver interface {
	ResolveThreadVirtualPath(threadID, virtualPath string) (string, error)
}

type Scanner interface {
	ScanStaticMarkdown(name, content string) ([]map[string]any, error)
	ScanContent(content string, executable bool, location string, staticFindings []map[string]any) (ScanResults, error)
}

type ScanResults struct{}

type PromptCache interface {
	RefreshAll(ctx context.Context) error
	RefreshUser(ctx context.Context, userID string) error
	ClearAll(ctx context.Context) error
}

type ExtensionsConfig interface {
	WriteSkillEnabled(storage SkillStorage, skillName string, enabled, rebuildPublicProjection bool) error
}

type Expoter interface {
	ExportManifest(storage SkillStorage, skillName string, cancel *skillexport.CancelEvent) (*skillexport.SkillExportManifestResponse, error)
	BuildExport(storage SkillStorage, skillName, expectedRevision string, cancel *skillexport.CancelEvent) (*skillexport.SkillExportArchive, error)
}

type Handler struct {
	UserID       func(*http.Request) string
	RequireAdmin func(*http.Request) error
	NewStorage   func(userID string) SkillStorage
	Paths        PathResolver
	Scanner      Scanner
	Prompt       PromptCache
	Extensions   ExtensionsConfig
	Export       Expoter
	// Catalog is the host path layout used to list skills.
	Catalog *svcconfig.Paths
}

func (h *Handler) Register(r gin.IRouter) {
	api := r.Group("/api")
	api.GET("/skills", h.ListSkills)
	api.POST("/skills/install", h.InstallSkill)
	api.POST("/skills/install/upload", h.UploadAndInstall)
	api.POST("/skills/reload", h.ReloadSkills)
	api.GET("/skills/custom", h.ListCustomSkills)
	api.GET("/skills/custom/:skill_name/export-manifest", h.PreviewCustomSkillExport)
	api.GET("/skills/custom/:skill_name/export", h.DownloadCustomSkillExport)
	api.GET("/skills/custom/:skill_name/history", h.GetCustomSkillHistory)
	api.POST("/skills/custom/:skill_name/rollback", h.RollbackCustomSkill)
	api.GET("/skills/custom/:skill_name", h.GetCustomSkill)
	api.PUT("/skills/custom/:skill_name", h.UpdateCustomSkill)
	api.DELETE("/skills/custom/:skill_name", h.DeleteCustomSkill)
	api.GET("/skills/:skill_name", h.GetSkill)
	api.PUT("/skills/:skill_name", h.UpdateSkill)
}

func (h *Handler) ListSkills(c *gin.Context) {
	catalog := h.Catalog
	if catalog == nil {
		var err error
		catalog, err = svcconfig.NewPaths(nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error()})
			return
		}
	}
	userID := ""
	if h.UserID != nil {
		userID = h.UserID(c.Request)
	}
	if userID == "" {
		userID = c.Query("user_id")
	}

	loaded, err := svcskills.LoadCatalog(catalog, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error()})
		return
	}

	resp := SkillListResponse{Skills: make([]SkillResponse, 0, len(loaded))}
	for _, skill := range loaded {
		resp.Skills = append(resp.Skills, SkillResponse{
			Name:        skill.Name,
			Description: skill.Description,
			License:     licensePtr(skill.License),
			Category:    skill.Category,
			Enabled:     skill.Enabled,
			Editable:    skill.Editable,
		})
	}
	c.JSON(http.StatusOK, resp)
}

func licensePtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
func (h *Handler) InstallSkill(c *gin.Context)             { notImplemented(c) }
func (h *Handler) UploadAndInstall(c *gin.Context)         { notImplemented(c) }
func (h *Handler) ReloadSkills(c *gin.Context)             { notImplemented(c) }
func (h *Handler) ListCustomSkills(c *gin.Context)         { notImplemented(c) }
func (h *Handler) PreviewCustomSkillExport(c *gin.Context) { notImplemented(c) }
func (h *Handler) DownloadCustomSkillExport(c *gin.Context) {
	notImplemented(c)
}
func (h *Handler) GetCustomSkillHistory(c *gin.Context) { notImplemented(c) }
func (h *Handler) RollbackCustomSkill(c *gin.Context)   { notImplemented(c) }
func (h *Handler) GetCustomSkill(c *gin.Context)        { notImplemented(c) }
func (h *Handler) UpdateCustomSkill(c *gin.Context)     { notImplemented(c) }
func (h *Handler) DeleteCustomSkill(c *gin.Context)     { notImplemented(c) }
func (h *Handler) GetSkill(c *gin.Context)              { notImplemented(c) }
func (h *Handler) UpdateSkill(c *gin.Context)           { notImplemented(c) }

func notImplemented(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"code":    "not_implemented",
		"message": "skill route is registered but handler is not implemented yet",
	})
}
