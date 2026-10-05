package templatecompiler

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type compiler struct {
	bundle      SourceBundle
	metadata    PresentationMetadata
	manifest    Manifest
	inputs      map[string]*Input
	mappingIDs  map[string]int
	files       map[string]SourceFile
	dotenv      map[string]string
	secretPaths map[string]string
}

func Compile(bundle SourceBundle, metadata PresentationMetadata) (CompileResult, error) {
	normalized, diagnostics, err := normalizeSourceBundle(bundle)
	if err != nil {
		return CompileResult{}, err
	}
	digest, err := sourceDigest(normalized)
	if err != nil {
		return CompileResult{}, err
	}
	c := &compiler{
		bundle:      normalized,
		metadata:    metadata,
		inputs:      make(map[string]*Input),
		mappingIDs:  make(map[string]int),
		files:       make(map[string]SourceFile),
		secretPaths: make(map[string]string),
		manifest: Manifest{
			ManifestVersion: ManifestVersion,
			CompilerVersion: CompilerVersion,
			SourceDigest:    digest,
			Inputs:          []Input{},
			Bindings:        []Binding{},
			Mappings:        []Mapping{},
			FixedValues:     []FixedValue{},
			OpaqueNodes:     []OpaqueNode{},
			Diagnostics:     diagnostics,
		},
	}
	for _, file := range normalized.Files {
		c.files[file.Path] = file
	}
	c.dotenv, diagnostics = parseDotenv(normalized.Dotenv, ".env")
	c.manifest.Diagnostics = append(c.manifest.Diagnostics, diagnostics...)
	for key, value := range c.dotenv {
		c.ensureInput(Input{ID: "project:" + key, Key: key, Kind: "env", Scope: "project", File: ".env", DefaultValue: value, Sensitive: sensitiveKey(key)})
	}

	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(normalized.Compose), &doc); err != nil {
		c.error("invalid_compose_yaml", err.Error(), normalized.ComposePath, "", 0, 0)
	} else if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		c.error("invalid_compose_root", "Compose root must be a mapping", normalized.ComposePath, "", 0, 0)
	} else {
		root := doc.Content[0]
		c.parseSecrets(root)
		c.parseServices(root)
		c.scanNode(root, nil)
		c.recordOpaque(root, nil)
	}
	c.finish()
	if err := setManifestDigest(&c.manifest); err != nil {
		return CompileResult{}, err
	}
	return CompileResult{Manifest: c.manifest}, nil
}

func Verify(bundle SourceBundle, manifest Manifest) error {
	if manifest.ManifestVersion != ManifestVersion {
		return fmt.Errorf("unsupported manifest version %d", manifest.ManifestVersion)
	}
	if manifest.CompilerVersion != CompilerVersion {
		return fmt.Errorf("unsupported compiler version %q", manifest.CompilerVersion)
	}
	normalized, _, err := normalizeSourceBundle(bundle)
	if err != nil {
		return err
	}
	digest, err := sourceDigest(normalized)
	if err != nil {
		return err
	}
	if digest != manifest.SourceDigest {
		return fmt.Errorf("source digest mismatch")
	}
	expected, err := manifestDigest(manifest)
	if err != nil {
		return err
	}
	if expected != manifest.ManifestDigest {
		return fmt.Errorf("manifest digest mismatch")
	}
	metadata := make(PresentationMetadata)
	for _, input := range manifest.Inputs {
		presentation := input.Presentation
		if presentation.Label != "" || presentation.Description != "" || presentation.Group != "" || presentation.Order != 0 || presentation.Sensitive != nil {
			metadata[input.ID] = presentation
		}
	}
	compiled, err := Compile(normalized, metadata)
	if err != nil {
		return err
	}
	if compiled.Manifest.ManifestDigest != manifest.ManifestDigest {
		return fmt.Errorf("manifest does not match deterministic source compilation")
	}
	return nil
}

