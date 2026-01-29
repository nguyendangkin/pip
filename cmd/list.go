package cmd

import (
	"fmt"
	"os"
	"chin/internal/archive"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var listPassword string

var listCmd = &cobra.Command{
	Use:   "list [archive.chin]",
	Short: "List files in an archive",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		input := ensureChinExtension(args[0])

		reader, err := archive.NewReader(input, listPassword)
		if err != nil {
			fmt.Printf("Error opening archive: %v\n", err)
			os.Exit(1)
		}
		defer reader.Close()

		files := reader.ListFiles()
		
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "MODE\tSIZE\tNAME")
		for _, f := range files {
			modeStr := "FILE"
			if f.IsDir {
				modeStr = "DIR "
			}
			fmt.Fprintf(w, "%s\t%d\t%s\n", modeStr, f.Size, f.Name)
		}
		w.Flush()
		
		fmt.Printf("\nTotal: %d files\n", len(files))
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
	listCmd.Flags().StringVarP(&listPassword, "password", "p", "", "Password for decryption")
}
