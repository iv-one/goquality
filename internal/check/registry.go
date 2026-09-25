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

// Filter returns the checks whose names are not in skip. Security checks are
// dropped when security is false.
func Filter(checks []Check, skip map[string]bool, security bool) []Check {
	var out []Check
	for _, c := range checks {
		if skip[c.Name()] || (!security && c.Category() == Security) {
			continue
		}
		out = append(out, c)
	}
	return out
}