func ProjectLegacySchema(manifest Manifest) []LegacyVariable {
	result := make([]LegacyVariable, 0, len(manifest.Inputs))
	for _, input := range manifest.Inputs {
		service := "Global"
		paramType := input.Kind
		if input.Scope == "service" {
			parts := strings.SplitN(strings.TrimPrefix(input.ID, "service:"), ":", 2)
			if len(parts) > 0 && parts[0] != "" {
				service = parts[0]
			}
		}
		for _, mapping := range manifest.Mappings {
			if mapping.ID == input.ID {
				service = mapping.Service
				paramType = mapping.Kind
				if paramType == "bind" {
					paramType = "path"
				}
				break
			}
		}
		fieldType := "string"
		if input.Sensitive {
			fieldType = "password"
		} else if paramType == "port" {
			fieldType = "port"
		} else if paramType == "path" {
			fieldType = "path"
		}
		label := input.Presentation.Label
		if label == "" {
			label = input.Key
		}
		category := input.Presentation.Group
		if category == "" {
			category = "basic"
		}
		result = append(result, LegacyVariable{InputID: input.ID, Name: input.Key, Label: label, Description: input.Presentation.Description, Type: fieldType, Default: input.DefaultValue, Category: category, ServiceName: service, ParamType: paramType, EnvFile: input.File})
	}
	return result
}

func normalizeSourceBundle(bundle SourceBundle) (SourceBundle, []Diagnostic, error) {
	bundle.ComposePath = normalizeRelativePath(bundle.ComposePath)
	if bundle.ComposePath == "" {
		bundle.ComposePath = "compose.yaml"
	}
	if !safeRelativePath(bundle.ComposePath) {
		return bundle, nil, fmt.Errorf("unsafe Compose path %q", bundle.ComposePath)
	}
	if len(bundle.Compose) > MaxComposeBytes {
		return bundle, nil, fmt.Errorf("Compose exceeds %d bytes", MaxComposeBytes)
	}
	if len(bundle.Files) > MaxSourceFiles {
		return bundle, nil, fmt.Errorf("source file count exceeds %d", MaxSourceFiles)
	}
	total := len(bundle.Compose) + len(bundle.Dotenv)
	seen := map[string]struct{}{bundle.ComposePath: {}}
	for i := range bundle.Files {
		file := &bundle.Files[i]
		file.Path = normalizeRelativePath(file.Path)
		if file.Path == "" || !safeRelativePath(file.Path) {
			return bundle, nil, fmt.Errorf("unsafe source file path %q", file.Path)
		}
		if file.Path == ".env" {
			return bundle, nil, fmt.Errorf("reserved source file path %q", file.Path)
		}
		if _, exists := seen[file.Path]; exists {
			return bundle, nil, fmt.Errorf("duplicate source file path %q", file.Path)
		}
		seen[file.Path] = struct{}{}
		if len(file.Content) > MaxSourceFileBytes {
			return bundle, nil, fmt.Errorf("source file %s exceeds %d bytes", file.Path, MaxSourceFileBytes)
		}
		total += len(file.Content)
	}
	if total > MaxBundleBytes {
		return bundle, nil, fmt.Errorf("source bundle exceeds %d bytes", MaxBundleBytes)
	}
	sort.Slice(bundle.Files, func(i, j int) bool { return bundle.Files[i].Path < bundle.Files[j].Path })
	return bundle, []Diagnostic{}, nil
}

func normalizeRelativePath(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if value == "" {
		return ""
	}
	clean := path.Clean(value)
	if clean == "." {
		return ""
	}
	return strings.TrimPrefix(clean, "./")
}

func safeRelativePath(value string) bool {
	if value == "" || strings.HasPrefix(value, "/") || strings.ContainsRune(value, 0) {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == ".." || part == "" {
			return false
		}
	}
	return true
}

func (c *compiler) parseSecrets(root *yaml.Node) {
	secrets := mappingValue(root, "secrets")
	if secrets == nil || secrets.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(secrets.Content); i += 2 {
		name := strings.TrimSpace(secrets.Content[i].Value)
		definition := secrets.Content[i+1]
		if name == "" || definition.Kind != yaml.MappingNode {
			continue
		}
		if scalarBool(mappingValue(definition, "external")) {
			c.secretPaths[name] = "external:"
			continue
		}
		fileNode := mappingValue(definition, "file")
		if fileNode == nil || fileNode.Kind != yaml.ScalarNode {
			c.error("invalid_secret_definition", fmt.Sprintf("secret %s must be external or file-backed", name), c.bundle.ComposePath, "secrets."+name, definition.Line, definition.Column)
			continue
		}
		secretPath := normalizeRelativePath(fileNode.Value)
		if !safeRelativePath(secretPath) {
			c.error("unsafe_secret_path", fmt.Sprintf("secret %s has an unsafe file path", name), c.bundle.ComposePath, "secrets."+name+".file", fileNode.Line, fileNode.Column)
			continue
		}
		c.secretPaths[name] = secretPath
		if _, exists := c.files[secretPath]; exists {
			c.error("secret_content_in_source", fmt.Sprintf("managed secret %s content must not be stored in template source", name), secretPath, "", 0, 0)
		}
	}
}

