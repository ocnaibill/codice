package authz

import "testing"

func TestCanManageAccount(t *testing.T) {
	cases := []struct {
		name   string
		actor  string
		target string
		action Action
		want   bool
	}{
		// Owner manages readers and admins, never another owner.
		{"owner promotes reader", RoleOwner, RoleReader, ActionPromote, true},
		{"owner demotes admin", RoleOwner, RoleAdmin, ActionDemote, true},
		{"owner blocks admin", RoleOwner, RoleAdmin, ActionBlock, true},
		{"owner removes admin", RoleOwner, RoleAdmin, ActionRemove, true},
		{"owner blocks reader", RoleOwner, RoleReader, ActionBlock, true},
		{"owner cannot be demoted by owner", RoleOwner, RoleOwner, ActionDemote, false},
		{"owner cannot be removed by owner", RoleOwner, RoleOwner, ActionRemove, false},

		// RN-022 / DEC-056: admin never touches another admin or the owner.
		{"admin cannot promote reader to admin", RoleAdmin, RoleReader, ActionPromote, false},
		{"admin cannot demote admin", RoleAdmin, RoleAdmin, ActionDemote, false},
		{"admin cannot block admin", RoleAdmin, RoleAdmin, ActionBlock, false},
		{"admin cannot remove admin", RoleAdmin, RoleAdmin, ActionRemove, false},
		{"admin cannot block owner", RoleAdmin, RoleOwner, ActionBlock, false},
		{"admin cannot remove owner", RoleAdmin, RoleOwner, ActionRemove, false},
		{"admin cannot demote owner", RoleAdmin, RoleOwner, ActionDemote, false},
		{"admin cannot demote self", RoleAdmin, RoleAdmin, ActionDemote, false},
		{"admin may block reader", RoleAdmin, RoleReader, ActionBlock, true},
		{"admin may remove reader", RoleAdmin, RoleReader, ActionRemove, true},

		// Readers manage nobody.
		{"reader cannot block reader", RoleReader, RoleReader, ActionBlock, false},
		{"reader cannot promote", RoleReader, RoleReader, ActionPromote, false},
		{"reader cannot touch admin", RoleReader, RoleAdmin, ActionRemove, false},

		// Unknown roles and actions fail closed.
		{"unknown actor", "root", RoleReader, ActionBlock, false},
		{"empty actor", "", RoleReader, ActionBlock, false},
		{"unknown target", RoleOwner, "root", ActionBlock, false},
		{"unknown action", RoleOwner, RoleReader, Action("nuke"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CanManageAccount(c.actor, c.target, c.action); got != c.want {
				t.Errorf("CanManageAccount(%q, %q, %q) = %v, want %v", c.actor, c.target, c.action, got, c.want)
			}
		})
	}
}

func TestIsStaff(t *testing.T) {
	for role, want := range map[string]bool{
		RoleOwner: true, RoleAdmin: true, RoleReader: false, "": false, "root": false, "ADMIN": false,
	} {
		if got := IsStaff(role); got != want {
			t.Errorf("IsStaff(%q) = %v, want %v", role, got, want)
		}
	}
}

func TestValidAssignableRole(t *testing.T) {
	// Owner is only reachable through an explicit transfer, never by assignment.
	for role, want := range map[string]bool{
		RoleAdmin: true, RoleReader: true, RoleOwner: false, "": false, "root": false,
	} {
		if got := IsAssignableRole(role); got != want {
			t.Errorf("IsAssignableRole(%q) = %v, want %v", role, got, want)
		}
	}
}
