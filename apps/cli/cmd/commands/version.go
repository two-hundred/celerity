package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of Celerity CLI",
	Long:  `All software has versions. This is Celerity CLI's`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Celerity CLI v0.1")
	},
}