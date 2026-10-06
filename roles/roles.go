package roles

import (
	"github.com/kncept/guacamole/roles/complex"
	"github.com/kncept/guacamole/roles/definitions"
	"github.com/kncept/guacamole/roles/simple"
)

func AllRoles() []definitions.Role {
	roles := make([]definitions.Role, 0)
	roles = append(roles, complex.LoadRoles()...)
	roles = append(roles, simple.LoadRoles()...)
	return roles
}

// from https://chatlyai.app/blog/best-system-prompts-for-everyone
func SuggestionSet1() []definitions.Role {
	return []definitions.Role{
		{RoleName: "Software Engineer", RoleSystemPrompt: `
		`},
	}
}
