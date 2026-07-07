// Package main is the WeKnora admin CLI for maintenance tasks on Milvus
// collections (cleanup, inspection).
//
// Subcommands:
//
//	drop-summary       - drop the global weknora_summary collection
//	drop-legacy-kb     - drop the pre-refactor global weknora_kb collection
//	drop-kb <kb-id>    - drop the per-KB weknora_kb_<kbid> collection
//	inspect-summary    - print whether weknora_summary exists and its dim
//
// Connection parameters are read from the standard WeKnora env vars:
//
//	MILVUS_ADDRESS   (default: localhost:19530)
//	MILVUS_USERNAME  (optional)
//	MILVUS_PASSWORD  (optional)
//	MILVUS_DB_NAME   (optional)
//	MILVUS_COLLECTION (default: weknora_kb, used as the base name)
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	client "github.com/milvus-io/milvus/client/v2/milvusclient"
	"google.golang.org/grpc"
)

const (
	defaultBaseCollection = "weknora_kb"
	summaryCollectionName = "summary_knowledge_base"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cli, err := newMilvusClient(ctx)
	if err != nil {
		log.Fatalf("failed to connect to Milvus: %v", err)
	}
	defer func() { _ = cli.Close(ctx) }()

	switch cmd {
	case "drop-summary":
		runDrop(ctx, cli, summaryCollectionName)
	case "drop-legacy-kb":
		// Pre-refactor global chunks collection. After the per-KB split there
		// is no data in it; this command removes it cleanly.
		runDrop(ctx, cli, baseCollectionName())
	case "drop-kb":
		if len(args) < 1 {
			log.Fatalf("drop-kb requires <kb-id>")
		}
		runDrop(ctx, cli, perKBCollectionName(args[0]))
	case "inspect-summary":
		runInspectSummary(ctx, cli)
	case "help", "-h", "--help":
		printUsage()
	default:
		log.Printf("unknown subcommand: %q", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(strings.TrimSpace(`
weknora-admin: Milvus maintenance CLI.

Usage:
  weknora-admin <command> [args]

Commands:
  drop-summary          drop the global weknora_summary collection
  drop-legacy-kb        drop the pre-refactor global chunks collection
                        (named by MILVUS_COLLECTION, default weknora_kb)
  drop-kb <kb-id>       drop the per-KB collection for the given KB id
  inspect-summary       print whether weknora_summary exists and its dim
  help                  show this message

Environment variables:
  MILVUS_ADDRESS        Milvus server (default: localhost:19530)
  MILVUS_USERNAME       optional username
  MILVUS_PASSWORD       optional password
  MILVUS_DB_NAME        optional database name
  MILVUS_COLLECTION     base collection name (default: weknora_kb)
`))
}

func newMilvusClient(ctx context.Context) (*client.Client, error) {
	cfg := client.ClientConfig{
		DialOptions: []grpc.DialOption{grpc.WithTimeout(5 * time.Second)},
	}
	if addr := os.Getenv("MILVUS_ADDRESS"); addr != "" {
		cfg.Address = addr
	} else {
		cfg.Address = "localhost:19530"
	}
	if u := os.Getenv("MILVUS_USERNAME"); u != "" {
		cfg.Username = u
	}
	if p := os.Getenv("MILVUS_PASSWORD"); p != "" {
		cfg.Password = p
	}
	if db := os.Getenv("MILVUS_DB_NAME"); db != "" {
		cfg.DBName = db
	}
	return client.New(ctx, &cfg)
}

func baseCollectionName() string {
	if name := os.Getenv("MILVUS_COLLECTION"); name != "" {
		return name
	}
	return defaultBaseCollection
}

// perKBCollectionName mirrors the sanitisation rule used in
// internal/application/repository/retriever/milvus/repository.go to keep
// naming consistent across the server and this CLI.
func perKBCollectionName(kbID string) string {
	base := baseCollectionName()
	replacer := strings.NewReplacer("-", "_", ".", "_", "/", "_")
	return fmt.Sprintf("%s_%s", base, replacer.Replace(kbID))
}

func runDrop(ctx context.Context, cli *client.Client, name string) {
	has, err := cli.HasCollection(ctx, client.NewHasCollectionOption(name))
	if err != nil {
		log.Fatalf("failed to check %s: %v", name, err)
	}
	if !has {
		fmt.Printf("collection %s does not exist, nothing to do\n", name)
		return
	}
	if err := cli.DropCollection(ctx, client.NewDropCollectionOption(name)); err != nil {
		log.Fatalf("failed to drop %s: %v", name, err)
	}
	fmt.Printf("dropped collection %s\n", name)
}

func runInspectSummary(ctx context.Context, cli *client.Client) {
	has, err := cli.HasCollection(ctx, client.NewHasCollectionOption(summaryCollectionName))
	if err != nil {
		log.Fatalf("failed to check %s: %v", summaryCollectionName, err)
	}
	if !has {
		fmt.Printf("collection %s does not exist\n", summaryCollectionName)
		return
	}
	coll, err := cli.DescribeCollection(ctx, client.NewDescribeCollectionOption(summaryCollectionName))
	if err != nil {
		log.Fatalf("failed to describe %s: %v", summaryCollectionName, err)
	}
	fmt.Printf("collection %s exists\n", summaryCollectionName)
	if coll.Schema == nil {
		fmt.Println("  (schema not available)")
		return
	}
	for _, f := range coll.Schema.Fields {
		if f == nil {
			continue
		}
		if f.Name == "summary_vector" {
			if dim, ok := f.TypeParams["dim"]; ok {
				fmt.Printf("  summary_vector dim=%s\n", dim)
			}
		}
	}
}
