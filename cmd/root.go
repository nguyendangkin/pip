package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "chin",
	Short: "chin is a fast and secure file archiver",
	Long:  `chin is a CLI tool for packing, unpackaging, and managing secure file archives with speed and efficiency.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	// Global flags can be defined here
}