func (c *compiler) parseServices(root *yaml.Node) {
	services := mappingValue(root, "services")
	if services == nil || services.Kind != yaml.MappingNode || len(services.Content) == 0 {
		c.error("missing_services", "Compose must contain a non-empty services mapping", c.bundle.ComposePath, "services", 0, 0)
		return
	}
	for i := 0; i+1 < len(services.Content); i += 2 {
		service := strings.TrimSpace(services.Content[i].Value)
		node := services.Content[i+1]
		if service == "" || node.Kind != yaml.MappingNode {
			continue
		}
		c.manifest.Services = append(c.manifest.Services, service)
		c.parseServiceEnvFiles(service, node)
		c.parseServiceEnvironment(service, node)
		c.parseServiceMappings(service, node)
		c.parseServiceSecrets(service, node)
	}
}

func (c *compiler) parseServiceEnvFiles(service string, node *yaml.Node) {
	envNode := mappingValue(node, "env_file")
	if envNode == nil {
		return
	}
	type ref struct {
		path     string
		required bool
	}
	refs := make([]ref, 0)
	add := func(value *yaml.Node, required bool) {
		if value == nil || value.Kind != yaml.ScalarNode {
			return
		}
		refs = append(refs, ref{normalizeRelativePath(value.Value), required})
	}
	switch envNode.Kind {
	case yaml.ScalarNode:
		add(envNode, true)
	case yaml.SequenceNode:
		for _, item := range envNode.Content {
			if item.Kind == yaml.ScalarNode {
				add(item, true)
			} else if item.Kind == yaml.MappingNode {
				add(mappingValue(item, "path"), !mappingHasFalse(item, "required"))
			}
		}
	}
	for _, item := range refs {
		if !safeRelativePath(item.path) {
			c.error("unsafe_env_file_path", fmt.Sprintf("service %s has unsafe env_file path", service), c.bundle.ComposePath, "services."+service+".env_file", envNode.Line, envNode.Column)
			continue
		}
		file, exists := c.files[item.path]
		if !exists {
			severity := "warning"
			if item.required {
				severity = "error"
			}
			c.manifest.Diagnostics = append(c.manifest.Diagnostics, Diagnostic{Code: "missing_env_file", Severity: severity, Message: fmt.Sprintf("service %s env_file %s is not present in source bundle", service, item.path), File: item.path})
			continue
		}
		values, diagnostics := parseDotenv(file.Content, item.path)
		c.manifest.Diagnostics = append(c.manifest.Diagnostics, diagnostics...)
		for key, value := range values {
			id := "env_file:" + item.path + ":" + key
			c.ensureInput(Input{ID: id, Key: key, Kind: "env", Scope: "env_file", File: item.path, DefaultValue: value, Required: item.required && value == "", Sensitive: sensitiveKey(key)})
			c.manifest.Bindings = append(c.manifest.Bindings, Binding{InputID: id, Service: service, TargetKind: "env_file", TargetKey: key, File: item.path})
		}
	}
}

