package store

// RoleLevel defines the four-level account hierarchy. Level 2 is reserved and
// cannot be assigned until a role and its permissions are explicitly defined.
// Unknown roles have no privileges.
func RoleLevel(role string) int {
	switch role {
	case "USER":
		return 1
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
