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

		if def.MCPTransport == "http" {
			if def.MCPURL == "" {
				continue // skip HTTP tools without URL
			}
			headers := make(map[string]string)
			mapConfigToEnv(def.RequiredConfig, inst.Config, headers)
			mapConfigToEnv(def.OptionalConfig, inst.Config, headers)

			srv := mcp.Server{
				Name:      def.Name,
				Transport: "http",
				URL:       def.MCPURL,
			}
			if len(headers) > 0 {
				srv.Headers = headers
			}
			servers = append(servers, srv)
			continue
		}

		// stdio transport
		if def.MCPPackage == "" {
			continue // skip stdio tools without package
		}

		env := make(map[string]string)
		mapConfigToEnv(def.RequiredConfig, inst.Config, env)
		mapConfigToEnv(def.OptionalConfig, inst.Config, env)

		srv := mcp.Server{
			Name:    def.Name,
			Command: def.MCPCommand,
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
