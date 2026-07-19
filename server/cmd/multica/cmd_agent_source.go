package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
)

// agentSourceCmd is the CLI surface for the AgentWaker directory integration:
//
//	multica agent-source add --type agentwaker-directory --daemon-id <rt> --path /abs/path
//	multica agent-source scan <source-id> [--wait] [--output json]
//	multica agent-source plan <source-id> [--snapshot <id>] [--output json]
//	multica agent-source apply <source-id> --snapshot <id> [--env-file <path>] [--wait]
//	multica agent-source status <source-id>
//	multica agent-source rollback <source-id> --snapshot <id>
//
// Every command reads/writes through the authenticated Multica API. Env values
// supplied to `apply` travel in the explicit authenticated apply payload (the
// same value-safe channel the UI uses); they are never printed.
var agentSourceCmd = &cobra.Command{
	Use:   "agent-source",
	Short: "Configure and sync AgentWaker directory sources",
}

var agentSourceAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Configure a new AgentWaker directory source",
	RunE:  runAgentSourceAdd,
}

var agentSourceScanCmd = &cobra.Command{
	Use:   "scan <source-id>",
	Short: "Initiate a read-only directory scan",
	Args:  exactArgs(1),
	RunE:  runAgentSourceScan,
}

var agentSourcePlanCmd = &cobra.Command{
	Use:   "plan <source-id>",
	Short: "Show the read-only import plan for the latest (or a given) snapshot",
	Args:  exactArgs(1),
	RunE:  runAgentSourcePlan,
}

var agentSourceApplyCmd = &cobra.Command{
	Use:   "apply <source-id>",
	Short: "Atomically apply a snapshot (capabilities, roles, skills, bindings, env)",
	Args:  exactArgs(1),
	RunE:  runAgentSourceApply,
}

var agentSourceStatusCmd = &cobra.Command{
	Use:   "status <source-id>",
	Short: "Show one source's configuration and latest snapshot",
	Args:  exactArgs(1),
	RunE:  runAgentSourceStatus,
}

var agentSourceRollbackCmd = &cobra.Command{
	Use:   "rollback <source-id>",
	Short: "Re-apply a prior superseded snapshot",
	Args:  exactArgs(1),
	RunE:  runAgentSourceRollback,
}

func init() {
	agentSourceAddCmd.Flags().String("type", "agentwaker-directory", "source type")
	agentSourceAddCmd.Flags().String("daemon-id", "", "owning daemon runtime id")
	agentSourceAddCmd.Flags().String("path", "", "absolute AgentWaker root path on the daemon")
	agentSourceAddCmd.Flags().String("sync-mode", "manual", "sync mode: manual|scheduled|watch-assisted")
	_ = agentSourceAddCmd.MarkFlagRequired("daemon-id")
	_ = agentSourceAddCmd.MarkFlagRequired("path")

	agentSourceScanCmd.Flags().Bool("wait", false, "wait for the scan to reach a terminal status")
	agentSourceScanCmd.Flags().String("output", "text", "output format: text or json")

	agentSourcePlanCmd.Flags().String("snapshot", "", "snapshot id (defaults to latest)")
	agentSourcePlanCmd.Flags().String("output", "text", "output format: text or json")

	agentSourceApplyCmd.Flags().String("snapshot", "", "snapshot id to apply")
	agentSourceApplyCmd.Flags().String("env-file", "", "path to a JSON file of {role: {var: value}} env values")
	agentSourceApplyCmd.Flags().String("merge-mode", "source-authoritative", "env merge mode: source-authoritative|merge-preserve")
	agentSourceApplyCmd.Flags().Bool("wait", false, "wait for apply to complete")
	_ = agentSourceApplyCmd.MarkFlagRequired("snapshot")

	agentSourceRollbackCmd.Flags().String("snapshot", "", "snapshot id to roll back to")
	_ = agentSourceRollbackCmd.MarkFlagRequired("snapshot")

	agentSourceCmd.AddCommand(agentSourceAddCmd)
	agentSourceCmd.AddCommand(agentSourceScanCmd)
	agentSourceCmd.AddCommand(agentSourcePlanCmd)
	agentSourceCmd.AddCommand(agentSourceApplyCmd)
	agentSourceCmd.AddCommand(agentSourceStatusCmd)
	agentSourceCmd.AddCommand(agentSourceRollbackCmd)
}

