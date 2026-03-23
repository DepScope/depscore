package main

import (
	"context"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"
	"github.com/depscope/depscope/internal/server"
	"github.com/depscope/depscope/internal/server/store"
)

func main() {
	tableName := os.Getenv("DYNAMODB_TABLE")
	if tableName == "" {
		tableName = "depscope-scans"
	}

	s, err := store.NewDynamoStore(context.Background(), tableName)
	if err != nil {
		log.Fatalf("create dynamo store: %v", err)
	}

	srv, err := server.NewServer(server.Options{
		Store: s,
		Mode:  server.ModeLambda,
	})
	if err != nil {
		log.Fatalf("create server: %v", err)
	}

	adapter := httpadapter.NewV2(srv.Handler())
	lambda.Start(adapter.ProxyWithContext)
}
