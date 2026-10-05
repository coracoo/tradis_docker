package deployment

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/docker/go-connections/nat"
)

var composeProjectNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

type ComposeLocalizationOverrides struct {
	ProjectName         string
	AutoAllocatePorts   *bool
	PortAvailable       func(port int, protocol string) bool
	InterpolationValues map[string]string
}

type LocalizedCompose struct {
	SourceYAML  string       `json:"sourceYaml"`
	RuntimeYAML string       `json:"runtimeYaml"`
	Changes     []PlanChange `json:"changes"`
}

func LocalizeCompose(
	sourceYAML string,
	profile NASProfile,
	overrides ComposeLocalizationOverrides,
) (string, []PlanChange, error) {
	profile = normalizeNASProfile(profile)
	if err := validateNASProfile(profile); err != nil {
		return "", nil, localizationInputError("profile_invalid", err, nil)
	}
	projectName := strings.TrimSpace(overrides.ProjectName)
	if !composeProjectNamePattern.MatchString(projectName) {
		return "", nil, localizationInputError("project_name_invalid", errors.New("invalid Compose project name"), map[string]any{
			"project": projectName,
		})
	}
	projectDir := filepath.Clean(filepath.Join(profile.ProjectRoot, projectName))
	if !pathAllowedByRoots(projectDir, []string{profile.ProjectRoot}) {
		return "", nil, localizationInputError("project_path_invalid", errors.New("project path escapes project root"), map[string]any{
			"project": projectName,
		})
	}

	document, err := parseComposeDocument(sourceYAML)
	if err != nil {
		return "", nil, localizationInputError("compose_invalid", err, nil)
	}
	autoAllocate := profile.PortRange.AutoAllocate
	if overrides.AutoAllocatePorts != nil {
		autoAllocate = *overrides.AutoAllocatePorts
	}
	allocator := newComposePortAllocator(profile.PortRange, overrides.PortAvailable)
	changes := make([]PlanChange, 0)
	for _, serviceName := range sortedMapKeys(document.services) {
		service, ok := document.services[serviceName].(map[string]any)
		if !ok {
			return "", nil, localizationInputError("compose_service_invalid", errors.New("Compose service must be a mapping"), map[string]any{
				"service": serviceName,
			})
		}
		volumeChanges, err := localizeComposeVolumes(serviceName, service, projectDir, profile.AllowedBindRoots)
		if err != nil {
			return "", nil, err
		}
		changes = append(changes, volumeChanges...)
		if autoAllocate && !strings.EqualFold(strings.TrimSpace(fmt.Sprint(service["network_mode"])), "host") {
			portChanges, err := localizeComposePorts(serviceName, service, allocator, overrides.InterpolationValues)
			if err != nil {
				return "", nil, err
			}
			changes = append(changes, portChanges...)
		}
	}

	runtimeYAML, err := marshalComposeDocument(document)
	if err != nil {
		return "", nil, localizationInputError("compose_marshal_failed", err, nil)
	}
	return runtimeYAML, changes, nil
}

func localizationInputError(reason string, cause error, details map[string]any) error {
	return localizationStructuredError(ErrorCodeInvalidInput, reason, cause, false, []string{"fix_compose"}, details)
}

func localizationBlockedError(reason string, cause error, actions []string, details map[string]any) error {
	return localizationStructuredError(ErrorCodePreflightBlocked, reason, cause, false, actions, details)
}

func localizationStructuredError(
	code ErrorCode,
	reason string,
	cause error,
	retryable bool,
	actions []string,
	details map[string]any,
) error {
	copyDetails := make(map[string]any, len(details)+1)
	for key, value := range details {
		copyDetails[key] = value
	}
	copyDetails["reasonCode"] = reason
	return NewStructuredError(code, cause, retryable, actions, copyDetails)
}

func localizeComposeVolumes(
	serviceName string,
	service map[string]any,
	projectDir string,
	allowedRoots []string,
) ([]PlanChange, error) {
	rawVolumes, exists := service["volumes"]
	if !exists || rawVolumes == nil {
		return nil, nil
	}
	volumes, ok := rawVolumes.([]any)
	if !ok {
		return nil, localizationInputError("compose_volumes_invalid", errors.New("Compose volumes must be a list"), map[string]any{
			"service": serviceName,
		})
	}
	changes := make([]PlanChange, 0)
	for index, rawVolume := range volumes {
		target := fmt.Sprintf("services.%s.volumes[%d].source", serviceName, index)
		switch volume := rawVolume.(type) {
		case string:
			rewritten, before, after, changed, err := localizeShortComposeVolume(volume, projectDir, allowedRoots)
			if err != nil {
				return nil, withLocalizationService(err, serviceName, target)
			}
			if changed {
				volumes[index] = rewritten
				changes = append(changes, PlanChange{Type: "bind", Target: target, Before: before, After: after, ReasonCode: "relative_bind_to_project"})
			}
		case map[string]any:
			changed, before, after, err := localizeLongComposeVolume(volume, projectDir, allowedRoots)
			if err != nil {
				return nil, withLocalizationService(err, serviceName, target)
			}
			if changed {
				changes = append(changes, PlanChange{Type: "bind", Target: target, Before: before, After: after, ReasonCode: "relative_bind_to_project"})
			}
		default:
			return nil, localizationInputError("compose_volume_invalid", errors.New("Compose volume entry is invalid"), map[string]any{
				"service": serviceName,
				"target":  target,
			})
		}
	}
	return changes, nil
}

