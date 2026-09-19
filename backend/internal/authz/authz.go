// Package authz holds the role model and the account-management policy
// (spec DEC-014, DEC-022, DEC-056 and RN-022). It has no dependencies so both
// the HTTP layer and future commands can share the same rules.
package authz

const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleReader = "reader"
)

// Action is an operation on another account.
type Action string

const (
	ActionPromote Action = "promote" // reader -> admin
	ActionDemote  Action = "demote"  // admin -> reader
	ActionBlock   Action = "block"
	ActionRemove  Action = "remove"
	ActionReset   Action = "reset" // approve a password reset
)

// IsStaff reports whether the role may run catalog administration
// (ingestion, editing, removal). Owner has every admin capability.
func IsStaff(role string) bool {
	return role == RoleOwner || role == RoleAdmin
}

// IsAssignableRole reports whether a role can be assigned to an account.
// Owner is excluded: it only changes hands through an explicit transfer.
func IsAssignableRole(role string) bool {
	return role == RoleAdmin || role == RoleReader
}

// CanManageAccount decides whether actor may perform action on an account
// whose current role is target. Only the owner touches admins; nobody touches
// the owner; admins manage readers; readers manage nobody. Unknown roles and
// actions are denied.
func CanManageAccount(actor, target string, action Action) bool {
	switch action {
	case ActionPromote, ActionDemote, ActionBlock, ActionRemove, ActionReset:
	default:
		return false
	}

	if target == RoleOwner {
		return false
	}

	switch actor {
	case RoleOwner:
		return target == RoleAdmin || target == RoleReader
	case RoleAdmin:
		// Admins may block or remove readers, but never change roles and
		// never act on other admins (closes "invite as reader, then promote").
		return target == RoleReader && (action == ActionBlock || action == ActionRemove || action == ActionReset)
	default:
		return false
	}
}

// CanInvite decides whether actor may issue (or revoke) an invitation that
// creates an account with the given role. Admins invite readers; only the owner
// invites admins (DEC-055, DEC-056); nobody invites an owner.
func CanInvite(actor, role string) bool {
	switch role {
	case RoleReader:
		return IsStaff(actor)
	case RoleAdmin:
		return actor == RoleOwner
	default:
		return false
	}
}
