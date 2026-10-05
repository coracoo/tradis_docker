package templatecompiler

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

func Render(bundle SourceBundle, manifest Manifest, values map[string]string, overlays []MappingOverlay) (RenderedBundle, error) {
	if err := Verify(bundle, manifest); err != nil {
		return RenderedBundle{}, err
	}
	if !manifest.Complete {
		return RenderedBundle{}, fmt.Errorf("template manifest is incomplete")
	}
	known := make(map[string]Input, len(manifest.Inputs))
	effective := make(map[string]string, len(manifest.Inputs))
	for _, input := range manifest.Inputs {
		known[input.ID] = input
		value := input.DefaultValue
		if supplied, ok := values[input.ID]; ok {
			value = supplied
		}
		if input.Required && strings.TrimSpace(value) == "" {
			return RenderedBundle{}, fmt.Errorf("required input %s is empty", input.ID)
		}
		effective[input.ID] = value
	}
	for id := range values {
		if _, ok := known[id]; !ok {
			return RenderedBundle{}, fmt.Errorf("unknown input %s", id)
		}
	}

	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(bundle.Compose), &doc); err != nil {
		return RenderedBundle{}, err
	}
	if len(doc.Content) == 0 {
		return RenderedBundle{}, fmt.Errorf("Compose document is empty")
	}
	root := doc.Content[0]
	files := make(map[string]SourceFile, len(bundle.Files)+2)
	for _, file := range bundle.Files {
		if file.Mode == 0 {
			file.Mode = 0o644
		}
		files[file.Path] = file
	}
	dotenv := bundle.Dotenv

	for _, input := range manifest.Inputs {
		value := effective[input.ID]
		switch input.Scope {
		case "project":
			dotenv = upsertDotenv(dotenv, input.Key, value)
		case "env_file":
			file := files[input.File]
			file.Path = input.File
			file.Mode = 0o644
			file.Content = upsertDotenv(file.Content, input.Key, value)
			files[input.File] = file
		case "secret":
			files[input.File] = SourceFile{Path: input.File, Content: value, Mode: 0o600}
		case "service":
			if input.Kind == "env" {
				if err := patchServiceEnvironment(root, manifest.Bindings, input.ID, value); err != nil {
					return RenderedBundle{}, err
				}
			}
		}
	}
	if dotenv != "" || hasProjectInputs(manifest.Inputs) {
		files[".env"] = SourceFile{Path: ".env", Content: ensureTrailingNewline(dotenv), Mode: 0o644}
	}

	overlaysByID := make(map[string]MappingOverlay, len(overlays))
	for _, overlay := range overlays {
		if overlay.ID == "" {
			return RenderedBundle{}, fmt.Errorf("mapping overlay id is required")
		}
		if _, exists := overlaysByID[overlay.ID]; exists {
			return RenderedBundle{}, fmt.Errorf("duplicate mapping overlay %s", overlay.ID)
		}
		overlaysByID[overlay.ID] = overlay
	}
	for _, mapping := range manifest.Mappings {
		value, supplied := values[mapping.ID]
		overlay, hasOverlay := overlaysByID[mapping.ID]
		if !supplied {
			value = mapping.Source
		}
		if !hasOverlay && (!supplied || value == mapping.Source) {
			continue
		}
		target := ""
		if hasOverlay {
			if overlay.Kind != mapping.Kind || overlay.Service != mapping.Service {
				return RenderedBundle{}, fmt.Errorf("mapping overlay %s does not match manifest", overlay.ID)
			}
			if overlay.Source != "" {
				value = overlay.Source
			}
			target = overlay.Target
			if overlay.Protocol != "" {
				mapping.Protocol = overlay.Protocol
			}
			if overlay.Mode != "" {
				mapping.Mode = overlay.Mode
			}
			delete(overlaysByID, mapping.ID)
		}
		if err := patchMapping(root, mapping, value, target); err != nil {
			return RenderedBundle{}, err
		}
	}
	for _, overlay := range overlaysByID {
		if err := appendMappingOverlay(root, overlay); err != nil {
			return RenderedBundle{}, err
		}
	}

	compose, err := yaml.Marshal(&doc)
	if err != nil {
		return RenderedBundle{}, err
	}
	renderedFiles := make([]SourceFile, 0, len(files))
	for _, file := range files {
		if file.Mode == 0 {
			file.Mode = 0o644
		}
		renderedFiles = append(renderedFiles, file)
	}
	sort.Slice(renderedFiles, func(i, j int) bool { return renderedFiles[i].Path < renderedFiles[j].Path })
	return RenderedBundle{ComposePath: bundle.ComposePath, Compose: string(compose), Files: renderedFiles}, nil
}

