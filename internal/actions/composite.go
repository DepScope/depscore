// internal/actions/composite.go
package actions

// ExtractCompositeActions returns ActionRefs from a composite action's steps.
// It walks the runs.steps of the given ActionYAML and collects every uses: entry.
// Returns nil if the action is not composite or if actionYAML is nil.
func ExtractCompositeActions(actionYAML *ActionYAML) []ActionRef {
	if actionYAML == nil || actionYAML.Runs.Using != "composite" {
		return nil
	}
	var refs []ActionRef
	for _, step := range actionYAML.Runs.Steps {
		if step.Uses != "" {
			refs = append(refs, ParseActionRef(step.Uses))
		}
	}
	return refs
}
