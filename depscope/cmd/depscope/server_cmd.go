package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/depscope/depscope/internal/server"
	"github.com/depscope/depscope/internal/server/store"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the depscope web server",
	RunE:  runServer,
}

func init() {
	serverCmd.Flags().Int("port", 8080, "port to listen on")
	serverCmd.Flags().String("store", "memory", "storage backend: memory, dynamo")
	serverCmd.Flags().String("table", "depscope-scans", "DynamoDB table name")
	rootCmd.AddCommand(serverCmd)
}

func runServer(cmd *cobra.Command, args []string) error {
	port, _ := cmd.Flags().GetInt("port")
	storeName, _ := cmd.Flags().GetString("store")

	var (
		s   store.ScanStore
		err error
	)
	switch storeName {
	case "dynamo":
		tableName, _ := cmd.Flags().GetString("table")
		s, err = store.NewDynamoStore(cmd.Context(), tableName)
		if err != nil {
			return fmt.Errorf("create dynamo store: %w", err)
		}
	default:
		s = store.NewMemoryStore()
	}

	srv, err := server.NewServer(server.Options{
		Store: s,
		Mode:  server.ModeLocal,
	})
	if err != nil {
		return fmt.Errorf("create server: %w", err)
	}

	addr := fmt.Sprintf(":%d", port)
	log.Printf("depscope server listening on http://localhost%s", addr)
	return http.ListenAndServe(addr, srv.Handler())
}
