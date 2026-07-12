package featureflags

import (
	"context"

	"github.com/multica-ai/multica/server/pkg/featureflag"
)

const (
	// ComposioMCPApps gates the Composio app management UI and — together with
	// the MUL-3963 permission_mode / invocation_targets access model it depends
	// on — the aligned Private / Public-to picker in the agent create flow.
	// The access model exists to gate Composio sharing, so the two ship on the
	// same switch.
	ComposioMCPApps = "composio_mcp_apps"

	// AgentWakerDirectorySync gates the AgentWaker directory integration:
	// source configuration, the daemon scan protocol, sanitized snapshot
	// previews, atomic import (M2), runtime capability injection (M3), and
	// continuous resync (M4). Ships dark; flip per workspace to enable.
	AgentWakerDirectorySync = "agentwaker_directory_sync"
)

var frontendPublicFlags = []string{
	ComposioMCPApps,
}

func ComposioMCPAppsEnabled(ctx context.Context, flags *featureflag.Service) bool {
	return flags.IsEnabled(ctx, ComposioMCPApps, false)
}

// AgentWakerDirectorySyncEnabled reports whether the AgentWaker directory
// integration is on for the current context. Defaults off.
func AgentWakerDirectorySyncEnabled(ctx context.Context, flags *featureflag.Service) bool {
	return flags.IsEnabled(ctx, AgentWakerDirectorySync, false)
}

func EvaluateFrontendPublicFlags(ctx context.Context, flags *featureflag.Service) map[string]bool {
	out := make(map[string]bool, len(frontendPublicFlags))
	for _, key := range frontendPublicFlags {
		out[key] = flags.IsEnabled(ctx, key, false)
	}
	return out
}