func (c *compiler) parseServiceEnvironment(service string, node *yaml.Node) {
	envNode := mappingValue(node, "environment")
	if envNode == nil {
		return
	}
	addLiteral := func(key, value, yamlPath string, source *yaml.Node) {
		if key == "" || len(scanInterpolations(value)) > 0 {
			return
		}
		id := "service:" + service + ":" + key
		c.ensureInput(Input{ID: id, Key: key, Kind: "env", Scope: "service", DefaultValue: value, Sensitive: sensitiveKey(key)})
		c.manifest.Bindings = append(c.manifest.Bindings, Binding{InputID: id, Service: service, TargetKind: "environment", TargetKey: key, YAMLPath: yamlPath, Line: source.Line, Column: source.Column})
	}
	switch envNode.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(envNode.Content); i += 2 {
			key := strings.TrimSpace(envNode.Content[i].Value)
			addLiteral(key, envNode.Content[i+1].Value, "services."+service+".environment."+key, envNode.Content[i+1])
		}
	case yaml.SequenceNode:
		for i, item := range envNode.Content {
			parts := strings.SplitN(item.Value, "=", 2)
			if len(parts) == 2 {
				addLiteral(strings.TrimSpace(parts[0]), parts[1], fmt.Sprintf("services.%s.environment[%d]", service, i), item)
			}
		}
	}
}

func (c *compiler) parseServiceMappings(service string, node *yaml.Node) {
	c.parsePorts(service, mappingValue(node, "ports"))
	c.parseVolumes(service, mappingValue(node, "volumes"))
	c.parseDevices(service, mappingValue(node, "devices"))
}

func (c *compiler) parsePorts(service string, node *yaml.Node) {
	if node == nil || node.Kind != yaml.SequenceNode {
		return
	}
	for i, item := range node.Content {
		source, target, protocol := "", "", "tcp"
		if item.Kind == yaml.ScalarNode {
			value := strings.TrimSpace(item.Value)
			if slash := strings.LastIndex(value, "/"); slash >= 0 {
				protocol, value = value[slash+1:], value[:slash]
			}
			parts := strings.Split(value, ":")
			if len(parts) >= 2 {
				source, target = parts[len(parts)-2], parts[len(parts)-1]
			} else if len(parts) == 1 {
				target = parts[0]
			}
		} else if item.Kind == yaml.MappingNode {
			source = scalarString(mappingValue(item, "published"))
			target = scalarString(mappingValue(item, "target"))
			if p := scalarString(mappingValue(item, "protocol")); p != "" {
				protocol = p
			}
		}
		if target == "" {
			continue
		}
		id := c.uniqueMappingID(fmt.Sprintf("port:%s:%s/%s", service, target, protocol))
		c.manifest.Mappings = append(c.manifest.Mappings, Mapping{ID: id, Kind: "port", Service: service, Source: source, Target: target, Protocol: protocol, Editable: source != "", YAMLPath: fmt.Sprintf("services.%s.ports[%d]", service, i), SequenceIndex: i})
		if source != "" {
			c.ensureInput(Input{ID: id, Key: source, Kind: "port", Scope: "service", DefaultValue: source})
		}
	}
}

func (c *compiler) parseVolumes(service string, node *yaml.Node) {
	if node == nil || node.Kind != yaml.SequenceNode {
		return
	}
	for i, item := range node.Content {
		source, target, mode, kind := "", "", "", ""
		if item.Kind == yaml.ScalarNode {
			parts := strings.Split(item.Value, ":")
			if len(parts) >= 2 {
				source, target = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
				if len(parts) > 2 {
					mode = strings.TrimSpace(parts[2])
				}
			}
		} else if item.Kind == yaml.MappingNode {
			source = scalarString(mappingValue(item, "source"))
			target = scalarString(mappingValue(item, "target"))
			mode = scalarString(mappingValue(item, "read_only"))
			kind = scalarString(mappingValue(item, "type"))
		}
		if target == "" {
			continue
		}
		if kind == "" {
			kind = "volume"
			if strings.HasPrefix(source, ".") || strings.HasPrefix(source, "/") || strings.HasPrefix(source, "~") {
				kind = "bind"
			}
		}
		id := c.uniqueMappingID(fmt.Sprintf("%s:%s:%s", kind, service, target))
		editable := kind == "bind"
		c.manifest.Mappings = append(c.manifest.Mappings, Mapping{ID: id, Kind: kind, Service: service, Source: source, Target: target, Mode: mode, Editable: editable, YAMLPath: fmt.Sprintf("services.%s.volumes[%d]", service, i), SequenceIndex: i})
		if editable {
			c.ensureInput(Input{ID: id, Key: source, Kind: "bind", Scope: "service", DefaultValue: source})
		}
	}
}

