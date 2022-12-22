// cmd/root.go
// Server main command.

package cmd

import (
	"fmt"
	"os"
	"runtime"

	"github.com/cubeflix/lily-http/server"
	"github.com/cubeflix/lily/version"
	"github.com/spf13/cobra"
)

var insecure bool
var host string
var port int
var lilyHost string
var lilyPort int
var cert string
var key string

// Base Lily command.
var RootCmd = &cobra.Command{
	Use:   "lily-http",
	Short: "The HTTP server for Lily, a secure file server.",
	Long:  `lily-http is the Lily HTTP server.`,
	Run: func(cmd *cobra.Command, args []string) {
		server.Serve(insecure, host, port, lilyHost, lilyPort, cert, key)
	},
}

// Version command.
var VersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version and exit.",
	Long:  `Print the Lily server version number.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("lily", version.VERSION, runtime.GOOS)
	},
}

// Execute the root command.
func Execute() {
	// Execute the main command.
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// Init cobra.
func init() {
	// Set the arguments.
	RootCmd.PersistentFlags().StringVar(&host, "host", "", "The host to listen on")
	RootCmd.PersistentFlags().IntVar(&port, "port", 443, "The port to listen on")
	RootCmd.PersistentFlags().StringVar(&lilyHost, "lily-host", "", "The Lily server host")
	RootCmd.PersistentFlags().IntVar(&lilyPort, "lily-port", 42069, "The Lily server port")
	RootCmd.PersistentFlags().BoolVarP(&insecure, "insecure", "i", false, "If we should ignore the certificate from the server")
	RootCmd.PersistentFlags().StringVar(&cert, "cert", "", "The certificate file")
	RootCmd.PersistentFlags().StringVar(&key, "key", "", "The key file")

	// Add the commands.
	RootCmd.AddCommand(VersionCmd)
}
