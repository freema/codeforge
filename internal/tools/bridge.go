package tools

import (
	"github.com/freema/codeforge/internal/tool/mcp"
)

// ToMCPServers converts resolved ToolInstances into mcp.Server entries
// that can be passed to the MCP installer.
func ToMCPServers(instances []ToolInstance) []mcp.Server {
	if len(instances) == 0 {
		return nil
	}

	servers := make([]mcp.Server, 0, len(instances))
	for _, inst := range instances {
		def := inst.Definition
		if def.MCPPackage == "" {
			continue // skip tools without MCP backing
		}

		env := make(map[string]string)
		// Map config values to env vars based on field definitions
		mapConfigToEnv(def.RequiredConfig, inst.Config, env)
		mapConfigToEnv(def.OptionalConfig, inst.Config, env)

		srv := mcp.Server{
			Name:    def.Name,
			Package: def.MCPPackage,
			Args:    def.MCPArgs,
		}
		if len(env) > 0 {
			srv.Env = env
		}

		servers = append(servers, srv)
	}

	return servers
}

func mapConfigToEnv(fields []ConfigField, config map[string]string, env map[string]string) {
	for _, f := range fields {
		if f.EnvVar == "" {
			continue
		}
		if v, ok := config[f.Name]; ok && v != "" {
			env[f.EnvVar] = v
		}
	}
}