func (c *compiler) parseDevices(service string, node *yaml.Node) {
	if node == nil || node.Kind != yaml.SequenceNode {
		return
	}
	for i, item := range node.Content {
		parts := strings.Split(item.Value, ":")
		if len(parts) < 2 {
			continue
		}
		source, target := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		id := c.uniqueMappingID(fmt.Sprintf("device:%s:%s", service, target))
		c.manifest.Mappings = append(c.manifest.Mappings, Mapping{ID: id, Kind: "device", Service: service, Source: source, Target: target, Editable: true, YAMLPath: fmt.Sprintf("services.%s.devices[%d]", service, i), SequenceIndex: i})
		c.ensureInput(Input{ID: id, Key: source, Kind: "device", Scope: "service", DefaultValue: source})
	}
}

func (c *compiler) parseServiceSecrets(service string, node *yaml.Node) {
	secretNode := mappingValue(node, "secrets")
	if secretNode == nil || secretNode.Kind != yaml.SequenceNode {
		return
	}
	for _, item := range secretNode.Content {
		name := strings.TrimSpace(item.Value)
		if item.Kind == yaml.MappingNode {
			name = scalarString(mappingValue(item, "source"))
		}
		secretPath, exists := c.secretPaths[name]
		if !exists {
			c.error("unresolved_secret", fmt.Sprintf("service %s references undefined secret %s", service, name), c.bundle.ComposePath, "services."+service+".secrets", item.Line, item.Column)
			continue
		}
		if secretPath == "external:" {
			c.manifest.FixedValues = append(c.manifest.FixedValues, FixedValue{Kind: "secret_external", Service: service, Value: name})
			continue
		}
		id := "secret:" + name
		c.ensureInput(Input{ID: id, Key: name, Kind: "secret", Scope: "secret", File: secretPath, Required: true, Sensitive: true})
		c.manifest.Bindings = append(c.manifest.Bindings, Binding{InputID: id, Service: service, TargetKind: "secret_file", TargetKey: name, File: secretPath})
	}
}

func (c *compiler) scanNode(node *yaml.Node, parts []string) {
	if node == nil {
		return
	}
	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i].Value
			c.scanNode(node.Content[i+1], append(parts, key))
		}
	case yaml.SequenceNode:
		for i, child := range node.Content {
			c.scanNode(child, append(parts, "["+strconv.Itoa(i)+"]"))
		}
	case yaml.ScalarNode:
		for _, ref := range scanInterpolations(node.Value) {
			id := "project:" + ref.Name
			input := Input{ID: id, Key: ref.Name, Kind: "env", Scope: "project", File: ".env", DefaultValue: c.dotenv[ref.Name], Required: ref.Required, Sensitive: sensitiveKey(ref.Name)}
			if input.DefaultValue == "" && ref.HasDefault {
				input.DefaultValue = ref.Default
			}
			if input.DefaultValue == "" && !ref.HasDefault {
				input.Required = true
			}
			c.ensureInput(input)
			service := ""
			if len(parts) >= 2 && parts[0] == "services" {
				service = parts[1]
			}
			targetKey := ""
			if len(parts) > 0 {
				targetKey = parts[len(parts)-1]
			}
			c.manifest.Bindings = append(c.manifest.Bindings, Binding{InputID: id, Service: service, TargetKind: "interpolation", TargetKey: targetKey, Expression: ref.Raw, YAMLPath: yamlPath(parts), Line: node.Line, Column: node.Column})
		}
	}
}

func (c *compiler) recordOpaque(root *yaml.Node, parts []string) {
	if root == nil || root.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		key := root.Content[i].Value
		next := append(parts, key)
		if strings.HasPrefix(key, "x-") {
			c.manifest.OpaqueNodes = append(c.manifest.OpaqueNodes, OpaqueNode{YAMLPath: yamlPath(next), Reason: "extension field preserved in source"})
		}
		c.recordOpaque(root.Content[i+1], next)
	}
}

