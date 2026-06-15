package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wassimk/granary/exporter"
	"github.com/wassimk/granary/service"
)

// version is set at build time via ldflags
var version = "dev"

func main() {
	rootCmd := &cobra.Command{
		Use:   "granary",
		Short: "Export Granola meeting transcripts to markdown",
	}

	// run
	var outputDir string
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run the export",
		RunE: func(cmd *cobra.Command, args []string) error {
			if outputDir == "" {
				outputDir = exporter.DefaultOutputDir()
			}
			return runExport(outputDir)
		},
	}
	runCmd.Flags().StringVarP(&outputDir, "output-dir", "o", "", "Custom output directory (default: ~/.local/share/granola-transcripts)")
	rootCmd.AddCommand(runCmd)

	// install
	var force bool
	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Install macOS LaunchAgent for scheduled exports",
		RunE: func(cmd *cobra.Command, args []string) error {
			return service.Install(force)
		},
	}
	installCmd.Flags().BoolVar(&force, "force", false, "Overwrite existing LaunchAgent")
	rootCmd.AddCommand(installCmd)

	// uninstall
	uninstallCmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the LaunchAgent",
		RunE: func(cmd *cobra.Command, args []string) error {
			return service.Uninstall()
		},
	}
	rootCmd.AddCommand(uninstallCmd)

	// status
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show whether the LaunchAgent is installed and running",
		RunE: func(cmd *cobra.Command, args []string) error {
			installed, running, err := service.Status()
			if err != nil {
				return err
			}

			label := service.Label
			plist := service.PlistPath()
			logDir := service.LogDir()

			fmt.Printf("Label:     %s\n", label)
			fmt.Printf("Plist:     %s\n", plist)
			fmt.Printf("Logs:      %s\n", logDir)
			fmt.Printf("Installed: %v\n", installed)
			fmt.Printf("Running:   %v\n", running)
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
		os.Exit(1)
	}
}

func runExport(outputDir string) error {
	supportDir, err := exporter.GranolaSupportDir()
	if err != nil {
		return err
	}

	token, err := exporter.AccessToken(supportDir)
	if err != nil {
		return err
	}

	client := &exporter.APIClient{
		BaseURL: exporter.DefaultAPIBaseURL,
		Token:   token,
		Version: exporter.GranolaClientVersion(),
	}

	fmt.Printf("Fetching documents and transcripts from Granola API (client %s)...\n", client.Version)
	state, err := client.FetchState()
	if err != nil {
		return err
	}

	fmt.Printf("Found %d documents\n", len(state.Documents))
	fmt.Printf("Found %d transcripts\n\n", len(state.Transcripts))

	exp := exporter.NewExporter(outputDir)
	result, err := exp.Export(state, true)
	if err != nil {
		return err
	}

	result.PrintSummary(outputDir)

	return nil
}