func localizeShortComposeVolume(
	raw string,
	projectDir string,
	allowedRoots []string,
) (rewritten string, before string, after string, changed bool, err error) {
	parts := strings.SplitN(strings.TrimSpace(raw), ":", 3)
	if len(parts) < 2 {
		return raw, "", "", false, nil
	}
	source := strings.TrimSpace(parts[0])
	if !looksLikeComposeBindSource(source, false) {
		return raw, "", "", false, nil
	}
	localized, changed, err := localizeComposeBindSource(source, projectDir, allowedRoots)
	if err != nil || !changed {
		return raw, source, localized, false, err
	}
	parts[0] = localized
	return strings.Join(parts, ":"), source, localized, true, nil
}

func localizeLongComposeVolume(
	volume map[string]any,
	projectDir string,
	allowedRoots []string,
) (changed bool, before string, after string, err error) {
	volumeType := strings.ToLower(strings.TrimSpace(fmt.Sprint(volume["type"])))
	if volumeType == "volume" || volumeType == "tmpfs" || volumeType == "image" || volumeType == "npipe" {
		return false, "", "", nil
	}
	source := strings.TrimSpace(fmt.Sprint(volume["source"]))
	if source == "" || source == "<nil>" {
		return false, "", "", nil
	}
	if volumeType != "bind" && !looksLikeComposeBindSource(source, false) {
		return false, "", "", nil
	}
	localized, changed, err := localizeComposeBindSource(source, projectDir, allowedRoots)
	if err != nil || !changed {
		return false, source, localized, err
	}
	volume["source"] = localized
	return true, source, localized, nil
}

func looksLikeComposeBindSource(source string, longBind bool) bool {
	source = strings.TrimSpace(source)
	return longBind || filepath.IsAbs(source) || strings.HasPrefix(source, ".") || strings.HasPrefix(source, "~") ||
		strings.HasPrefix(source, "$PWD") || strings.HasPrefix(source, "${PWD}")
}

func localizeComposeBindSource(source string, projectDir string, allowedRoots []string) (string, bool, error) {
	source = strings.TrimSpace(source)
	for _, prefix := range []string{"${PWD}", "$PWD"} {
		if source == prefix {
			source = "."
			break
		}
		if strings.HasPrefix(source, prefix+"/") {
			source = "." + strings.TrimPrefix(source, prefix)
			break
		}
	}
	if strings.HasPrefix(source, "~") {
		return "", false, localizationBlockedError("bind_home_not_resolved", errors.New("home-relative bind requires confirmation"), []string{"confirm_bind_path", "map_to_project_directory"}, map[string]any{
			"path": source,
		})
	}
	if filepath.IsAbs(source) {
		clean := filepath.Clean(source)
		if !pathAllowedByRoots(clean, allowedRoots) {
			return "", false, localizationBlockedError("bind_root_not_allowed", errors.New("absolute bind is outside allowed roots"), []string{"confirm_bind_path", "update_nas_profile"}, map[string]any{
				"path": clean,
			})
		}
		return clean, false, nil
	}
	localized := filepath.Clean(filepath.Join(projectDir, source))
	if !pathAllowedByRoots(localized, []string{projectDir}) {
		return "", false, localizationBlockedError("bind_path_escapes_project", errors.New("relative bind escapes project directory"), []string{"fix_bind_path", "confirm_bind_path"}, map[string]any{
			"path": source,
		})
	}
	return localized, localized != source, nil
}

func withLocalizationService(err error, service string, target string) error {
	var structured *StructuredError
	if !errors.As(err, &structured) {
		return err
	}
	details := make(map[string]any, len(structured.Details)+2)
	for key, value := range structured.Details {
		details[key] = value
	}
	details["service"] = service
	details["target"] = target
	return NewStructuredError(structured.Code, errors.Unwrap(structured), structured.Retryable, structured.NextActions, details)
}

