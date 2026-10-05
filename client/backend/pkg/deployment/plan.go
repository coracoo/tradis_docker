package deployment

func BuildDeploymentPlan(intent Intent, localized LocalizedCompose, preflight PreflightResult) Plan {
	sourceYAML := localized.SourceYAML
	if sourceYAML == "" {
		sourceYAML = intent.ComposeYAML
	}
	runtimeYAML := localized.RuntimeYAML
	if runtimeYAML == "" {
		runtimeYAML = sourceYAML
	}

	planIntent := cloneDeploymentIntent(intent)
	planIntent.ComposeYAML = sourceYAML
	plan := Plan{
		Intent:         planIntent,
		RuntimeCompose: runtimeYAML,
		Changes:        clonePlanChanges(localized.Changes),
		Preflight:      clonePreflightResult(preflight),
	}
	plan.RequiresConfirmation = len(plan.Preflight.IssuesBySeverity(PreflightWarning)) > 0
	return plan
}

func cloneDeploymentIntent(intent Intent) Intent {
	cloned := intent
	cloned.Source.Metadata = cloneStringAnyMap(intent.Source.Metadata)
	cloned.Parameters = cloneStringAnyMap(intent.Parameters)
	cloned.Options = cloneStringAnyMap(intent.Options)
	return cloned
}

func clonePlanChanges(changes []PlanChange) []PlanChange {
	if changes == nil {
		return nil
	}
	out := make([]PlanChange, len(changes))
	for index, change := range changes {
		out[index] = change
		out[index].Before = cloneDeploymentValue(change.Before)
		out[index].After = cloneDeploymentValue(change.After)
	}
	return out
}

func clonePreflightResult(result PreflightResult) PreflightResult {
	if result.Issues == nil {
		return result
	}
	out := PreflightResult{Issues: make([]PreflightIssue, len(result.Issues))}
	for index, issue := range result.Issues {
		out.Issues[index] = issue
		out.Issues[index].NextActions = append([]string(nil), issue.NextActions...)
		out.Issues[index].Details = cloneStringAnyMap(issue.Details)
	}
	return out
}

func cloneStringAnyMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = cloneDeploymentValue(value)
	}
	return out
}

func cloneDeploymentValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneStringAnyMap(typed)
	case []any:
		out := make([]any, len(typed))
		for index, item := range typed {
			out[index] = cloneDeploymentValue(item)
		}
		return out
	case []string:
		return append([]string(nil), typed...)
	default:
		return value
	}
}
