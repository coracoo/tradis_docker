package templatecompiler

import "strings"

type interpolationRef struct {
	Name       string
	Raw        string
	Default    string
	HasDefault bool
	Required   bool
}

func scanInterpolations(value string) []interpolationRef {
	refs := make([]interpolationRef, 0)
	for i := 0; i < len(value); i++ {
		if value[i] != '$' || i+1 >= len(value) {
			continue
		}
		if value[i+1] == '$' {
			i++
			continue
		}
		if value[i+1] == '{' {
			end := strings.IndexByte(value[i+2:], '}')
			if end < 0 {
				continue
			}
			end += i + 2
			raw := value[i : end+1]
			if ref, ok := parseInterpolation(value[i+2:end], raw); ok {
				refs = append(refs, ref)
			}
			i = end
			continue
		}
		j := i + 1
		for j < len(value) && isEnvKeyByte(value[j], j == i+1) {
			j++
		}
		if j > i+1 {
			refs = append(refs, interpolationRef{Name: value[i+1 : j], Raw: value[i:j]})
			i = j - 1
		}
	}
	return refs
}

func parseInterpolation(inner, raw string) (interpolationRef, bool) {
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return interpolationRef{}, false
	}
	best := -1
	op := ""
	for _, candidate := range []string{":-", ":?", ":+", "-", "?", "+"} {
		if idx := strings.Index(inner, candidate); idx >= 0 && (best < 0 || idx < best || (idx == best && len(candidate) > len(op))) {
			best, op = idx, candidate
		}
	}
	name := inner
	arg := ""
	if best >= 0 {
		name = strings.TrimSpace(inner[:best])
		arg = inner[best+len(op):]
	}
	if !envKeyPattern.MatchString(name) {
		return interpolationRef{}, false
	}
	ref := interpolationRef{Name: name, Raw: raw}
	switch op {
	case "-", ":-":
		ref.HasDefault, ref.Default = true, arg
	case "?", ":?":
		ref.Required = true
	}
	return ref, true
}

func isEnvKeyByte(value byte, first bool) bool {
	if value == '_' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z' {
		return true
	}
	return !first && value >= '0' && value <= '9'
}