type composePortAllocator struct {
	rangeConfig PortRange
	available   func(int, string) bool
	next        map[string]int
	allocated   map[string]struct{}
}

func newComposePortAllocator(config PortRange, available func(int, string) bool) *composePortAllocator {
	if available == nil {
		available = func(int, string) bool { return true }
	}
	return &composePortAllocator{
		rangeConfig: config,
		available:   available,
		next:        make(map[string]int),
		allocated:   make(map[string]struct{}),
	}
}

func (allocator *composePortAllocator) allocate(protocol string) (int, error) {
	protocol = normalizeComposeProtocol(protocol)
	start := allocator.rangeConfig.Start
	end := allocator.rangeConfig.End
	if start < 1 || end < start || end > 65535 {
		return 0, localizationInputError("port_range_invalid", errors.New("invalid automatic port range"), map[string]any{
			"start": start,
			"end":   end,
		})
	}
	next := allocator.next[protocol]
	if next < start || next > end {
		next = start
	}
	for offset := 0; offset <= end-start; offset++ {
		port := start + ((next - start + offset) % (end - start + 1))
		key := fmt.Sprintf("%d/%s", port, protocol)
		if _, exists := allocator.allocated[key]; exists || !allocator.available(port, protocol) {
			continue
		}
		allocator.allocated[key] = struct{}{}
		allocator.next[protocol] = port + 1
		return port, nil
	}
	return 0, localizationBlockedError("port_pool_exhausted", errors.New("automatic port pool is exhausted"), []string{"expand_port_range", "free_port", "disable_auto_port"}, map[string]any{
		"start":    start,
		"end":      end,
		"protocol": protocol,
	})
}

func localizeComposePorts(
	serviceName string,
	service map[string]any,
	allocator *composePortAllocator,
	interpolationValues map[string]string,
) ([]PlanChange, error) {
	rawPorts, exists := service["ports"]
	if !exists || rawPorts == nil {
		return nil, nil
	}
	ports, ok := rawPorts.([]any)
	if !ok {
		return nil, localizationInputError("compose_ports_invalid", errors.New("Compose ports must be a list"), map[string]any{"service": serviceName})
	}
	localized := make([]any, 0, len(ports))
	changes := make([]PlanChange, 0)
	for index, rawPort := range ports {
		target := fmt.Sprintf("services.%s.ports[%d].published", serviceName, index)
		switch port := rawPort.(type) {
		case string:
			expanded, err := expandComposePortValue(port, interpolationValues)
			if err != nil {
				return nil, localizationInputError("compose_port_interpolation_invalid", err, map[string]any{
					"service": serviceName,
					"target":  target,
				})
			}
			items, itemChanges, err := localizeShortComposePorts(serviceName, target, expanded, allocator)
			if err != nil {
				return nil, err
			}
			localized = append(localized, items...)
			changes = append(changes, itemChanges...)
		case int:
			items, itemChanges, err := localizeShortComposePorts(serviceName, target, strconv.Itoa(port), allocator)
			if err != nil {
				return nil, err
			}
			localized = append(localized, items...)
			changes = append(changes, itemChanges...)
		case map[string]any:
			protocol := normalizeComposeProtocol(fmt.Sprint(port["protocol"]))
			allocated, err := allocator.allocate(protocol)
			if err != nil {
				return nil, withLocalizationService(err, serviceName, target)
			}
			before := port["published"]
			port["published"] = allocated
			localized = append(localized, port)
			changes = append(changes, PlanChange{Type: "port", Target: target, Before: before, After: allocated, ReasonCode: "auto_allocate_port"})
		default:
			return nil, localizationInputError("compose_port_invalid", errors.New("invalid Compose port mapping"), map[string]any{"service": serviceName, "target": target})
		}
	}
	service["ports"] = localized
	return changes, nil
}

var composePortVariablePattern = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

func expandComposePortValue(raw string, values map[string]string) (string, error) {
	return expandComposePortValueDepth(raw, values, 0)
}

func expandComposePortValueDepth(raw string, values map[string]string, depth int) (string, error) {
	if depth > 16 {
		return "", fmt.Errorf("Compose port variable expansion is too deeply nested")
	}
	var out strings.Builder
	for index := 0; index < len(raw); {
		if raw[index] != '$' {
			out.WriteByte(raw[index])
			index++
			continue
		}
		if index+1 < len(raw) && raw[index+1] == '$' {
			out.WriteByte('$')
			index += 2
			continue
		}
		if index+1 < len(raw) && raw[index+1] == '{' {
			end, ok := composePortVariableEnd(raw, index)
			if !ok {
				return "", fmt.Errorf("unclosed Compose variable in port mapping")
			}
			value, err := resolveComposePortVariable(raw[index+2:end], values)
			if err != nil {
				return "", err
			}
			if strings.Contains(value, "$") {
				value, err = expandComposePortValueDepth(value, values, depth+1)
				if err != nil {
					return "", err
				}
			}
			out.WriteString(value)
			index = end + 1
			continue
		}

		match := composePortVariablePattern.FindStringIndex(raw[index+1:])
		if match == nil || match[0] != 0 {
			out.WriteByte(raw[index])
			index++
			continue
		}
		name := raw[index+1 : index+1+match[1]]
		value, exists := values[name]
		if !exists {
			return "", fmt.Errorf("Compose port variable %s is not configured", name)
		}
		out.WriteString(value)
		index += 1 + match[1]
	}
	return out.String(), nil
}