func (c *compiler) ensureInput(input Input) {
	if input.ID == "" || len(c.inputs) >= MaxInputs {
		return
	}
	if existing, ok := c.inputs[input.ID]; ok {
		if existing.DefaultValue == "" {
			existing.DefaultValue = input.DefaultValue
		}
		existing.Required = existing.Required || input.Required
		existing.Sensitive = existing.Sensitive || input.Sensitive
		return
	}
	if metadata, ok := c.metadata[input.ID]; ok {
		input.Presentation = metadata
		if metadata.Sensitive != nil {
			input.Sensitive = *metadata.Sensitive
		}
	}
	copyInput := input
	c.inputs[input.ID] = &copyInput
}

func (c *compiler) uniqueMappingID(base string) string {
	c.mappingIDs[base]++
	if c.mappingIDs[base] == 1 {
		return base
	}
	return fmt.Sprintf("%s#%d", base, c.mappingIDs[base])
}

func (c *compiler) finish() {
	sort.Strings(c.manifest.Services)
	for _, input := range c.inputs {
		input.Presentation.Group = NormalizePresentationGroup(input.Presentation.Group)
		if input.Required && input.Presentation.Group == PresentationAdvanced {
			input.Presentation.Group = PresentationBasic
			c.manifest.Diagnostics = append(c.manifest.Diagnostics, Diagnostic{
				Code:     "required_input_promoted_to_basic",
				Severity: "warning",
				Message:  fmt.Sprintf("required input %s was promoted to basic configuration", input.ID),
				YAMLPath: "inputs." + input.ID + ".presentation.group",
			})
		}
		c.manifest.Inputs = append(c.manifest.Inputs, *input)
	}
	sort.Slice(c.manifest.Inputs, func(i, j int) bool { return c.manifest.Inputs[i].ID < c.manifest.Inputs[j].ID })
	sort.Slice(c.manifest.Bindings, func(i, j int) bool {
		a, b := c.manifest.Bindings[i], c.manifest.Bindings[j]
		return a.InputID+"\x00"+a.YAMLPath+"\x00"+a.Service < b.InputID+"\x00"+b.YAMLPath+"\x00"+b.Service
	})
	sort.Slice(c.manifest.Mappings, func(i, j int) bool { return c.manifest.Mappings[i].ID < c.manifest.Mappings[j].ID })
	sort.Slice(c.manifest.Diagnostics, func(i, j int) bool {
		a, b := c.manifest.Diagnostics[i], c.manifest.Diagnostics[j]
		return a.Severity+"\x00"+a.Code+"\x00"+a.File+"\x00"+a.YAMLPath < b.Severity+"\x00"+b.Code+"\x00"+b.File+"\x00"+b.YAMLPath
	})
	c.manifest.Complete = true
	for _, diagnostic := range c.manifest.Diagnostics {
		if diagnostic.Severity == "error" {
			c.manifest.Complete = false
			break
		}
	}
}

func (c *compiler) error(code, message, file, yamlPath string, line, column int) {
	c.manifest.Diagnostics = append(c.manifest.Diagnostics, Diagnostic{Code: code, Severity: "error", Message: message, File: file, YAMLPath: yamlPath, Line: line, Column: column})
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func mappingHasFalse(node *yaml.Node, key string) bool {
	value := mappingValue(node, key)
	return value != nil && value.Kind == yaml.ScalarNode && strings.EqualFold(strings.TrimSpace(value.Value), "false")
}

func scalarBool(node *yaml.Node) bool {
	return node != nil && node.Kind == yaml.ScalarNode && strings.EqualFold(strings.TrimSpace(node.Value), "true")
}

func scalarString(node *yaml.Node) string {
	if node == nil || node.Kind != yaml.ScalarNode {
		return ""
	}
	return strings.TrimSpace(node.Value)
}

func yamlPath(parts []string) string {
	var builder strings.Builder
	for _, part := range parts {
		if strings.HasPrefix(part, "[") {
			builder.WriteString(part)
			continue
		}
		if builder.Len() > 0 {
			builder.WriteByte('.')
		}
		builder.WriteString(part)
	}
	return builder.String()
}

func cloneManifest(manifest Manifest) (Manifest, error) {
	raw, err := json.Marshal(manifest)
	if err != nil {
		return Manifest{}, err
	}
	var clone Manifest
	if err := json.Unmarshal(raw, &clone); err != nil {
		return Manifest{}, err
	}
	return clone, nil
}
