package templatecompiler

import "strings"

const (
	ManifestVersion      = 1
	CompilerVersion      = "1.0.1"
	MaxComposeBytes      = 2 << 20
	MaxSourceFileBytes   = 1 << 20
	MaxBundleBytes       = 8 << 20
	MaxSourceFiles       = 64
	MaxInputs            = 1000
	MaxMappings          = 1000
	PresentationBasic    = "basic"
	PresentationAdvanced = "advanced"
)

func NormalizePresentationGroup(group string) string {
	if strings.EqualFold(strings.TrimSpace(group), PresentationAdvanced) {
		return PresentationAdvanced
	}
	return PresentationBasic
}

type SourceBundle struct {
	ComposePath string       `json:"compose_path"`
	Compose     string       `json:"compose"`
	Dotenv      string       `json:"dotenv,omitempty"`
	Files       []SourceFile `json:"files,omitempty"`
}

type SourceFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Mode    uint32 `json:"mode,omitempty"`
}

type PresentationMetadata map[string]InputPresentation

type InputPresentation struct {
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
	Group       string `json:"group,omitempty"`
	Order       int    `json:"order,omitempty"`
	Sensitive   *bool  `json:"sensitive,omitempty"`
}

type Input struct {
	ID           string            `json:"id"`
	Key          string            `json:"key"`
	Kind         string            `json:"kind"`
	Scope        string            `json:"scope"`
	File         string            `json:"file,omitempty"`
	DefaultValue string            `json:"default_value,omitempty"`
	Required     bool              `json:"required"`
	Sensitive    bool              `json:"sensitive"`
	Presentation InputPresentation `json:"presentation,omitempty"`
}

type Binding struct {
	InputID    string `json:"input_id"`
	Service    string `json:"service,omitempty"`
	TargetKind string `json:"target_kind"`
	TargetKey  string `json:"target_key,omitempty"`
	Expression string `json:"expression,omitempty"`
	YAMLPath   string `json:"yaml_path,omitempty"`
	File       string `json:"file,omitempty"`
	Line       int    `json:"line,omitempty"`
	Column     int    `json:"column,omitempty"`
}

type Mapping struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	Service       string `json:"service"`
	Source        string `json:"source,omitempty"`
	Target        string `json:"target"`
	Protocol      string `json:"protocol,omitempty"`
	Mode          string `json:"mode,omitempty"`
	Editable      bool   `json:"editable"`
	YAMLPath      string `json:"yaml_path,omitempty"`
	SequenceIndex int    `json:"sequence_index,omitempty"`
}

type FixedValue struct {
	Kind     string `json:"kind"`
	Service  string `json:"service,omitempty"`
	YAMLPath string `json:"yaml_path,omitempty"`
	Value    string `json:"value,omitempty"`
}

type OpaqueNode struct {
	YAMLPath string `json:"yaml_path"`
	Reason   string `json:"reason"`
}

type Diagnostic struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	File     string `json:"file,omitempty"`
	YAMLPath string `json:"yaml_path,omitempty"`
	Line     int    `json:"line,omitempty"`
	Column   int    `json:"column,omitempty"`
}

type Manifest struct {
	ManifestVersion int          `json:"manifest_version"`
	CompilerVersion string       `json:"compiler_version"`
	SourceDigest    string       `json:"source_digest"`
	ManifestDigest  string       `json:"manifest_digest"`
	Services        []string     `json:"services"`
	Inputs          []Input      `json:"inputs"`
	Bindings        []Binding    `json:"bindings"`
	Mappings        []Mapping    `json:"mappings"`
	FixedValues     []FixedValue `json:"fixed_values"`
	OpaqueNodes     []OpaqueNode `json:"opaque_nodes"`
	Diagnostics     []Diagnostic `json:"diagnostics"`
	Complete        bool         `json:"complete"`
}

type CompileResult struct {
	Manifest Manifest `json:"manifest"`
}

type LegacyVariable struct {
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

type MappingOverlay struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Service  string `json:"service"`
	Source   string `json:"source,omitempty"`
	Target   string `json:"target"`
	Protocol string `json:"protocol,omitempty"`
	Mode     string `json:"mode,omitempty"`
}

type RenderedBundle struct {
	ComposePath string       `json:"compose_path"`
	Compose     string       `json:"compose"`
	Files       []SourceFile `json:"files"`
}