func composePortVariableEnd(raw string, start int) (int, bool) {
	depth := 1
	for index := start + 2; index < len(raw); index++ {
		if raw[index] == '$' && index+1 < len(raw) && raw[index+1] == '{' {
			depth++
			index++
			continue
		}
		if raw[index] != '}' {
			continue
		}
		depth--
		if depth == 0 {
			return index, true
		}
	}
	return 0, false
}

func resolveComposePortVariable(expression string, values map[string]string) (string, error) {
	name := expression
	operator := ""
	operand := ""
	for _, candidate := range []string{":-", ":?", ":+", "-", "?", "+"} {
		if offset := strings.Index(expression, candidate); offset >= 0 {
			name = expression[:offset]
			operator = candidate
			operand = expression[offset+len(candidate):]
			break
		}
	}
	name = strings.TrimSpace(name)
	if !composePortVariablePattern.MatchString(name) || composePortVariablePattern.FindString(name) != name {
		return "", fmt.Errorf("invalid Compose port variable expression ${%s}", expression)
	}
	value, exists := values[name]
	nonEmpty := exists && value != ""
	switch operator {
	case ":-":
		if !nonEmpty {
			return operand, nil
		}
	case "-":
		if !exists {
			return operand, nil
		}
	case ":?":
		if !nonEmpty {
			return "", fmt.Errorf("Compose port variable %s is required: %s", name, strings.TrimSpace(operand))
		}
	case "?":
		if !exists {
			return "", fmt.Errorf("Compose port variable %s is required: %s", name, strings.TrimSpace(operand))
		}
	case ":+":
		if nonEmpty {
			return operand, nil
		}
		return "", nil
	case "+":
		if exists {
			return operand, nil
		}
		return "", nil
	default:
		if !exists {
			return "", fmt.Errorf("Compose port variable %s is not configured", name)
		}
	}
	return value, nil
}

func localizeShortComposePorts(
	serviceName string,
	target string,
	raw string,
	allocator *composePortAllocator,
) ([]any, []PlanChange, error) {
	mappings, err := nat.ParsePortSpec(strings.TrimSpace(raw))
	if err != nil || len(mappings) == 0 {
		return nil, nil, localizationInputError("compose_port_invalid", errors.New("invalid Compose port mapping"), map[string]any{"service": serviceName, "target": target})
	}
	localized := make([]any, 0, len(mappings))
	changes := make([]PlanChange, 0, len(mappings))
	for mappingIndex, mapping := range mappings {
		protocol := normalizeComposeProtocol(mapping.Port.Proto())
		allocated, err := allocator.allocate(protocol)
		if err != nil {
			return nil, nil, withLocalizationService(err, serviceName, target)
		}
		localized = append(localized, formatLocalizedShortPort(raw, mapping, allocated))
		changeTarget := target
		if len(mappings) > 1 {
			changeTarget = fmt.Sprintf("%s[%d]", target, mappingIndex)
		}
		changes = append(changes, PlanChange{
			Type:       "port",
			Target:     changeTarget,
			Before:     parsePublishedPort(mapping.Binding.HostPort),
			After:      allocated,
			ReasonCode: "auto_allocate_port",
		})
	}
	return localized, changes, nil
}

func normalizeComposeProtocol(protocol string) string {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if protocol == "" || protocol == "<nil>" {
		return "tcp"
	}
	return protocol
}

func parsePublishedPort(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if port, err := strconv.Atoi(value); err == nil {
		return port
	}
	return value
}

func formatLocalizedShortPort(original string, mapping nat.PortMapping, published int) string {
	host := strings.TrimSpace(mapping.Binding.HostIP)
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	parts := make([]string, 0, 3)
	if host != "" {
		parts = append(parts, host)
	}
	parts = append(parts, strconv.Itoa(published), mapping.Port.Port())
	out := strings.Join(parts, ":")
	protocol := normalizeComposeProtocol(mapping.Port.Proto())
	if protocol != "tcp" || strings.Contains(original, "/") {
		out += "/" + protocol
	}
	return out
}
