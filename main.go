package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/skorten/granary/exporter"
	"github.com/skorten/granary/service"
	"github.com/spf13/cobra"
)

// version is set at build time via ldflags
var version = "dev"

func main() {
	var outputDir string
	var openAfter bool
	var forceAll bool

	rootCmd := &cobra.Command{
		Use:   "granary",
		Short: "Export your Granola meeting transcripts to markdown files",
		Long: "Granary downloads your Granola meeting transcripts and saves them as\n" +
			"markdown files on your Mac. Run `granary` (or `granary run`) to export.",
		// Bare `granary` runs the export, so a first-time user doesn't have to
		// discover the `run` subcommand.
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExport(resolveOutputDir(outputDir), openAfter, forceAll)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	rootCmd.PersistentFlags().StringVarP(&outputDir, "output-dir", "o", "", "Folder to save transcripts in (default: ~/Documents/Granola Transcripts)")
	rootCmd.PersistentFlags().BoolVar(&openAfter, "open", false, "Open the transcripts folder in Finder when done")
	rootCmd.Flags().BoolVar(&forceAll, "all", false, "Re-download every transcript, ignoring what's already saved")

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Download and export your transcripts",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExport(resolveOutputDir(outputDir), openAfter, forceAll)
		},
	}
	runCmd.Flags().BoolVar(&forceAll, "all", false, "Re-download every transcript, ignoring what's already saved")
	rootCmd.AddCommand(runCmd)

	// install
	var force bool
	var atTime string
	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Set up automatic daily exports (macOS background task)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return service.Install(force, atTime)
		},
	}
	installCmd.Flags().BoolVar(&force, "force", false, "Replace an existing background task")
	installCmd.Flags().StringVar(&atTime, "at", "", "Daily run time as HH:MM, 24-hour (default: a random time between 00:00 and 03:00)")
	rootCmd.AddCommand(installCmd)

	// uninstall
	uninstallCmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Stop the automatic exports",
		RunE: func(cmd *cobra.Command, args []string) error {
			return service.Uninstall()
		},
	}
	rootCmd.AddCommand(uninstallCmd)

	// status
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show whether automatic exports are set up",
		RunE: func(cmd *cobra.Command, args []string) error {
			installed, running, err := service.Status()
			if err != nil {
				return err
			}
			printStatus(installed, running)
			return nil
		},
	}
	rootCmd.AddCommand(statusCmd)

	// version
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Show version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(strings.TrimPrefix(version, "v"))
		},
	}
	rootCmd.AddCommand(versionCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "\nError: %s\n", err)
		os.Exit(1)
	}
}

func resolveOutputDir(outputDir string) string {
	if outputDir == "" {
		return exporter.DefaultOutputDir()
	}
	return outputDir
}

func runExport(outputDir string, openAfter bool, forceAll bool) error {
	// First-run preamble: explain plainly what is about to happen the first time,
	// before any credentials are touched.
	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		fmt.Println("Granary reads your existing Granola login from this Mac (no password needed)")
		fmt.Println("and downloads your meeting transcripts. Your login is never stored by Granary")
		fmt.Println("and is only sent to Granola's own servers.")
		fmt.Println()
	}

	supportDir, err := exporter.GranolaSupportDir()
	if err != nil {
		return err
	}

	token, err := exporter.AccessToken(supportDir)
	if err != nil {
		return err
	}

	client := &exporter.APIClient{
		BaseURL:   exporter.DefaultAPIBaseURL,
		Token:     token,
		Version:   exporter.GranolaClientVersion(),
		OutputDir: outputDir,
		ForceAll:  forceAll,
	}

	fmt.Println("Connecting to Granola and downloading your transcripts...")
	state, err := client.FetchState()
	if err != nil {
		return err
	}

	if len(state.Documents) == 0 {
		fmt.Println("\nNo meetings were found in your Granola account yet.")
		fmt.Println("Record or open a meeting in Granola, then run this again.")
		return nil
	}

	exp := exporter.NewExporter(outputDir)
	result, err := exp.Export(state, true)
	if err != nil {
		return err
	}

	result.PrintSummary(outputDir)
	fmt.Printf("\nYour transcripts are in: %s\n", outputDir)
	fmt.Printf("To open the folder, run:  open %q\n", outputDir)

	if openAfter {
		_ = exec.Command("open", outputDir).Run()
	}

	return nil
}

// printStatus reports the background-export state in plain language.
func printStatus(installed, running bool) {
	switch {
	case installed && running:
		fmt.Println("Automatic exports: ON — Granary backs up your transcripts once a day.")
	case installed:
		fmt.Println("Automatic exports: set up but not currently running (it runs once a day).")
	default:
		fmt.Println("Automatic exports: OFF. Run `granary install` to turn them on.")
	}
	fmt.Printf("Logs: %s\n", service.LogDir())
}