func runAgentSourceAdd(cmd *cobra.Command, _ []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	daemonID, _ := cmd.Flags().GetString("daemon-id")
	path, _ := cmd.Flags().GetString("path")
	syncMode, _ := cmd.Flags().GetString("sync-mode")
	body := map[string]any{
		"daemon_runtime_id": daemonID,
		"local_path":        path,
		"sync_mode":         syncMode,
	}
	var created map[string]any
	if err := client.PostJSON(ctx, "/api/agent-sources", body, &created); err != nil {
		return fmt.Errorf("create source: %w", err)
	}
	printJSON(created)
	return nil
}

func runAgentSourceScan(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	sourceID := args[0]
	var initiated map[string]any
	if err := client.PostJSON(ctx, "/api/agent-sources/"+sourceID+"/scan", nil, &initiated); err != nil {
		return fmt.Errorf("initiate scan: %w", err)
	}
	requestID, _ := initiated["id"].(string)
	fmt.Fprintln(os.Stderr, "scan started:", requestID)

	wait, _ := cmd.Flags().GetBool("wait")
	if !wait {
		printJSON(initiated)
		return nil
	}

	// Poll until terminal.
	for {
		var req map[string]any
		path := fmt.Sprintf("/api/agent-sources/%s/scan/%s", sourceID, requestID)
		if err := client.GetJSON(ctx, path, &req); err != nil {
			return fmt.Errorf("poll scan: %w", err)
		}
		status, _ := req["status"].(string)
		if status == "completed" || status == "failed" || status == "timeout" {
			output, _ := cmd.Flags().GetString("output")
			if output == "json" {
				// The scan manifest may contain scoped source bodies needed by
				// apply. Never print it from an ordinary CLI scan summary.
				printJSON(agentSourceScanSummary(req))
			} else {
				fmt.Println("status:", status)
				if h, ok := req["directory_hash"].(string); ok && h != "" {
					fmt.Println("directory_hash:", h)
				}
				if e, ok := req["error"].(string); ok && e != "" {
					fmt.Println("error:", e)
				}
			}
			if status != "completed" {
				return fmt.Errorf("scan ended with status %s", status)
			}
			return nil
		}
		time.Sleep(2 * time.Second)
	}
}

func agentSourceScanSummary(req map[string]any) map[string]any {
	summary := make(map[string]any, len(req))
	for key, value := range req {
		if key == "manifest" {
			continue
		}
		summary[key] = value
	}
	return summary
}

func runAgentSourcePlan(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	sourceID := args[0]
	params := url.Values{}
	if snap, _ := cmd.Flags().GetString("snapshot"); snap != "" {
		params.Set("snapshot", snap)
	}
	path := "/api/agent-sources/" + sourceID + "/plan"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	var plan map[string]any
	if err := client.GetJSON(ctx, path, &plan); err != nil {
		return fmt.Errorf("build plan: %w", err)
	}
	printJSON(plan)
	return nil
}

func runAgentSourceApply(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	sourceID := args[0]
	snapshot, _ := cmd.Flags().GetString("snapshot")
	mergeMode, _ := cmd.Flags().GetString("merge-mode")
	envFile, _ := cmd.Flags().GetString("env-file")

	body := map[string]any{
		"snapshot_id":    snapshot,
		"env_merge_mode": mergeMode,
	}
	// Env values are normally derived server-side from the scoped env/.env
	// source body. An explicit JSON payload overrides those values and is never
	// printed.
	if envFile != "" {
		raw, rerr := os.ReadFile(envFile)
		if rerr != nil {
			return fmt.Errorf("read env-file: %w", rerr)
		}
		var envValues map[string]map[string]string
		if err := json.Unmarshal(raw, &envValues); err != nil {
			return fmt.Errorf("parse env-file (expect {role: {var: value}}): %w", err)
		}
		body["env_values"] = envValues
	}

	var result map[string]any
	if err := client.PostJSON(ctx, "/api/agent-sources/"+sourceID+"/apply", body, &result); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	printJSON(result)
	return nil
}

func runAgentSourceStatus(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	sourceID := args[0]
	var src map[string]any
	if err := client.GetJSON(ctx, "/api/agent-sources/"+sourceID, &src); err != nil {
		return fmt.Errorf("get source: %w", err)
	}
	printJSON(src)
	return nil
}

func runAgentSourceRollback(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	sourceID := args[0]
	snapshot, _ := cmd.Flags().GetString("snapshot")
	body := map[string]any{"snapshot_id": snapshot}
	var result map[string]any
	if err := client.PostJSON(ctx, "/api/agent-sources/"+sourceID+"/rollback", body, &result); err != nil {
		return fmt.Errorf("rollback: %w", err)
	}
	printJSON(result)
	return nil
}

// printJSON pretty-prints a value as JSON to stdout.
func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
