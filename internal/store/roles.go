package store

// RoleLevel defines the four-level account hierarchy.
// LEVEL_2 has user privileges; administrative access starts at ADMIN.
// Unknown roles have no privileges.
func RoleLevel(role string) int {
	switch role {
	case "USER":
		return 1
	case "LEVEL_2":
		return 2
	case "ADMIN":
		return 3
	case "SUPER_ADMIN":
		return 4
	default:
		return 0
	}
}

func MeetsMinimumRole(role, minimum string) bool {
	if minimum == "" {
		minimum = "USER"
	}
	return RoleLevel(minimum) > 0 && RoleLevel(role) >= RoleLevel(minimum)
}
