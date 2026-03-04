package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "GlimmerWorksCli",
	Short: "Build and manage dependencies for GlimmerWorks projects",
	Long:  `Build and manage dependencies for GlimmerWorks projects`,
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
