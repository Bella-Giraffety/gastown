package cmd

import "strings"

func isCapacityNeutralTarget(target string) bool {
	target = strings.TrimSpace(target)
	if target == "" || target == "." {
		return false
	}
	if err := ValidateTarget(target); err != nil {
		return false
	}
	if _, isDog := IsDogTarget(target); isDog {
		return true
	}

	switch strings.ToLower(target) {
	case "mayor", "may", "deacon", "dea":
		return true
	}

	parts := strings.Split(target, "/")
	if len(parts) < 2 {
		return false
	}
	if _, isRig := IsRigName(parts[0]); !isRig {
		return false
	}
	role := strings.ToLower(parts[1])
	switch {
	case len(parts) == 2 && (role == "witness" || role == "refinery"):
		return true
	case len(parts) == 3 && role == "crew" && parts[2] != "":
		return true
	default:
		return false
	}
}
