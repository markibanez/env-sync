# env-sync

**Securely sync your `.env` files across machines with military-grade encryption.**

A blazingly fast CLI tool built for developers who manage multiple projects across different machines. Scan once, sync everywhere. Keep your environment variables in perfect sync between your work laptop, home desktop, and MacBook without manual copying.

> **⚠️ IMPORTANT:** This tool handles sensitive secrets. **Review the source code before use** and build from source for production use. See [Build from Source](#build-from-source) below.

[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Cross-Platform](https://img.shields.io/badge/platform-Windows%20%7C%20macOS%20%7C%20Linux-lightgrey)](https://github.com/markibanez/env-sync/releases)

---

## Features

- **Recursive Scanning** - Finds all `.env` files in your projects automatically
- **Military-Grade Encryption** - AES-GCM with Argon2 key derivation
- **Smart Bidirectional Sync** - SHA-256 hash comparison + timestamp-based conflict resolution
- **Multiple Databases** - Turso/LibSQL and PostgreSQL support
- **Dry Run Mode** - Preview changes before applying
- **Cross-Platform** - Works on Windows, macOS (Apple Silicon), and Linux
- **Fast & Lightweight** - Written in Go, single binary, no dependencies
- **Secure by Design** - Files never leave your machine unencrypted

---

## Who Is This For?

**Developers with multiple projects across multiple machines.**

If you have a base directory like `~/Projects` or `D:\Github` with dozens of repos, each with their own `.env` files, and you switch between:
- Work laptop
- Home desktop
- Personal MacBook
- Remote dev server

...then this tool is for you. Scan your entire projects folder once, and keep all environment files in sync across every machine.

**Recommended Setup:** Add `env-sync sync` to a cron job or scheduled task on each machine to automatically sync every hour or on login.

---

## Quick Start

### Installation (Build from Source - Recommended)

**For production use, always build from source after reviewing the code:**

```bash
# Clone and review the repository
git clone https://github.com/markibanez/env-sync.git
cd env-sync

# Review the source code (IMPORTANT - this handles your secrets!)
# Check: main.go, crypto.go, database.go, sync.go

# Install dependencies
go mod download

# Build for your platform
go build -o env-sync

# Move to your PATH
sudo mv env-sync /usr/local/bin/  # macOS/Linux
# or add to PATH on Windows
```

### Installation (Pre-built Binaries)

If you trust the releases, download from [Releases](https://github.com/markibanez/env-sync/releases):

**macOS (Apple Silicon):**
```bash
curl -L https://github.com/markibanez/env-sync/releases/latest/download/env-sync-macos-arm64 -o env-sync
chmod +x env-sync
sudo mv env-sync /usr/local/bin/
```

**Windows (PowerShell):**
```powershell
# Download from releases and add to PATH
```

**Linux:**
```bash
curl -L https://github.com/markibanez/env-sync/releases/latest/download/env-sync-linux -o env-sync
chmod +x env-sync
sudo mv env-sync /usr/local/bin/
```

### Basic Usage

```bash
# 1. Scan for .env files in your projects
env-sync scan ~/Projects

# 2. Sync to cloud database (upload newer, download newer)
env-sync sync \
  --db "libsql://your-db.turso.io?authToken=eyJhbGc..." \
  --password "your-secret-password"

# 3. Preview changes without applying (dry run)
env-sync sync \
  --db "libsql://your-db.turso.io?authToken=eyJhbGc..." \
  --password "your-secret-password" \
  --dry-run
```

---

## Commands

### `scan <path>`
Recursively scans a directory for `.env` files and remembers their locations.

```bash
env-sync scan /path/to/projects
```

**Features:**
- Finds all `.env`, `.env.local`, `.env.production`, etc.
- Skips `node_modules`, `vendor`, and hidden directories
- Stores file paths locally for sync operations

---

### `sync`
Smart bidirectional sync with hash comparison and timestamp-based conflict resolution.

```bash
env-sync sync \
  --db "libsql://db-name.turso.io?authToken=..." \
  --password "encryption-password" \
  --base "/path/to/projects" \
  --dry-run
```

**Flags:**
- `--db` - Database connection string (required)
- `--password` - Encryption password (required)
- `--base` - Base path for relative paths (default: current directory)
- `--dry-run` - Preview changes without applying

**Sync Logic:**
1. **Hash comparison first** (most reliable)
   - If hashes match → Skip (files are identical)
2. **Timestamp comparison** (if hashes differ)
   - Local newer → Upload to database
   - Remote newer → Download from database
   - Same time, different content → Upload local (prefer local changes)

**Example Output:**
```
Syncing 15 .env file(s)...

↑ Uploaded: ./api/.env (local newer)
↓ Downloaded: ./web/.env (remote newer)
= Skipped: ./db/.env (identical)

--------------------------------------------------
Sync Summary:
  ↑ Uploaded (local newer):   1
  ↓ Downloaded (remote newer): 1
  = Skipped (same):           13
--------------------------------------------------
```

---

### `upload`
Force upload all scanned files to the database (overwrites remote).

```bash
env-sync upload \
  --db "postgres://user:pass@localhost:5432/dbname" \
  --password "encryption-password"
```

---

### `download`
Download all files from database to a specified directory.

```bash
env-sync download \
  --db "libsql://db-name.turso.io?authToken=..." \
  --password "encryption-password" \
  --output "./restored-env-files"
```

---

### `list`
List all remembered `.env` files from the last scan.

```bash
env-sync list
```

---

## Database Setup

### Turso/LibSQL (Recommended)

[Turso](https://turso.tech/) is a distributed SQLite database perfect for this use case.

```bash
# Install Turso CLI
brew install tursodatabase/tap/turso  # macOS
# or
curl -sSfL https://get.tur.so/install.sh | bash  # Linux/WSL

# Create database
turso db create env-sync

# Get database URL
turso db show env-sync --url
# Output: libsql://env-sync-yourname.turso.io

# Create auth token
turso db tokens create env-sync
# Output: eyJhbGciOiJFZERTQSIsInR5cCI6IkpXVCJ9...

# Use in env-sync
env-sync sync \
  --db "libsql://env-sync-yourname.turso.io?authToken=eyJhbGc..." \
  --password "my-secret-password"
```

### PostgreSQL

```bash
env-sync sync \
  --db "postgres://user:password@localhost:5432/env_sync" \
  --password "my-secret-password"
```

---

## Security

- **Encryption:** AES-256-GCM (Galois/Counter Mode)
- **Key Derivation:** Argon2id with 64MB memory, 4 threads, 1 iteration
- **Random Salt:** 16 bytes per file
- **Random Nonce:** 12 bytes per encryption
- **Hash Verification:** SHA-256 for content comparison
- **Zero Knowledge:** Database stores only encrypted content, never plaintext

**Database Schema:**
```sql
CREATE TABLE env_files (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  path TEXT NOT NULL,              -- Unix-style relative path
  filename TEXT NOT NULL,           -- Filename (e.g., .env)
  contents TEXT NOT NULL,           -- AES-GCM encrypted + base64
  file_hash TEXT NOT NULL,          -- SHA-256 of plaintext
  file_modified_at DATETIME NOT NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(path, filename)
);
```

---

## Build from Source

```bash
# Clone the repository
git clone https://github.com/markibanez/env-sync.git
cd env-sync

# Install dependencies
go mod download

# Build for your platform
go build -o env-sync

# Or cross-compile for all platforms
GOOS=darwin GOARCH=arm64 go build -o env-sync-macos-arm64
GOOS=windows GOARCH=amd64 go build -o env-sync-windows.exe
GOOS=linux GOARCH=amd64 go build -o env-sync-linux
```

---

## Automated Sync (Recommended)

For seamless multi-machine development, set up automated syncing with cron (macOS/Linux) or Task Scheduler (Windows) to run `env-sync sync` periodically. This ensures your environment files stay in sync across all your machines without manual intervention.

Run it every hour, every 30 minutes, or on system login—whatever fits your workflow.

---

## Use Cases

**Primary Use Case: Multi-Machine Development with Many Projects**

You have `~/Projects` or `D:\Github` with 50+ repos, each with their own `.env` files. You work on:
- MacBook Pro at the coffee shop
- Windows desktop at home
- Linux workstation at the office

Instead of manually copying files or using git (which you shouldn't for secrets), run `env-sync scan ~/Projects` once, then `env-sync sync` on each machine. All your environment files stay in sync automatically.

**Other Use Cases:**

- **Team Onboarding** - Share encrypted environment configs with new team members
- **Backup & Recovery** - Never lose API keys or database credentials again
- **CI/CD Secrets** - Securely pull environment variables in deployment pipelines
- **Freelance/Consulting** - Keep client project configs organized and synced

---

## FAQ

**Q: What happens if I forget my password?**
A: Your files are encrypted with your password. Without it, decryption is impossible. Store your password in a password manager.

**Q: Can I use different passwords for different projects?**
A: Yes! Use different databases or different passwords. The tool doesn't restrict this.

**Q: Does this replace `.env.example` files?**
A: No. Keep `.env.example` in your repos with dummy values. Use `env-sync` for actual secrets.

**Q: What files are synced?**
A: Files matching `.env*` pattern (`.env`, `.env.local`, `.env.production`, etc.)

**Q: Is this safe for production secrets?**
A: The encryption is production-grade, but review your threat model. For high-security needs, consider HashiCorp Vault or AWS Secrets Manager.

---

## License

MIT License - see [LICENSE](LICENSE) file for details.

---

## Contributing

Contributions welcome! Open an issue or submit a PR.

---

## Links

- **GitHub:** [markibanez/env-sync](https://github.com/markibanez/env-sync)
- **Issues:** [Report a bug](https://github.com/markibanez/env-sync/issues)
- **Turso Database:** [turso.tech](https://turso.tech/)

---

**Made with ❤️ by [@markibanez](https://github.com/markibanez)**

*Keep your secrets safe, sync them everywhere.*

### Installation

Download the latest binary for your platform from [Releases](https://github.com/markibanez/env-sync/releases):

**macOS (Apple Silicon):**
```bash
curl -L https://github.com/markibanez/env-sync/releases/latest/download/env-sync-macos-arm64 -o env-sync
chmod +x env-sync
sudo mv env-sync /usr/local/bin/
```

**Windows (PowerShell):**
```powershell
# Download from releases and add to PATH
```

**Linux:**
```bash
curl -L https://github.com/markibanez/env-sync/releases/latest/download/env-sync-linux -o env-sync
chmod +x env-sync
sudo mv env-sync /usr/local/bin/
```

### Basic Usage

```bash
# 1. Scan for .env files in your projects
env-sync scan ~/Projects

# 2. Sync to cloud database (upload newer, download newer)
env-sync sync \
  --db "libsql://your-db.turso.io?authToken=eyJhbGc..." \
  --password "your-secret-password"

# 3. Preview changes without applying (dry run)
env-sync sync \
  --db "libsql://your-db.turso.io?authToken=eyJhbGc..." \
  --password "your-secret-password" \
  --dry-run
```

---

## 📖 Commands

### `scan <path>`
Recursively scans a directory for `.env` files and remembers their locations.

```bash
env-sync scan /path/to/projects
```

**Features:**
- Finds all `.env`, `.env.local`, `.env.production`, etc.
- Skips `node_modules`, `vendor`, and hidden directories
- Stores file paths locally for sync operations

---

### `sync`
Smart bidirectional sync with hash comparison and timestamp-based conflict resolution.

```bash
env-sync sync \
  --db "libsql://db-name.turso.io?authToken=..." \
  --password "encryption-password" \
  --base "/path/to/projects" \
  --dry-run
```

**Flags:**
- `--db` - Database connection string (required)
- `--password` - Encryption password (required)
- `--base` - Base path for relative paths (default: current directory)
- `--dry-run` - Preview changes without applying

**Sync Logic:**
1. **Hash comparison first** (most reliable)
   - If hashes match → ✓ Skip (files are identical)
2. **Timestamp comparison** (if hashes differ)
   - Local newer → ↑ Upload to database
   - Remote newer → ↓ Download from database
   - Same time, different content → ↑ Upload local (prefer local changes)

**Example Output:**
```
Syncing 15 .env file(s)...

↑ Uploaded: ./api/.env (local newer)
↓ Downloaded: ./web/.env (remote newer)
= Skipped: ./db/.env (identical)

--------------------------------------------------
Sync Summary:
  ↑ Uploaded (local newer):   1
  ↓ Downloaded (remote newer): 1
  = Skipped (same):           13
--------------------------------------------------
```

---

### `upload`
Force upload all scanned files to the database (overwrites remote).

```bash
env-sync upload \
  --db "postgres://user:pass@localhost:5432/dbname" \
  --password "encryption-password"
```

---

### `download`
Download all files from database to a specified directory.

```bash
env-sync download \
  --db "libsql://db-name.turso.io?authToken=..." \
  --password "encryption-password" \
  --output "./restored-env-files"
```

---

### `list`
List all remembered `.env` files from the last scan.

```bash
env-sync list
```

---

## 🗄️ Database Setup

### Turso/LibSQL (Recommended)

[Turso](https://turso.tech/) is a distributed SQLite database perfect for this use case.

```bash
# Install Turso CLI
brew install tursodatabase/tap/turso  # macOS
# or
curl -sSfL https://get.tur.so/install.sh | bash  # Linux/WSL

# Create database
turso db create env-sync

# Get database URL
turso db show env-sync --url
# Output: libsql://env-sync-yourname.turso.io

# Create auth token
turso db tokens create env-sync
# Output: eyJhbGciOiJFZERTQSIsInR5cCI6IkpXVCJ9...

# Use in env-sync
env-sync sync \
  --db "libsql://env-sync-yourname.turso.io?authToken=eyJhbGc..." \
  --password "my-secret-password"
```

### PostgreSQL

```bash
env-sync sync \
  --db "postgres://user:password@localhost:5432/env_sync" \
  --password "my-secret-password"
```

---

## 🔐 Security

- **Encryption:** AES-256-GCM (Galois/Counter Mode)
- **Key Derivation:** Argon2id with 64MB memory, 4 threads, 1 iteration
- **Random Salt:** 16 bytes per file
- **Random Nonce:** 12 bytes per encryption
- **Hash Verification:** SHA-256 for content comparison
- **Zero Knowledge:** Database stores only encrypted content, never plaintext

**Database Schema:**
```sql
CREATE TABLE env_files (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  path TEXT NOT NULL,              -- Unix-style relative path
  filename TEXT NOT NULL,           -- Filename (e.g., .env)
  contents TEXT NOT NULL,           -- AES-GCM encrypted + base64
  file_hash TEXT NOT NULL,          -- SHA-256 of plaintext
  file_modified_at DATETIME NOT NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(path, filename)
);
```

---

## 🛠️ Build from Source

```bash
# Clone the repository
git clone https://github.com/markibanez/env-sync.git
cd env-sync

# Install dependencies
go mod download

# Build for your platform
go build -o env-sync

# Or cross-compile for all platforms
GOOS=darwin GOARCH=arm64 go build -o env-sync-macos-arm64
GOOS=windows GOARCH=amd64 go build -o env-sync-windows.exe
GOOS=linux GOARCH=amd64 go build -o env-sync-linux
```

---

## 🎯 Use Cases

- **Multi-Machine Development** - Sync `.env` files between your work laptop, home desktop, and MacBook
- **Team Onboarding** - Share encrypted environment configs with new team members
- **Backup & Recovery** - Never lose API keys or database credentials again
- **CI/CD Secrets** - Securely pull environment variables in deployment pipelines
- **Freelance/Consulting** - Keep client project configs organized and synced

---

## 🤔 FAQ

**Q: What happens if I forget my password?**
A: Your files are encrypted with your password. Without it, decryption is impossible. Store your password in a password manager.

**Q: Can I use different passwords for different projects?**
A: Yes! Use different databases or different passwords. The tool doesn't restrict this.

**Q: Does this replace `.env.example` files?**
A: No. Keep `.env.example` in your repos with dummy values. Use `env-sync` for actual secrets.

**Q: What files are synced?**
A: Files matching `.env*` pattern (`.env`, `.env.local`, `.env.production`, etc.)

**Q: Is this safe for production secrets?**
A: The encryption is production-grade, but review your threat model. For high-security needs, consider HashiCorp Vault or AWS Secrets Manager.

---

## 📄 License

MIT License - see [LICENSE](LICENSE) file for details.

---

## 🙌 Contributing

Contributions welcome! Open an issue or submit a PR.

---

## 🔗 Links

- **GitHub:** [markibanez/env-sync](https://github.com/markibanez/env-sync)
- **Issues:** [Report a bug](https://github.com/markibanez/env-sync/issues)
- **Turso Database:** [turso.tech](https://turso.tech/)

---

**Made with ❤️ by [@markibanez](https://github.com/markibanez)**

*Keep your secrets safe, sync them everywhere.*

