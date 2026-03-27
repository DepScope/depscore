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
	serverCmd.Flags().String("store", "memory", "storage backend: memory, sqlite, dynamo")
	serverCmd.Flags().String("table", "depscope-scans", "DynamoDB table name")
	serverCmd.Flags().String("db-path", "./depscope.db", "SQLite database file path")
	serverCmd.Flags().String("cache-db", "", "path to dependency cache database")
	serverCmd.Flags().StringSlice("trusted-orgs", nil, "GitHub orgs to treat as trusted")
	rootCmd.AddCommand(serverCmd)
}

func runServer(cmd *cobra.Command, args []string) error {
	port, _ := cmd.Flags().GetInt("port")
	storeName, _ := cmd.Flags().GetString("store")

	var (
		s          store.ScanStore
		graphStore store.GraphStore
		err        error
	)
	switch storeName {
	case "sqlite":
		dbPath, _ := cmd.Flags().GetString("db-path")
		sq, sqErr := store.NewSQLiteStore(dbPath)
		if sqErr != nil {
			return fmt.Errorf("create sqlite store: %w", sqErr)
		}
		s = sq
		graphStore = sq
	case "dynamo":
		tableName, _ := cmd.Flags().GetString("table")
		s, err = store.NewDynamoStore(cmd.Context(), tableName)
		if err != nil {
			return fmt.Errorf("create dynamo store: %w", err)
		}
	default:
		s = store.NewMemoryStore()
	}

	cacheDB, _ := cmd.Flags().GetString("cache-db")
	trustedOrgs, _ := cmd.Flags().GetStringSlice("trusted-orgs")

	srv, err := server.NewServer(server.Options{
		Store:       s,
		GraphStore:  graphStore,
		Mode:        server.ModeLocal,
		CacheDBPath: cacheDB,
		TrustedOrgs: trustedOrgs,
	})
	if err != nil {
		return fmt.Errorf("create server: %w", err)
	}

	addr := fmt.Sprintf(":%d", port)
	log.Printf("depscope server listening on http://localhost%s", addr)
	return http.ListenAndServe(addr, srv.Handler())
}
