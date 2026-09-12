package catalogmeta

// Environment declares which runtime surfaces an operation is valid on. It
// distinguishes the CLI frontend, the local (stdio) MCP server, and the
// hosted (Portal-embedded) MCP server so an operation can declare where it
// belongs instead of frontends hard-coding name skip-lists. Consumers use it
// to gate per-operation registration and visibility for each frontend.
type Environment int

const (
	// EnvBoth is valid on every surface: the CLI frontend, local MCP, and
	// hosted MCP. It is the default for operations that make no carve-out.
	EnvBoth Environment = iota
	// EnvCLIOnly is valid only on the urfave CLI frontend; it is omitted from
	// every MCP surface. Used for plain SDK-call account credential ops that
	// collect an access credential (password) or duplicate an OOB hand-off
	// tool (account_update_email / account_update_password /
	// account_otp_disable). They stay reachable from the CLI but must never
	// accept credentials on, or be advertised to, a model channel.
	EnvCLIOnly
	// EnvLocalOnly is valid on the CLI frontend and the local (stdio) MCP
	// server, but is excluded from the hosted MCP surface. Used for operations
	// that mutate shared local config state (e.g. auth_login / auth_logout),
	// which is meaningless — and harmful — in a stateless Portal-embedded
	// server whose identity is established by Portal OAuth middleware.
	EnvLocalOnly
	// EnvHostedOnly is valid only on the hosted MCP surface. None are declared
	// today; the value exists for symmetry and future hosted-only operations.
	EnvHostedOnly
)

// opEnvironments maps operation IDs to their surface carve-out. Operations
// absent from the map are EnvBoth (valid on every surface), which is the
// default for operations that make no carve-out.
var opEnvironments = map[string]Environment{
	"account_otp_disable":     EnvCLIOnly,
	"account_update_email":    EnvCLIOnly,
	"account_update_password": EnvCLIOnly,
	"auth_login":              EnvLocalOnly,
	"auth_logout":             EnvLocalOnly,
}

// EnvironmentOf reports the surface carve-out declared for the operation with
// the given ID. Unknown (or undeclared) IDs return EnvBoth.
func EnvironmentOf(opID string) Environment {
	if e, ok := opEnvironments[opID]; ok {
		return e
	}
	return EnvBoth
}