func patchServiceEnvironment(root *yaml.Node, bindings []Binding, inputID, value string) error {
	for _, binding := range bindings {
		if binding.InputID != inputID || binding.TargetKind != "environment" {
			continue
		}
		service := serviceNode(root, binding.Service)
		if service == nil {
			return fmt.Errorf("service %s no longer exists", binding.Service)
		}
		environment := mappingValue(service, "environment")
		if environment == nil {
			return fmt.Errorf("service %s environment no longer exists", binding.Service)
		}
		switch environment.Kind {
		case yaml.MappingNode:
			entry := mappingValue(environment, binding.TargetKey)
			if entry == nil {
				return fmt.Errorf("environment target %s no longer exists", binding.TargetKey)
			}
			entry.Value = value
			entry.Tag = "!!str"
		case yaml.SequenceNode:
			found := false
			for _, entry := range environment.Content {
				parts := strings.SplitN(entry.Value, "=", 2)
				if strings.TrimSpace(parts[0]) == binding.TargetKey {
					entry.Value = binding.TargetKey + "=" + value
					entry.Tag = "!!str"
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("environment target %s no longer exists", binding.TargetKey)
			}
		}
	}
	return nil
}

func patchMapping(root *yaml.Node, mapping Mapping, source, targetOverride string) error {
	service := serviceNode(root, mapping.Service)
	if service == nil {
		return fmt.Errorf("service %s no longer exists", mapping.Service)
	}
	field := mapping.Kind + "s"
	if mapping.Kind == "bind" || mapping.Kind == "volume" {
		field = "volumes"
	}
	sequence := mappingValue(service, field)
	if sequence == nil || sequence.Kind != yaml.SequenceNode || mapping.SequenceIndex < 0 || mapping.SequenceIndex >= len(sequence.Content) {
		return fmt.Errorf("mapping %s target no longer exists", mapping.ID)
	}
	entry := sequence.Content[mapping.SequenceIndex]
	target := mapping.Target
	if targetOverride != "" {
		target = targetOverride
	}
	if entry.Kind == yaml.ScalarNode {
		switch mapping.Kind {
		case "port":
			entry.Value = rewritePortScalar(entry.Value, source, target, mapping.Protocol)
		case "bind", "volume":
			entry.Value = source + ":" + target
			if mapping.Mode != "" {
				entry.Value += ":" + mapping.Mode
			}
		case "device":
			entry.Value = rewriteDeviceScalar(entry.Value, source, target)
		}
		entry.Tag = "!!str"
		return nil
	}
	if entry.Kind != yaml.MappingNode {
		return fmt.Errorf("mapping %s has unsupported YAML shape", mapping.ID)
	}
	switch mapping.Kind {
	case "port":
		if source == "" {
			removeMappingKey(entry, "published")
		} else {
			setMappingScalar(entry, "published", source)
		}
		setMappingScalar(entry, "target", target)
	case "bind", "volume":
		setMappingScalar(entry, "source", source)
		setMappingScalar(entry, "target", target)
	case "device":
		setMappingScalar(entry, "source", source)
		setMappingScalar(entry, "target", target)
	}
	return nil
}

func rewritePortScalar(original, source, target, protocol string) string {
	base := strings.TrimSpace(original)
	originalProtocol := ""
	if slash := strings.LastIndex(base, "/"); slash >= 0 {
		originalProtocol = strings.TrimSpace(base[slash+1:])
		base = base[:slash]
	}
	hostPrefix := ""
	if last := strings.LastIndex(base, ":"); last >= 0 {
		beforeTarget := base[:last]
		if previous := strings.LastIndex(beforeTarget, ":"); previous >= 0 {
			hostPrefix = beforeTarget[:previous+1]
		}
	}
	result := target
	if source != "" {
		result = hostPrefix + source + ":" + target
	}
	if protocol == "" {
		protocol = originalProtocol
	}
	if originalProtocol != "" || (protocol != "" && protocol != "tcp") {
		result += "/" + protocol
	}
	return result
}

func rewriteDeviceScalar(original, source, target string) string {
	parts := strings.Split(original, ":")
	result := source + ":" + target
	if len(parts) > 2 {
		result += ":" + strings.Join(parts[2:], ":")
	}
	return result
}

func removeMappingKey(mapping *yaml.Node, key string) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value != key {
			continue
		}
		mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
		return
	}
}

