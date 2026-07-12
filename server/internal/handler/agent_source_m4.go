package handler

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
)

// M4: capability version compatibility checking and source role lifecycle.
//
// When a capability is updated to a new version, the plan generator checks
// every consuming binding's version_requirement. If the new version does not
// satisfy the requirement, the affected role is marked "blocked-by-update" in
// the plan. The apply path refuses to activate a blocked role until either:
//   - the role's capability constraint is updated (by the workspace admin), or
//   - the prior capability version is pinned (rollback).
//
// This prevents silently breaking consumers — the plan shows every affected
// role before apply, and the last-good applied snapshot remains active.

// checkCapabilityCompatibility examines a capability's new version against all
// consuming bindings' version requirements. Returns a map of role_id → block
// reason for roles that would be blocked by the version change.
func (h *Handler) checkCapabilityCompatibility(ctx context.Context, sourceID pgtype.UUID, capSourceKey, newVersion string) map[string]string {
	blocked := map[string]string{}
	// Get the capability by source key.
	caps, err := h.Queries.ListSharedCapabilitiesBySource(ctx, sourceID)
	if err != nil {
		return blocked
	}
	var capID pgtype.UUID
	for _, c := range caps {
		if c.SourceKey == capSourceKey {
			capID = c.ID
			break
		}
	}
	if !capID.Valid {
		return blocked
	}
	// Find all bindings consuming this capability.
	bindings, err := h.Queries.ListAgentCapabilityBindingsByCapability(ctx, capID)
	if err != nil {
		return blocked
	}
	for _, b := range bindings {
		// Check if the new version satisfies the binding's version requirement.
		// For M4 we use a simple major-version check: if the major version
		// changes and the requirement uses ^ (caret), the binding is blocked.
		// A full semver constraint resolver can be added later.
		if !versionSatisfies(b.VersionRequirement, newVersion) {
			// Find the role_id for this agent from the source-role mapping.
			roleMappings, _ := h.Queries.ListAgentSourceRolesBySource(ctx, sourceID)
			for _, rm := range roleMappings {
				if rm.AgentID == b.AgentID {
					blocked[rm.SourceRoleID] = fmt.Sprintf(
						"capability %s updated to %s, but binding requires %s",
						capSourceKey, newVersion, b.VersionRequirement,
					)
					break
				}
			}
		}
	}
	return blocked
}

// detectRemovedRoles compares the scan manifest's role set against the source's
// existing role mappings. Roles present in the mapping but absent from the
// manifest are candidates for archive/detach. The plan proposes them as
// "archive-candidate" rather than silently deleting them.
func (h *Handler) detectRemovedRoles(ctx context.Context, sourceID pgtype.UUID, manifestRoles []map[string]any) []AgentSourceChange {
	existing, err := h.Queries.ListAgentSourceRolesBySource(ctx, sourceID)
	if err != nil {
		slog.Warn("agent_source: failed to list existing roles", "source_id", sourceID, "error", err)
		return nil
	}
	manifestIDs := make(map[string]bool, len(manifestRoles))
	for _, r := range manifestRoles {
		if id, ok := r["id"].(string); ok {
			manifestIDs[id] = true
		}
	}
	var changes []AgentSourceChange
	for _, rm := range existing {
		if !manifestIDs[rm.SourceRoleID] {
			changes = append(changes, AgentSourceChange{
				Key:    rm.SourceRoleID,
				Action: "archive-candidate",
				Reason: "role removed from source directory; propose archive or explicit detach",
			})
		}
	}
	return changes
}

// versionSatisfies performs a simplified semver compatibility check. For M4 it
// handles the common ^x.y.z (caret) and >=x.y.z constraints:
//   - ^1.0.0 satisfied by 1.x.y (same major)
//   - >=1.0.0 satisfied by any version >= 1.0.0
//   - bare 1.0.0 satisfied by exact match
//
// A full semver library can replace this later. Returns true when the
// requirement cannot be parsed (fail-open — the plan shows a warning, not a
// hard block).
func versionSatisfies(requirement, version string) bool {
	if requirement == "" || version == "" {
		return true // fail-open
	}
	reqMajor, reqMinor, reqPatch, ok := parseSemver(requirement)
	if !ok {
		return true
	}
	verMajor, verMinor, verPatch, ok := parseSemver(version)
	if !ok {
		return true
	}
	// Determine constraint prefix.
	prefix := ""
	for _, p := range []string{"^", "~", ">=", ">", "<=", "<"} {
		if len(requirement) >= len(p) && requirement[:len(p)] == p {
			prefix = p
			break
		}
	}
	// Strip prefix from the requirement for version comparison.
	reqMajor, reqMinor, reqPatch, _ = parseSemver(requirement[len(prefix):])

	switch prefix {
	case "^":
		// Caret: same major, minor/patch can be >=.
		return verMajor == reqMajor && (verMinor > reqMinor ||
			(verMinor == reqMinor && verPatch >= reqPatch))
	case ">=":
		return verMajor > reqMajor ||
			(verMajor == reqMajor && (verMinor > reqMinor ||
				(verMinor == reqMinor && verPatch >= reqPatch)))
	case "~":
		// Tilde: same major.minor, patch can be >=.
		return verMajor == reqMajor && verMinor == reqMinor && verPatch >= reqPatch
	default:
		// No prefix or exact match.
		return reqMajor == verMajor && reqMinor == verMinor && reqPatch == verPatch
	}
}

// parseSemver parses "x.y.z" into (major, minor, patch, ok). Ignores pre-release.
func parseSemver(v string) (int, int, int, bool) {
	var major, minor, patch int
	n, err := fmt.Sscanf(v, "%d.%d.%d", &major, &minor, &patch)
	return major, minor, patch, n == 3 && err == nil
}
