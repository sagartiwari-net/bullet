package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"authchecker/internal/storage"
	"authchecker/internal/web"
)

func main() {
	addr := flag.String("addr", ":8080", "web dashboard address")
	flareURL := flag.String("flare", "http://localhost:8191", "flaresolverr url")
	flag.Parse()

	root, _ := os.Getwd()
	configsDir := filepath.Join(root, "configs")
	wordlistsDir := filepath.Join(root, "wordlists")
	resultsDir := filepath.Join(root, "results")
	dbPath := filepath.Join(resultsDir, "checker.db")

	for _, dir := range []string{configsDir, wordlistsDir, resultsDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "mkdir %s: %v\n", dir, err)
			os.Exit(1)
		}
	}

	db, err := storage.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "db open: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	srv := web.New(web.Options{
		Addr:         *addr,
		ConfigsDir:   configsDir,
		WordlistsDir: wordlistsDir,
		ResultsDir:   resultsDir,
		DB:           db,
		FlareURL:     *flareURL,
	})

	fmt.Println("========================================")
	fmt.Println("  Auth Checker v0.1")
	fmt.Println("========================================")
	fmt.Printf("  Configs:    %s\n", configsDir)
	fmt.Printf("  Wordlists:  %s\n", wordlistsDir)
	fmt.Printf("  Results:    %s\n", resultsDir)
	fmt.Println("----------------------------------------")
	fmt.Println("  Step 1: Run demo server")
	fmt.Println("    go run ./demo-server")
	fmt.Println("  Step 2: Open dashboard")
	fmt.Printf("    http://localhost%s\n", *addr)
	fmt.Println("========================================")

	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "server: %v\n", err)
		os.Exit(1)
	}
}
