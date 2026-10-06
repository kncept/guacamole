package simple

import (
	"embed"
	"slices"
	"strings"

	"github.com/kncept/guacamole/roles/definitions"
)

//go:embed *.txt
var staticFiles embed.FS

// https://chatlyai.app/blog/best-system-prompts-for-everyone
func LoadRoles() []definitions.Role {
	roles := make([]definitions.Role, 0)
	entries, _ := staticFiles.ReadDir(".")
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".txt") {
			data, err := staticFiles.ReadFile(entry.Name())
			if err == nil {
				roles = append(roles, definitions.Role{
					RoleName:         entry.Name(),
					RoleSetName:      "simple",
					RoleSystemPrompt: string(data),
				})
			}
		}
	}
	slices.SortStableFunc(roles, func(r1, r2 definitions.Role) int {
		return strings.Compare(r1.RoleName, r2.RoleName)
	})
	return roles
}
