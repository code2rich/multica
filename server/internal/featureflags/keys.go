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
	// AgentBuilder controls writes of system builder agents. It stays disabled
	// through the schema-only rollout so an older server cannot expose them.
	AgentBuilder = "agents_agent_builder"
	// ResourceLabels controls the agent- and skill-scoped label namespaces.
	// Issue labels remain available while this release flag is off.
	ResourceLabels = "settings_resource_labels"
	// agentSkillTogglesCompat is no longer a release flag. Keep publishing the
	// key as enabled so installed v0.4.0 desktop clients, which still gate the
	// switch on this config decision, receive the permanently enabled behavior.
	agentSkillTogglesCompat = "agents_skill_toggles"
)

var frontendPublicFlags = []string{
	ComposioMCPApps,
	AgentWakerDirectorySync,
	AgentBuilder,
	ResourceLabels,
}

func ComposioMCPAppsEnabled(ctx context.Context, flags *featureflag.Service) bool {
	return flags.IsEnabled(ctx, ComposioMCPApps, false)
}

// AgentWakerDirectorySyncEnabled reports whether the AgentWaker directory
// integration is on for the current context. Defaults off.
func AgentWakerDirectorySyncEnabled(ctx context.Context, flags *featureflag.Service) bool {
	return flags.IsEnabled(ctx, AgentWakerDirectorySync, false)
}

func AgentBuilderEnabled(ctx context.Context, flags *featureflag.Service) bool {
	return flags.IsEnabled(ctx, AgentBuilder, false)
}

func ResourceLabelsEnabled(ctx context.Context, flags *featureflag.Service) bool {
	return flags.IsEnabled(ctx, ResourceLabels, false)
}

func EvaluateFrontendPublicFlags(ctx context.Context, flags *featureflag.Service) map[string]bool {
	out := make(map[string]bool, len(frontendPublicFlags)+1)
	for _, key := range frontendPublicFlags {
		out[key] = flags.IsEnabled(ctx, key, false)
	}
	out[agentSkillTogglesCompat] = true
	return out
}