func appendMappingOverlay(root *yaml.Node, overlay MappingOverlay) error {
	if overlay.ID == "" || overlay.Service == "" || overlay.Source == "" || (overlay.Kind != "environment" && overlay.Target == "") {
		return fmt.Errorf("invalid mapping overlay")
	}
	service := serviceNode(root, overlay.Service)
	if service == nil {
		return fmt.Errorf("overlay service %s does not exist", overlay.Service)
	}
	if overlay.Kind == "environment" {
		return appendEnvironmentOverlay(service, overlay)
	}
	field := overlay.Kind + "s"
	if overlay.Kind == "bind" || overlay.Kind == "volume" {
		field = "volumes"
	}
	if field != "ports" && field != "volumes" && field != "devices" {
		return fmt.Errorf("unsupported overlay kind %s", overlay.Kind)
	}
	sequence := mappingValue(service, field)
	if sequence == nil {
		sequence = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		service.Content = append(service.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: field}, sequence)
	}
	if sequence.Kind != yaml.SequenceNode {
		return fmt.Errorf("service %s %s is not a sequence", overlay.Service, field)
	}
	value := overlay.Source + ":" + overlay.Target
	if overlay.Kind == "port" && overlay.Protocol != "" && overlay.Protocol != "tcp" {
		value += "/" + overlay.Protocol
	}
	if (overlay.Kind == "bind" || overlay.Kind == "volume") && overlay.Mode != "" {
		value += ":" + overlay.Mode
	}
	sequence.Content = append(sequence.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
	return nil
}

func appendEnvironmentOverlay(service *yaml.Node, overlay MappingOverlay) error {
	key := strings.TrimSpace(overlay.Source)
	if key == "" || strings.Contains(key, "=") {
		return fmt.Errorf("invalid environment key")
	}
	environment := mappingValue(service, "environment")
	if environment == nil {
		environment = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		service.Content = append(service.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "environment"},
			environment,
		)
	}
	switch environment.Kind {
	case yaml.MappingNode:
		if mappingValue(environment, key) != nil {
			return fmt.Errorf("environment key %s already exists", key)
		}
		environment.Content = append(environment.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: overlay.Target},
		)
	case yaml.SequenceNode:
		for _, entry := range environment.Content {
			parts := strings.SplitN(entry.Value, "=", 2)
			if strings.TrimSpace(parts[0]) == key {
				return fmt.Errorf("environment key %s already exists", key)
			}
		}
		environment.Content = append(environment.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key + "=" + overlay.Target},
		)
	default:
		return fmt.Errorf("service environment has unsupported YAML shape")
	}
	return nil
}

func serviceNode(root *yaml.Node, name string) *yaml.Node {
	services := mappingValue(root, "services")
	return mappingValue(services, name)
}

func setMappingScalar(mapping *yaml.Node, key, value string) {
	if existing := mappingValue(mapping, key); existing != nil {
		existing.Kind = yaml.ScalarNode
		existing.Tag = "!!str"
		existing.Value = value
		return
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
}

func upsertDotenv(content, key, value string) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	found := false
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		parts := strings.SplitN(line, "=", 2)
		if strings.TrimSpace(parts[0]) == key {
			lines[i] = key + "=" + quoteDotenv(value)
			found = true
		}
	}
	if !found {
		if len(lines) == 1 && lines[0] == "" {
			lines = lines[:0]
		}
		lines = append(lines, key+"="+quoteDotenv(value))
	}
	return ensureTrailingNewline(strings.Join(lines, "\n"))
}

func quoteDotenv(value string) string {
	if !strings.ContainsAny(value, " \t#\"'\r\n") {
		return value
	}
	return `"` + strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), `"`, `\"`) + `"`
}

func ensureTrailingNewline(content string) string {
	return strings.TrimRight(content, "\n") + "\n"
}

func hasProjectInputs(inputs []Input) bool {
	for _, input := range inputs {
		if input.Scope == "project" {
			return true
		}
	}
	return false
}
