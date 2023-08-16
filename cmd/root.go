/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"os"

	"github.com/jphastings/dnslink-pinner/internal/monitor"
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "dnslink-pinner",
	Short: "Pins DNSLink records to IPFS",
	Long: `Ensures that the monitored DNSLink records have the latest CID pinned on the
target IPFS node, by polling DNS and rotating the pin if the CID changes.`,
	Args:      cobra.MaximumNArgs(1),
	ValidArgs: []string{"repoPath"},
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}

		r, err := monitor.New(os.DirFS(dir))
		if err != nil {
			return err
		}

		// TODO: retry & exponential backoff
		return r.Monitor()
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {

}
