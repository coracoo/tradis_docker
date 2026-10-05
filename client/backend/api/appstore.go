package api

import (
	"dockerpanel/backend/internal/templatecompiler"
	"dockerpanel/backend/pkg/cloudflare"
	"time"

	"github.com/gin-gonic/gin"
)

type App struct {
	ID                 uint                                  `json:"id"`
	SortOrder          int                                   `json:"sort_order"`
	CreatedAt          time.Time                             `json:"created_at"`
	UpdatedAt          time.Time                             `json:"updated_at"`
	Name               string                                `json:"name"`
	Category           string                                `json:"category"`
	Description        string                                `json:"description"`
	Version            string                                `json:"version"`
	Logo               string                                `json:"logo"`
	Website            string                                `json:"website"`
	Tutorial           string                                `json:"tutorial"`
	Dotenv             string                                `json:"dotenv"`
	Compose            string                                `json:"compose"`
	SourceFiles        []templatecompiler.SourceFile         `json:"source_files,omitempty"`
	InputMetadata      templatecompiler.PresentationMetadata `json:"input_metadata,omitempty"`
	Manifest           *templatecompiler.Manifest            `json:"manifest,omitempty"`
	SourceDigest       string                                `json:"source_digest,omitempty"`
	ManifestDigest     string                                `json:"manifest_digest,omitempty"`
	CompilerVersion    string                                `json:"compiler_version,omitempty"`
	CompileDiagnostics []templatecompiler.Diagnostic         `json:"compile_diagnostics,omitempty"`
	Screenshots        []string                              `json:"screenshots"`
	Schema             []Variable                            `json:"schema"`
	DeploymentCount    int                                   `json:"deployment_count"`
	Source             string                                `json:"source,omitempty"`
	ShowDeployCount    bool                                  `json:"show_deployment_count"`
}

type Variable struct {
	InputID     string `json:"inputId,omitempty"`
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Default     string `json:"default"`
	Category    string `json:"category"`
	ServiceName string `json:"serviceName"`
	ParamType   string `json:"paramType"`
	EnvFile     string `json:"envFile,omitempty"`
}

func convertCloudflareVariables(cfVars []cloudflare.Variable) []Variable {
	if cfVars == nil {
		return nil
	}
	vars := make([]Variable, len(cfVars))
	for i, v := range cfVars {
		vars[i] = Variable{
			InputID:     v.InputID,
			Name:        v.Name,
			Label:       v.Label,
			Description: v.Description,
			Type:        v.Type,
			Default:     v.Default,
			Category:    v.Category,
			ServiceName: v.ServiceName,
			ParamType:   v.ParamType,
			EnvFile:     v.EnvFile,
		}
	}
	return vars
}

type Port struct {
	Container   int    `json:"container"`
	Host        int    `json:"host"`
	Description string `json:"description"`
}

type Volume struct {
	Container   string `json:"container"`
	Host        string `json:"host"`
	Description string `json:"description"`
}

type EnvVar struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	Description string `json:"description"`
}

type AppVariableInfo struct {
	InputID      string   `json:"inputId,omitempty"`
	Name         string   `json:"name"`
	Value        string   `json:"value"`
	DefaultValue string   `json:"defaultValue"`
	Required     bool     `json:"required"`
	Sources      []string `json:"sources"`
	Examples     []string `json:"examples"`
}

type AppVarsResponse struct {
	App           *App                          `json:"app"`
	Dotenv        string                        `json:"dotenv"`
	Manifest      templatecompiler.Manifest     `json:"manifest"`
	InitialValues map[string]string             `json:"initial_values"`
	Diagnostics   []templatecompiler.Diagnostic `json:"diagnostics"`
	Schema        []Variable                    `json:"schema"`
	Variables     []AppVariableInfo             `json:"variables"`
	Params        []AppParam                    `json:"params,omitempty"`
	Warnings      []string                      `json:"warnings"`
}

type AppParamKind string

const (
	AppParamKindEnv    AppParamKind = "env"
	AppParamKindSecret AppParamKind = "secret"
	AppParamKindPort   AppParamKind = "port"
	AppParamKindBind   AppParamKind = "bind"
	AppParamKindDevice AppParamKind = "device"
)

type AppParamUsage string

const (
	AppParamUsageInterpolation AppParamUsage = "interpolation"
	AppParamUsageRuntimeEnv    AppParamUsage = "runtime_env"
	AppParamUsageSecretMount   AppParamUsage = "secret_mount"
)

type AppParamSource string

const (
	AppParamSourceUserInput      AppParamSource = "user_input"
	AppParamSourceSchema         AppParamSource = "schema"
	AppParamSourceDotenv         AppParamSource = "dotenv"
	AppParamSourceComposeRef     AppParamSource = "compose_ref"
	AppParamSourceComposeDefault AppParamSource = "compose_default"
	AppParamSourceEnvFile        AppParamSource = "env_file"
	AppParamSourceComposeSecret  AppParamSource = "compose_secret"
)

type AppParamBinding struct {
	ServiceName string `json:"serviceName"`
	Target      string `json:"target"`
	File        string `json:"file,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

type AppParam struct {
	InputID      string            `json:"inputId,omitempty"`
	Key          string            `json:"key"`
	Kind         AppParamKind      `json:"kind"`
	Value        string            `json:"value,omitempty"`
	DefaultValue string            `json:"defaultValue,omitempty"`
	Required     bool              `json:"required"`
	Usages       []AppParamUsage   `json:"usages,omitempty"`
	Sources      []AppParamSource  `json:"sources,omitempty"`
	Examples     []string          `json:"examples,omitempty"`
	Bindings     []AppParamBinding `json:"bindings,omitempty"`
}

func RegisterAppStoreRoutes(r *gin.Engine) {
	public := r.Group("/api/appstore")
	{
		public.GET("/meta/status", getAppStoreMetaStatus)
		public.GET("/meta", getAppStoreListMeta)
		public.GET("/apps", listApps)
		public.GET("/apps/:id", getApp)
		public.GET("/apps/:id/vars", getAppVars)
	}
	registerAppStoreEditionPublicRoutes(public)
}

func RegisterAppStoreProtectedRoutes(r *gin.RouterGroup) {
	group := r.Group("/appstore")
	{
		group.GET("/status", getAppStoreStatus)
		group.POST("/parse-vars", parseAppStoreVars)
		group.GET("/deploy/status", listAppStoreTasks)
		group.POST("/deploy/:id", deployApp)
		group.GET("/status/:id", getAppStatus)
		group.GET("/tasks", listAppStoreTasks)
		group.GET("/tasks/:id", getAppStoreTask)
		group.POST("/tasks/:id/cancel", cancelComposeTask)
		group.GET("/tasks/:id/events", taskEvents)
	}
	registerAppStoreEditionProtectedRoutes(group)
}
