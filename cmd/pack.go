package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"chin/internal/archive"
	"strconv"
	"strings"
	"time"

	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

var (
	packOutput   string
	packPassword string
	packSplit    string
)

func parseSize(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	s = strings.ToUpper(s)
	multiplier := int64(1)
	if strings.HasSuffix(s, "KB") {
		multiplier = 1024
		s = strings.TrimSuffix(s, "KB")
	} else if strings.HasSuffix(s, "MB") {
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "MB")
	} else if strings.HasSuffix(s, "GB") {
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "GB")
	}
	
	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, err
	}
	return val * multiplier, nil
}

func calculateTotalSize(paths []string) (int64, error) {
	var totalSize int64
	for _, path := range paths {
		err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				totalSize += info.Size()
			}
			return nil
		})
		if err != nil {
			return 0, err
		}
	}
	return totalSize, nil
}

var packCmd = &cobra.Command{
	Use:   "pack [file/folder]",
	Short: "Create a new archive",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		start := time.Now()
		
		if packOutput == "" {
			packOutput = filepath.Clean(args[0]) + ".chin"
		} else {
			packOutput = ensureChinExtension(packOutput)
		}

		splitSize, err := parseSize(packSplit)
		if err != nil {
			fmt.Printf("Invalid split size: %v\n", err)
			os.Exit(1)
		}

		// Calculate total size
		totalSize, err := calculateTotalSize(args)
		if err != nil {
			fmt.Printf("Error calculating size: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Packing %d input(s) to '%s' (Split: %v)...\n", len(args), packOutput, packSplit)

		writer, err := archive.NewWriter(packOutput, packPassword, splitSize)
		if err != nil {
			fmt.Printf("Error creating archive: %v\n", err)
			os.Exit(1)
		}
		defer writer.Close()

		bar := progressbar.DefaultBytes(
			totalSize,
			"packing",
		)
		
		writer.OnProgress = func(n int) {
			bar.Add(n)
		}
		
		writer.OnFileStart = func(name string) {
			// Limit name length to avoid breaking UI
			if len(name) > 30 {
				name = "..." + name[len(name)-27:]
			}
			bar.Describe(fmt.Sprintf("packing %s", name))
		}

		for _, input := range args {
			input = filepath.Clean(input)
			err = writer.AddFile(input, filepath.Base(input))
			if err != nil {
				fmt.Printf("Error adding '%s': %v\n", input, err)
				os.Exit(1)
			}
		}

		if err := writer.Finalize(packPassword); err != nil {
			fmt.Printf("Error finalizing archive: %v\n", err)
			os.Exit(1)
		}
		
		bar.Finish()
		fmt.Printf("\nDone in %v\n", time.Since(start))
	},
}

func init() {
	rootCmd.AddCommand(packCmd)
	packCmd.Flags().StringVarP(&packOutput, "output", "o", "", "Output archive path")
	packCmd.Flags().StringVarP(&packPassword, "password", "p", "", "Password for encryption")
	packCmd.Flags().StringVar(&packSplit, "split", "", "Split archive size (e.g. 10MB, 1GB)")
}
