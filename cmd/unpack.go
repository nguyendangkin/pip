package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"chin/internal/archive"
	"time"

	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

var (
	unpackOutput   string
	unpackPassword string
	unpackWrap     bool
)

var unpackCmd = &cobra.Command{
	Use:   "unpack [archive.chin]",
	Short: "Extract files from an archive",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		start := time.Now()
		input := ensureChinExtension(args[0])
		
		if unpackOutput == "" {
			unpackOutput = "."
		}

		if unpackWrap {
			// Get filename without extension
			baseName := filepath.Base(input)
			ext := filepath.Ext(baseName)
			folderName := baseName[:len(baseName)-len(ext)]
			unpackOutput = filepath.Join(unpackOutput, folderName)

			// Check if collision with existing file
			if info, err := os.Stat(unpackOutput); err == nil && !info.IsDir() {
				// Conflict: Target exists and is a file
				unpackOutput += "_unpacked"
			}
		}

		fmt.Printf("Unpacking '%s' to '%s'...\n", input, unpackOutput)

		reader, err := archive.NewReader(input, unpackPassword)
		if err != nil {
			fmt.Printf("Error opening archive: %v\n", err)
			os.Exit(1)
		}
		defer reader.Close()

		// Calculate total size for progress bar
		var totalSize int64
		for _, file := range reader.ListFiles() {
			totalSize += int64(file.Size)
		}

		bar := progressbar.DefaultBytes(
			totalSize,
			"unpacking",
		)

		reader.OnProgress = func(n int) {
			bar.Add(n)
		}

		reader.OnFileStart = func(name string) {
			// Limit name length
			if len(name) > 30 {
				name = "..." + name[len(name)-27:]
			}
			bar.Describe(fmt.Sprintf("unpacking %s", name))
		}

		if err := reader.ExtractAll(unpackOutput, true); err != nil {
			fmt.Printf("Error extracting archive: %v\n", err)
			os.Exit(1)
		}

		bar.Finish()
		fmt.Printf("\nDone in %v\n", time.Since(start))
	},
}

func init() {
	rootCmd.AddCommand(unpackCmd)
	unpackCmd.Flags().StringVarP(&unpackOutput, "destination", "d", "", "Destination directory")
	unpackCmd.Flags().StringVarP(&unpackPassword, "password", "p", "", "Password for decryption")
	unpackCmd.Flags().BoolVar(&unpackWrap, "wrap", false, "Wrap extracted files in a parent folder derived from archive name")
}
