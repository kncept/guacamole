// Package tools implements the ai.Tool functions the model can call:
// read_file, ls and write_file.
package tools

import (
	"github.com/kncept/guacamole/ai"
)

// FileSystem returns all the file tools: read_file, ls and write_file.
func AllTools() []ai.Tool {
	tools := make([]ai.Tool, 0)
	tools = append(tools, FileSystem()...)
	tools = append(tools, Console()...)
	return tools
}
