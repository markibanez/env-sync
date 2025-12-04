package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "scan":
		if len(os.Args) < 3 {
			fmt.Println("Error: scan command requires a path argument")
			fmt.Println("Usage: env-sync scan <path>")
			os.Exit(1)
		}
		path := os.Args[2]
		if err := scanForEnvFiles(path); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	case "upload":
		uploadCmd := flag.NewFlagSet("upload", flag.ExitOnError)
		dbConnStr := uploadCmd.String("db", "", "Database connection string (required)")
		password := uploadCmd.String("password", "", "Encryption password (required)")
		basePath := uploadCmd.String("base", "", "Base path for relative paths (default: current directory)")

		uploadCmd.Parse(os.Args[2:])

		if *dbConnStr == "" || *password == "" {
			fmt.Println("Error: --db and --password are required")
			fmt.Println("Usage: env-sync upload --db <connection-string> --password <encryption-password> [--base <base-path>]")
			os.Exit(1)
		}

		if *basePath == "" {
			cwd, err := os.Getwd()
			if err != nil {
				fmt.Printf("Error: failed to get current directory: %v\n", err)
				os.Exit(1)
			}
			*basePath = cwd
		}

		if err := uploadEnvFiles(*dbConnStr, *password, *basePath); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	case "sync":
		syncCmd := flag.NewFlagSet("sync", flag.ExitOnError)
		dbConnStr := syncCmd.String("db", "", "Database connection string (required)")
		password := syncCmd.String("password", "", "Encryption password (required)")
		basePath := syncCmd.String("base", "", "Base path for relative paths (default: current directory)")
		dryRun := syncCmd.Bool("dry-run", false, "Show what would be synced without making changes")
		numWorkers := syncCmd.Int("workers", 10, "Number of parallel workers (default: 10)")

		syncCmd.Parse(os.Args[2:])

		if *dbConnStr == "" || *password == "" {
			fmt.Println("Error: --db and --password are required")
			fmt.Println("Usage: env-sync sync --db <connection-string> --password <encryption-password> [--base <base-path>] [--dry-run]")
			os.Exit(1)
		}

		if *basePath == "" {
			cwd, err := os.Getwd()
			if err != nil {
				fmt.Printf("Error: failed to get current directory: %v\n", err)
				os.Exit(1)
			}
			*basePath = cwd
		}

		if err := syncEnvFiles(*dbConnStr, *password, *basePath, *dryRun, *numWorkers); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	case "daemon":
		daemonCmd := flag.NewFlagSet("daemon", flag.ExitOnError)
		dbConnStr := daemonCmd.String("db", "", "Database connection string (required)")
		password := daemonCmd.String("password", "", "Encryption password (required)")
		basePath := daemonCmd.String("base", "", "Base path for relative paths (default: current directory)")
		interval := daemonCmd.Duration("interval", 1*time.Hour, "Sync interval (default: 1h)")
		numWorkers := daemonCmd.Int("workers", 10, "Number of parallel workers (default: 10)")

		daemonCmd.Parse(os.Args[2:])

		if *dbConnStr == "" || *password == "" {
			fmt.Println("Error: --db and --password are required")
			fmt.Println("Usage: env-sync daemon --db <connection-string> --password <encryption-password> [--base <base-path>] [--interval <duration>]")
			os.Exit(1)
		}

		if *basePath == "" {
			cwd, err := os.Getwd()
			if err != nil {
				fmt.Printf("Error: failed to get current directory: %v\n", err)
				os.Exit(1)
			}
			*basePath = cwd
		}

		runDaemon(*dbConnStr, *password, *basePath, *interval, *numWorkers)
	case "download":
		downloadCmd := flag.NewFlagSet("download", flag.ExitOnError)
		dbConnStr := downloadCmd.String("db", "", "Database connection string (required)")
		password := downloadCmd.String("password", "", "Decryption password (required)")
		outputPath := downloadCmd.String("output", "", "Output directory (default: current directory)")

		downloadCmd.Parse(os.Args[2:])

		if *dbConnStr == "" || *password == "" {
			fmt.Println("Error: --db and --password are required")
			fmt.Println("Usage: env-sync download --db <connection-string> --password <decryption-password> [--output <directory>]")
			os.Exit(1)
		}

		if *outputPath == "" {
			cwd, err := os.Getwd()
			if err != nil {
				fmt.Printf("Error: failed to get current directory: %v\n", err)
				os.Exit(1)
			}
			*outputPath = cwd
		}

		if err := downloadEnvFiles(*dbConnStr, *password, *outputPath); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	case "list":
		if err := listEnvFiles(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	case "version":
		fmt.Println("env-sync v0.2.0")
	case "help":
		printUsage()
	default:
		fmt.Printf("Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("env-sync - Environment synchronization tool")
	fmt.Println("\nUsage:")
	fmt.Println("  env-sync <command> [options]")
	fmt.Println("\nCommands:")
	fmt.Println("  scan <path>              Recursively scan for .env files in the given path")
	fmt.Println("  sync                     Smart bidirectional sync based on file timestamps")
	fmt.Println("    --db <conn-string>     Database connection string")
	fmt.Println("    --password <pwd>       Encryption password")
	fmt.Println("    --base <path>          Base path for relative paths (default: current dir)")
	fmt.Println("    --dry-run              Show what would be synced without making changes")
	fmt.Println("    --workers <n>          Number of parallel workers (default: 10)")
	fmt.Println("  daemon                   Run as a background daemon with periodic sync")
	fmt.Println("    --db <conn-string>     Database connection string")
	fmt.Println("    --password <pwd>       Encryption password")
	fmt.Println("    --base <path>          Base path for relative paths (default: current dir)")
	fmt.Println("    --interval <duration>  Sync interval (default: 1h, e.g., 30m, 2h)")
	fmt.Println("    --workers <n>          Number of parallel workers (default: 10)")
	fmt.Println("  upload                   Upload scanned .env files to database (encrypted)")
	fmt.Println("    --db <conn-string>     Database connection string")
	fmt.Println("    --password <pwd>       Encryption password")
	fmt.Println("    --base <path>          Base path for relative paths (default: current dir)")
	fmt.Println("  download                 Download .env files from database (decrypted)")
	fmt.Println("    --db <conn-string>     Database connection string")
	fmt.Println("    --password <pwd>       Decryption password")
	fmt.Println("    --output <path>        Output directory (default: current dir)")
	fmt.Println("  list                     List all remembered .env files")
	fmt.Println("  version                  Show version information")
	fmt.Println("  help                     Show this help message")
	fmt.Println("\nSupported Databases:")
	fmt.Println("  - Turso/LibSQL: libsql://[host]?authToken=[token]")
	fmt.Println("  - PostgreSQL:   postgres://user:pass@host:port/dbname")
	fmt.Println("\nExamples:")
	fmt.Println(`  # Scan for .env files`)
	fmt.Println(`  env-sync scan D:\Github`)
	fmt.Println()
	fmt.Println(`  # Smart sync (recommended) - compares timestamps and syncs both ways`)
	fmt.Println(`  env-sync sync --db "libsql://mydb-user.turso.io?authToken=xxxxx" --password "mypass"`)
	fmt.Println()
	fmt.Println(`  # Preview sync changes without applying them`)
	fmt.Println(`  env-sync sync --db "libsql://mydb-user.turso.io?authToken=xxxxx" --password "mypass" --dry-run`)
	fmt.Println()
	fmt.Println(`  # Force upload to Turso`)
	fmt.Println(`  env-sync upload --db "libsql://mydb-user.turso.io?authToken=xxxxx" --password "mypass"`)
	fmt.Println()
	fmt.Println(`  # Download and restore`)
	fmt.Println(`  env-sync download --db "libsql://mydb-user.turso.io?authToken=xxxxx" --password "mypass" --output ./restore`)
	fmt.Println()
	fmt.Println(`  # Run as daemon (syncs every hour)`)
	fmt.Println(`  env-sync daemon --db "libsql://mydb-user.turso.io?authToken=xxxxx" --password "mypass" --interval 1h`)
}

func runDaemon(dbConnStr, password, basePath string, interval time.Duration, numWorkers int) {
	fmt.Printf("env-sync daemon starting...\n")
	fmt.Printf("  Database: %s...\n", dbConnStr[:min(50, len(dbConnStr))])
	fmt.Printf("  Base path: %s\n", basePath)
	fmt.Printf("  Interval: %v\n", interval)
	fmt.Printf("  Workers: %d\n", numWorkers)
	fmt.Println()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Run initial sync
	fmt.Printf("[%s] Running initial sync...\n", time.Now().Format("2006-01-02 15:04:05"))
	if err := syncEnvFiles(dbConnStr, password, basePath, false, numWorkers); err != nil {
		fmt.Printf("Error during sync: %v\n", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	fmt.Printf("\n[%s] Daemon running. Next sync in %v. Press Ctrl+C to stop.\n", time.Now().Format("2006-01-02 15:04:05"), interval)

	for {
		select {
		case <-ticker.C:
			fmt.Printf("\n[%s] Running scheduled sync...\n", time.Now().Format("2006-01-02 15:04:05"))
			if err := syncEnvFiles(dbConnStr, password, basePath, false, numWorkers); err != nil {
				fmt.Printf("Error during sync: %v\n", err)
			}
			fmt.Printf("[%s] Next sync in %v\n", time.Now().Format("2006-01-02 15:04:05"), interval)
		case sig := <-sigChan:
			fmt.Printf("\n[%s] Received %v, shutting down...\n", time.Now().Format("2006-01-02 15:04:05"), sig)
			return
		}
	}
}
