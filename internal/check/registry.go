package check

// All returns every available check, in report order within each category.
func All() []Check {
	return []Check{
		Build(),
		GoVet(),
		Staticcheck(),
		ErrCheck(),
		IneffAssign(),
		GoFmt(),
		Complexity(),
		Misspell(),
		License(),
		TestPresence(),
		Coverage(),
		Govulncheck(),
		Gosec(),
	}
}

// Filter selects checks by name. When only is non-empty, just those checks
// run; checks in skip never run. Security checks are dropped when security is
// false.
func Filter(checks []Check, only, skip map[string]bool, security bool) []Check {
	var out []Check
	for _, c := range checks {
		if (len(only) > 0 && !only[c.Name()]) || skip[c.Name()] || (!security && c.Category() == Security) {
			continue
		}
		out = append(out, c)
	}
	return out
}
