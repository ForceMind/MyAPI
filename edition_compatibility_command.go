package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/service"
	"github.com/joho/godotenv"
)

// The explicit flag is recognized before normal initialization. A positional
// subcommand is deliberately not used: old binaries can ignore positional
// arguments and accidentally start a second server.
func handleEditionCompatibilityCommand(args []string, output io.Writer) (bool, int) {
	requested := false
	for _, arg := range args {
		if arg == "--edition-check" || strings.HasPrefix(arg, "--edition-check=") || arg == "-edition-check" || strings.HasPrefix(arg, "-edition-check=") {
			requested = true
			break
		}
	}
	if !requested {
		return false, 0
	}
	flags := flag.NewFlagSet("edition-check", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	target := flags.String("edition-check", "", "check target edition without starting MyAPI")
	jsonOutput := flags.Bool("json", false, "print the read-only report as JSON")
	err := flags.Parse(args)
	if errors.Is(err, flag.ErrHelp) {
		_, writeErr := fmt.Fprintln(output, "Usage: my-api --edition-check=<full|lite|lan> [--json]\nThis only checks configuration. It does not change edition, migrate data or test a login.")
		if writeErr != nil {
			return true, 2
		}
		return true, 0
	}
	var report service.EditionCompatibilityReport
	if err != nil || flags.NArg() != 0 {
		report = service.EditionCompatibilityReport{
			SchemaVersion: 1, Check: "edition-compatibility", ReadOnly: true,
			State: service.EditionConfigurationUnverifiable, Scope: "configuration", LoginContinuity: "not-verified",
			Blockers: []string{}, Unknowns: []string{"invalid-request"}, Warnings: []string{"local-login-not-verified"},
		}
	} else {
		// Match normal startup's .env precedence and SQLite selection, but never
		// call InitEnv: it creates log directories and initializes other state.
		_ = godotenv.Load(".env")
		previousSQLitePath := common.SQLitePath
		common.SQLitePath = common.ResolveSQLitePath(os.Getenv("SQLITE_PATH"))
		defer func() { common.SQLitePath = previousSQLitePath }()
		report = service.CheckEditionCompatibility(context.Background(), os.Getenv("MYAPI_EDITION"), *target)
	}
	if *jsonOutput {
		encoded, encodeErr := common.Marshal(report)
		if encodeErr != nil {
			return true, 2
		}
		if _, writeErr := fmt.Fprintf(output, "%s\n", encoded); writeErr != nil {
			return true, 2
		}
	} else if writeErr := writeEditionCompatibilitySummary(output, report); writeErr != nil {
		return true, 2
	}
	return true, report.ExitCode()
}

func writeEditionCompatibilitySummary(output io.Writer, report service.EditionCompatibilityReport) error {
	source, target := "unknown", "unknown"
	if report.SourceEdition != nil {
		source = *report.SourceEdition
	}
	if report.TargetEdition != nil {
		target = *report.TargetEdition
	}
	var text strings.Builder
	fmt.Fprintf(&text, "MyAPI edition configuration: %s\nSource: %s; target: %s\n", report.State, source, target)
	text.WriteString("Read-only check. Login was not tested; this report does not authorize a deployment change.\n")
	if report.Facts != nil {
		fmt.Fprintf(&text, "Database schema: %s; source: %s\n", report.Facts.Schema, report.Facts.Source)
		if report.Facts.TopUps == nil {
			text.WriteString("Top-up orders: unknown\n")
		} else {
			fmt.Fprintf(&text, "Top-up orders: %d pending; %d unrecognized\n", report.Facts.TopUps.Pending, report.Facts.TopUps.Unknown)
		}
		if report.Facts.SubscriptionOrders == nil {
			text.WriteString("Subscription purchase orders: unknown\n")
		} else {
			fmt.Fprintf(&text, "Subscription purchase orders: %d pending; %d unrecognized\n", report.Facts.SubscriptionOrders.Pending, report.Facts.SubscriptionOrders.Unknown)
		}
		if report.Facts.Authentication == nil {
			text.WriteString("Local login configuration: unknown\n")
		} else {
			fmt.Fprintf(&text, "Local login candidates: %d of %d enabled accounts; actual login not verified\n", report.Facts.Authentication.LocalCandidates, report.Facts.Authentication.EnabledUsers)
		}
	}
	advice := map[string]string{
		"pending-top-ups":             "Complete or resolve pending top-up orders through their existing Full payment flow, then check again.",
		"pending-subscription-orders": "Complete or resolve pending subscription purchases in Full, then check again. Existing subscription benefits are separate.",
		"missing-local-login":         "Use the existing account recovery, password or Passkey setup in Full. Confirm local login before changing edition.",
		"database-unavailable":        "The configured primary database could not be inspected. Check its connection and read permissions.",
		"schema-unverifiable":         "The existing schema is incomplete or cannot be identified. Keep the existing edition and review it before retrying.",
		"order-status-unverifiable":   "Some order states cannot be identified. Review them in the existing Full installation; this check does not change them.",
		"authentication-unverifiable": "Local login settings or credential records could not be verified. Review them in Full; this check does not reset credentials.",
		"invalid-source-edition":      "The current MYAPI_EDITION is not recognized. Keep the existing installation unchanged.",
		"invalid-target-edition":      "Choose full, lite or the legacy alias lan as the target.",
		"invalid-request":             "Use my-api --edition-check=<full|lite|lan> [--json]. No other startup arguments are accepted in this mode.",
	}
	for _, code := range append(append([]string{}, report.Blockers...), report.Unknowns...) {
		fmt.Fprintf(&text, "- %s\n", advice[code])
	}
	_, err := io.WriteString(output, text.String())
	return err
}
