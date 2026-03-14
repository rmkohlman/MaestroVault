// MaestroVault CLI — a macOS-first developer-focused secrets management tool.
//
// Usage:
//
//	mav init                  Initialize a new vault
//	mav set <name>            Store a secret
//	mav get <name>            Retrieve a secret
//	mav list                  List all secrets
//	mav delete <name>         Delete a secret
//	mav edit <name>           Edit an existing secret
//	mav copy <name>           Copy a secret to the clipboard
//	mav search <query>        Search secrets by name or metadata
//	mav generate              Generate a random password
//	mav env                   Export secrets as environment variables
//	mav exec -- <cmd>         Run a command with secrets injected as env vars
//	mav export                Export vault to file
//	mav import <file>         Import secrets from file
//	mav destroy               Destroy the vault completely
//	mav tui                   Launch interactive TUI
//	mav serve                 Start the REST API server
//	mav token create          Create an API token
//	mav token list            List API tokens
//	mav token revoke          Revoke an API token
//	mav touchid enable        Enable TouchID authentication
//	mav touchid disable       Disable TouchID authentication
//	mav touchid status        Show TouchID status
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rmkohlman/MaestroVault/internal/api"
	"github.com/rmkohlman/MaestroVault/internal/clipboard"
	"github.com/rmkohlman/MaestroVault/internal/crypto"
	"github.com/rmkohlman/MaestroVault/internal/touchid"
	"github.com/rmkohlman/MaestroVault/internal/tui"
	"github.com/rmkohlman/MaestroVault/internal/vault"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// Build info — set via ldflags at build time:
//
//	go build -ldflags "-X main.version=v1.0.0 -X main.commit=$(git rev-parse --short HEAD) -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// Global flags.
var (
	outputFormat string
	noColor      bool
)

func main() {
	rootCmd := buildRootCmd()
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func buildRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "mav",
		Short: "MaestroVault — secure local secrets management for developers",
		Long: `MaestroVault is a lightweight, macOS-first secrets manager for developers.

Secrets are encrypted with AES-256-GCM using envelope encryption.
The master key is stored securely in the macOS Keychain.`,
		SilenceErrors: true,
		SilenceUsage:  true,
		CompletionOptions: cobra.CompletionOptions{
			HiddenDefaultCmd: true,
		},
	}

	root.PersistentFlags().StringVarP(&outputFormat, "format", "o", "", "Output format: json, table (default: auto-detect)")
	root.PersistentFlags().BoolVar(&noColor, "no-color", false, "Disable colored output")

	root.AddCommand(
		newVersionCmd(),
		newInitCmd(),
		newSetCmd(),
		newGetCmd(),
		newListCmd(),
		newDeleteCmd(),
		newEditCmd(),
		newCopyCmd(),
		newSearchCmd(),
		newGenerateCmd(),
		newEnvCmd(),
		newExecCmd(),
		newExportCmd(),
		newImportCmd(),
		newDestroyCmd(),
		newTUICmd(),
		newServeCmd(),
		newTokenCmd(),
		newTouchIDCmd(),
	)

	return root
}

// ── withVault helper ──────────────────────────────────────────

// withVault handles config loading, TouchID authentication, vault opening,
// and cleanup. All commands that need an open vault should use this instead
// of calling vault.Open directly.
func withVault(fn func(context.Context, vault.Vault) error) error {
	ctx := context.Background()

	// Load config and check TouchID.
	cfg, err := vault.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if cfg.TouchID {
		auth := touchid.New()
		if err := auth.Authenticate("MaestroVault wants to access your secrets"); err != nil {
			return fmt.Errorf("authentication required: %w", err)
		}
	}

	// Open vault (no TouchID inside — we handled it above).
	v, err := vault.Open(ctx)
	if err != nil {
		return err
	}
	defer v.Close()

	return fn(ctx, v)
}

// --- Secret name completion helper ---

func secretNameCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	ctx := context.Background()
	v, err := vault.Open(ctx)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	defer v.Close()

	names, err := v.Names(ctx)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Deduplicate by name (env handled by --env flag).
	seen := make(map[string]bool)
	var result []string
	for _, n := range names {
		if !seen[n.Name] {
			seen[n.Name] = true
			result = append(result, n.Name)
		}
	}
	return result, cobra.ShellCompDirectiveNoFileComp
}

// --- Commands ---

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version and build information",
		Run: func(cmd *cobra.Command, args []string) {
			format := resolveFormat(outputFormat)
			if format == "json" {
				outputJSON(map[string]string{
					"version": version,
					"commit":  commit,
					"date":    date,
					"go":      runtime.Version(),
					"os":      runtime.GOOS,
					"arch":    runtime.GOARCH,
				})
				return
			}
			fmt.Printf("MaestroVault %s\n", colorize(version, ansiCyan+ansiBold))
			fmt.Printf("  Commit:  %s\n", commit)
			fmt.Printf("  Built:   %s\n", date)
			fmt.Printf("  Go:      %s\n", runtime.Version())
			fmt.Printf("  OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
		},
	}
}

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize a new vault",
		Long:  "Creates the vault directory, generates a master key, and stores it securely in the macOS Keychain.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			if err := vault.Init(ctx); err != nil {
				printError(err.Error(), "If the vault already exists, use 'mav destroy' first to reset it.")
				return err
			}
			printSuccess("Vault initialized successfully.")
			fmt.Printf("  %s  %s\n", colorize("Database:", ansiDim), vault.DBPath())
			fmt.Printf("  %s  stored in macOS Keychain\n", colorize("Master key:", ansiDim))
			return nil
		},
	}
}

func newSetCmd() *cobra.Command {
	var (
		valueFlag  string
		metadata   []string
		generatePw bool
		genLength  int
		genSymbols bool
	)

	cmd := &cobra.Command{
		Use:   "set <name>",
		Short: "Store a secret",
		Long: `Encrypts and stores a secret. Value can be provided via:
  --value flag, --generate flag (auto-generate password), or stdin.`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: secretNameCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			var value string
			switch {
			case generatePw:
				opts := crypto.DefaultGenerateOpts()
				opts.Length = genLength
				opts.Symbols = genSymbols
				pw, err := crypto.GeneratePassword(opts)
				if err != nil {
					return err
				}
				value = pw
			case valueFlag != "":
				value = valueFlag
			default:
				if term.IsTerminal(int(os.Stdin.Fd())) {
					fmt.Fprint(os.Stderr, "Enter secret value: ")
					raw, err := term.ReadPassword(int(os.Stdin.Fd()))
					fmt.Fprintln(os.Stderr) // newline after masked input
					if err != nil {
						return fmt.Errorf("reading input: %w", err)
					}
					value = string(raw)
				} else {
					// Piped input — read as plaintext.
					scanner := bufio.NewScanner(os.Stdin)
					if scanner.Scan() {
						value = scanner.Text()
					}
					if err := scanner.Err(); err != nil {
						return fmt.Errorf("reading input: %w", err)
					}
				}
			}

			if value == "" {
				printError("Secret value cannot be empty.", "Use --value, --generate, or pipe via stdin.")
				return fmt.Errorf("empty value")
			}

			metadataMap, err := parseMetadata(metadata)
			if err != nil {
				return err
			}

			return withVault(func(ctx context.Context, v vault.Vault) error {
				env, _ := cmd.Flags().GetString("env")
				if err := v.Set(ctx, name, env, value, metadataMap); err != nil {
					printError(err.Error(), "Run 'mav init' to create a new vault.")
					return err
				}

				printSuccess(fmt.Sprintf("Secret %q stored.", name))
				if generatePw {
					fmt.Printf("  %s  %s\n", colorize("Generated:", ansiDim), colorize(value, ansiGreen))
				}
				return nil
			})
		},
	}

	cmd.Flags().StringVarP(&valueFlag, "value", "v", "", "Secret value (reads from stdin if omitted)")
	cmd.Flags().StringSliceVarP(&metadata, "metadata", "m", nil, "Metadata as key=value (repeatable)")
	cmd.Flags().StringP("env", "e", "", "Environment (e.g. dev, staging, prod)")
	cmd.Flags().BoolVarP(&generatePw, "generate", "g", false, "Auto-generate a random password as the value")
	cmd.Flags().IntVar(&genLength, "length", 32, "Generated password length (with --generate)")
	cmd.Flags().BoolVar(&genSymbols, "symbols", true, "Include symbols in generated password (with --generate)")

	return cmd
}

func newGetCmd() *cobra.Command {
	var (
		quiet bool
		clip  bool
	)

	cmd := &cobra.Command{
		Use:               "get <name>",
		Short:             "Retrieve a secret",
		Long:              "Decrypts and displays a stored secret.",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: secretNameCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withVault(func(ctx context.Context, v vault.Vault) error {
				env, _ := cmd.Flags().GetString("env")
				entry, err := v.Get(ctx, args[0], env)
				if err != nil {
					printError(err.Error(), "Use 'mav list' to see available secrets.")
					return err
				}

				// Copy to clipboard if requested.
				if clip {
					cancel, err := clipboard.CopyWithClear(entry.Value, 45*time.Second)
					if err != nil {
						return fmt.Errorf("clipboard: %w", err)
					}
					_ = cancel
					printSuccess(fmt.Sprintf("Copied %q to clipboard. Auto-clears in 45s.", entry.Name))
					return nil
				}

				// Quiet mode: just print the value (for piping).
				if quiet {
					fmt.Print(entry.Value)
					return nil
				}

				format := resolveFormat(outputFormat)
				if format == "json" {
					return outputJSON(entry)
				}

				outputKeyValue("Name", entry.Name)
				if entry.Environment != "" {
					outputKeyValue("Environment", entry.Environment)
				}
				outputKeyValue("Value", colorize(entry.Value, ansiGreen))
				if len(entry.Metadata) > 0 {
					outputKeyValue("Metadata", formatMetadata(entry.Metadata))
				}
				outputKeyValue("Created", entry.CreatedAt)
				outputKeyValue("Updated", entry.UpdatedAt)
				return nil
			})
		},
	}

	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Print only the value (for piping)")
	cmd.Flags().BoolVarP(&clip, "clip", "c", false, "Copy value to clipboard (auto-clears in 45s)")
	cmd.Flags().StringP("env", "e", "", "Environment (e.g. dev, staging, prod)")
	return cmd
}

func newListCmd() *cobra.Command {
	var (
		filter        string
		metadataKey   string
		metadataValue string
	)

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List all secrets",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return withVault(func(ctx context.Context, v vault.Vault) error {
				env, _ := cmd.Flags().GetString("env")

				var entries []vault.SecretEntry
				var err error

				switch {
				case filter != "":
					entries, err = v.Search(ctx, filter)
				case metadataKey != "":
					entries, err = v.ListByMetadata(ctx, metadataKey, metadataValue)
				default:
					entries, err = v.List(ctx, env)
				}
				if err != nil {
					return err
				}

				if len(entries) == 0 {
					fmt.Println(colorize("No secrets stored.", ansiDim))
					fmt.Printf("  %s mav set <name> --value <value>\n", colorize("Hint:", ansiYellow))
					return nil
				}

				format := resolveFormat(outputFormat)
				if format == "json" {
					return outputJSON(entries)
				}

				headers := []string{"NAME", "ENVIRONMENT", "METADATA", "CREATED", "UPDATED"}
				rows := make([][]string, len(entries))
				for i, e := range entries {
					rows[i] = []string{
						colorize(e.Name, ansiBold),
						e.Environment,
						formatMetadata(e.Metadata),
						colorize(e.CreatedAt, ansiDim),
						colorize(e.UpdatedAt, ansiDim),
					}
				}
				outputTable(headers, rows)

				count := colorize(fmt.Sprintf("%d secret(s)", len(entries)), ansiDim)
				fmt.Printf("\n%s\n", count)
				return nil
			})
		},
	}

	cmd.Flags().StringVarP(&filter, "filter", "f", "", "Filter secrets by name or metadata content")
	cmd.Flags().StringVar(&metadataKey, "metadata-key", "", "Filter by metadata key")
	cmd.Flags().StringVar(&metadataValue, "metadata-value", "", "Filter by metadata value (used with --metadata-key)")
	cmd.Flags().StringP("env", "e", "", "Environment (e.g. dev, staging, prod)")
	return cmd
}

func newDeleteCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:               "delete <name>",
		Short:             "Delete a secret",
		Aliases:           []string{"rm"},
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: secretNameCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			if !force {
				fmt.Fprintf(os.Stderr, "Delete secret %q? [y/N]: ", name)
				scanner := bufio.NewScanner(os.Stdin)
				if scanner.Scan() {
					answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
					if answer != "y" && answer != "yes" {
						fmt.Println("Aborted.")
						return nil
					}
				}
			}

			return withVault(func(ctx context.Context, v vault.Vault) error {
				env, _ := cmd.Flags().GetString("env")
				if err := v.Delete(ctx, name, env); err != nil {
					printError(err.Error(), "Use 'mav list' to see available secrets.")
					return err
				}

				printSuccess(fmt.Sprintf("Secret %q deleted.", name))
				return nil
			})
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")
	cmd.Flags().StringP("env", "e", "", "Environment (e.g. dev, staging, prod)")
	return cmd
}

func newEditCmd() *cobra.Command {
	var (
		valueFlag string
		metadata  []string
	)

	cmd := &cobra.Command{
		Use:               "edit <name>",
		Short:             "Edit an existing secret",
		Long:              "Update the value and/or metadata of an existing secret. Unspecified fields are preserved.",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: secretNameCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			return withVault(func(ctx context.Context, v vault.Vault) error {
				env, _ := cmd.Flags().GetString("env")

				var newValue *string
				if cmd.Flags().Changed("value") {
					val, _ := cmd.Flags().GetString("value")
					newValue = &val
				}

				var newMetadata map[string]any
				if len(metadata) > 0 {
					var err error
					newMetadata, err = parseMetadata(metadata)
					if err != nil {
						return err
					}
				}

				if err := v.Edit(ctx, name, env, newValue, newMetadata); err != nil {
					printError(err.Error(), "Use 'mav list' to see available secrets.")
					return err
				}

				printSuccess(fmt.Sprintf("Secret %q updated.", name))
				return nil
			})
		},
	}

	cmd.Flags().StringVarP(&valueFlag, "value", "v", "", "New secret value (keeps existing if omitted)")
	cmd.Flags().StringSliceVarP(&metadata, "metadata", "m", nil, "New metadata as key=value (replaces all metadata)")
	cmd.Flags().StringP("env", "e", "", "Environment (e.g. dev, staging, prod)")
	return cmd
}

func newCopyCmd() *cobra.Command {
	var clearAfter int

	cmd := &cobra.Command{
		Use:               "copy <name>",
		Short:             "Copy a secret value to the clipboard",
		Aliases:           []string{"cp"},
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: secretNameCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withVault(func(ctx context.Context, v vault.Vault) error {
				env, _ := cmd.Flags().GetString("env")
				entry, err := v.Get(ctx, args[0], env)
				if err != nil {
					printError(err.Error(), "Use 'mav list' to see available secrets.")
					return err
				}

				dur := time.Duration(clearAfter) * time.Second
				_, err = clipboard.CopyWithClear(entry.Value, dur)
				if err != nil {
					return fmt.Errorf("clipboard: %w", err)
				}

				if clearAfter > 0 {
					printSuccess(fmt.Sprintf("Copied %q to clipboard. Auto-clears in %ds.", entry.Name, clearAfter))
				} else {
					printSuccess(fmt.Sprintf("Copied %q to clipboard.", entry.Name))
				}
				return nil
			})
		},
	}

	cmd.Flags().IntVar(&clearAfter, "clear", 45, "Seconds before clipboard is auto-cleared (0 to disable)")
	cmd.Flags().StringP("env", "e", "", "Environment (e.g. dev, staging, prod)")
	return cmd
}

func newSearchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search secrets by name or metadata",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withVault(func(ctx context.Context, v vault.Vault) error {
				entries, err := v.Search(ctx, args[0])
				if err != nil {
					return err
				}

				if len(entries) == 0 {
					fmt.Printf("No secrets matching %q.\n", args[0])
					return nil
				}

				format := resolveFormat(outputFormat)
				if format == "json" {
					return outputJSON(entries)
				}

				headers := []string{"NAME", "ENVIRONMENT", "METADATA", "CREATED", "UPDATED"}
				rows := make([][]string, len(entries))
				for i, e := range entries {
					rows[i] = []string{
						colorize(e.Name, ansiBold),
						e.Environment,
						formatMetadata(e.Metadata),
						colorize(e.CreatedAt, ansiDim),
						colorize(e.UpdatedAt, ansiDim),
					}
				}
				outputTable(headers, rows)

				count := colorize(fmt.Sprintf("%d result(s)", len(entries)), ansiDim)
				fmt.Printf("\n%s\n", count)
				return nil
			})
		},
	}

	cmd.Flags().StringP("env", "e", "", "Environment (e.g. dev, staging, prod)")
	return cmd
}

func newGenerateCmd() *cobra.Command {
	var (
		length     int
		uppercase  bool
		lowercase  bool
		digits     bool
		symbols    bool
		name       string
		metadata   []string
		clip       bool
		passphrase bool
		words      int
		delimiter  string
	)

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate a random password or passphrase",
		Long: `Generate a cryptographically random password or passphrase.
Optionally store it directly as a secret with --name.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var result string

			if passphrase {
				pw, err := crypto.GeneratePassphrase(words, delimiter)
				if err != nil {
					return err
				}
				result = pw
			} else {
				opts := crypto.GenerateOpts{
					Length:    length,
					Uppercase: uppercase,
					Lowercase: lowercase,
					Digits:    digits,
					Symbols:   symbols,
				}
				pw, err := crypto.GeneratePassword(opts)
				if err != nil {
					return err
				}
				result = pw
			}

			// Store if name provided.
			if name != "" {
				storeErr := withVault(func(ctx context.Context, v vault.Vault) error {
					env, _ := cmd.Flags().GetString("env")
					metadataMap, err := parseMetadata(metadata)
					if err != nil {
						return err
					}
					if err := v.Set(ctx, name, env, result, metadataMap); err != nil {
						return err
					}
					printSuccess(fmt.Sprintf("Generated and stored as %q.", name))
					return nil
				})
				if storeErr != nil {
					return storeErr
				}
			}

			// Copy to clipboard if requested.
			if clip {
				if _, err := clipboard.CopyWithClear(result, 45*time.Second); err != nil {
					return err
				}
				printSuccess("Copied to clipboard. Auto-clears in 45s.")
			}

			format := resolveFormat(outputFormat)
			if format == "json" {
				return outputJSON(map[string]string{"password": result})
			}
			fmt.Println(colorize(result, ansiGreen+ansiBold))
			return nil
		},
	}

	cmd.Flags().IntVar(&length, "length", 32, "Password length")
	cmd.Flags().BoolVar(&uppercase, "uppercase", true, "Include uppercase letters")
	cmd.Flags().BoolVar(&lowercase, "lowercase", true, "Include lowercase letters")
	cmd.Flags().BoolVar(&digits, "digits", true, "Include digits")
	cmd.Flags().BoolVar(&symbols, "symbols", true, "Include symbols")
	cmd.Flags().StringVar(&name, "name", "", "Store the generated password as a secret with this name")
	cmd.Flags().StringSliceVarP(&metadata, "metadata", "m", nil, "Metadata for stored secret (with --name)")
	cmd.Flags().BoolVarP(&clip, "clip", "c", false, "Copy to clipboard")
	cmd.Flags().BoolVar(&passphrase, "passphrase", false, "Generate a passphrase instead of a password")
	cmd.Flags().IntVar(&words, "words", 5, "Number of words in passphrase (with --passphrase)")
	cmd.Flags().StringVar(&delimiter, "delimiter", "-", "Word delimiter for passphrase (with --passphrase)")
	cmd.Flags().StringP("env", "e", "", "Environment (e.g. dev, staging, prod)")

	return cmd
}

func newEnvCmd() *cobra.Command {
	var (
		prefix string
		filter string
	)

	cmd := &cobra.Command{
		Use:   "env",
		Short: "Export secrets as environment variables",
		Long: `Outputs secrets as export statements for shell evaluation.

Usage:
  eval $(mav env)
  eval $(mav env --prefix APP_)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withVault(func(ctx context.Context, v vault.Vault) error {
				entries, err := v.Export(ctx)
				if err != nil {
					return err
				}

				for _, e := range entries {
					if filter != "" && !strings.Contains(strings.ToLower(e.Name), strings.ToLower(filter)) {
						continue
					}
					envName := prefix + toEnvName(e.Name)
					// Shell-escape the value by wrapping in single quotes and escaping internal single quotes.
					escaped := strings.ReplaceAll(e.Value, "'", "'\"'\"'")
					fmt.Printf("export %s='%s'\n", envName, escaped)
				}
				return nil
			})
		},
	}

	cmd.Flags().StringVar(&prefix, "prefix", "", "Prefix for environment variable names")
	cmd.Flags().StringVarP(&filter, "filter", "f", "", "Filter secrets by name")
	return cmd
}

func newExecCmd() *cobra.Command {
	var (
		prefix string
		filter string
	)

	cmd := &cobra.Command{
		Use:   "exec -- <command> [args...]",
		Short: "Run a command with secrets injected as environment variables",
		Long: `Executes a command with all vault secrets available as environment variables.

Example:
  mav exec -- env
  mav exec --prefix DB_ -- psql`,
		DisableFlagParsing: false,
		Args:               cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withVault(func(ctx context.Context, v vault.Vault) error {
				entries, err := v.Export(ctx)
				if err != nil {
					return err
				}

				environ := os.Environ()
				for _, e := range entries {
					if filter != "" && !strings.Contains(strings.ToLower(e.Name), strings.ToLower(filter)) {
						continue
					}
					envName := prefix + toEnvName(e.Name)
					environ = append(environ, fmt.Sprintf("%s=%s", envName, e.Value))
				}

				binary, err := exec.LookPath(args[0])
				if err != nil {
					return fmt.Errorf("command not found: %s", args[0])
				}

				return syscall.Exec(binary, args, environ)
			})
		},
	}

	cmd.Flags().StringVar(&prefix, "prefix", "", "Prefix for environment variable names")
	cmd.Flags().StringVarP(&filter, "filter", "f", "", "Filter secrets by name")
	return cmd
}

func newExportCmd() *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export vault secrets to a file",
		Long: `Export all secrets to stdout in JSON or .env format.

Examples:
  mav export > backup.json
  mav export --format env > .env`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withVault(func(ctx context.Context, v vault.Vault) error {
				entries, err := v.Export(ctx)
				if err != nil {
					return err
				}

				switch strings.ToLower(format) {
				case "env", "dotenv":
					for _, e := range entries {
						// .env format: KEY=VALUE (unquoted for simple values, quoted for complex)
						escaped := strings.ReplaceAll(e.Value, "\"", "\\\"")
						fmt.Printf("%s=\"%s\"\n", toEnvName(e.Name), escaped)
					}
				default:
					data, err := json.MarshalIndent(entries, "", "  ")
					if err != nil {
						return err
					}
					fmt.Println(string(data))
				}

				printWarning(fmt.Sprintf("Exported %d secret(s) in PLAINTEXT. Handle with care.", len(entries)))
				return nil
			})
		},
	}

	cmd.Flags().StringVar(&format, "format", "json", "Export format: json, env")
	cmd.Flags().StringP("env", "e", "", "Environment (e.g. dev, staging, prod)")
	return cmd
}

func newImportCmd() *cobra.Command {
	var (
		format string
		force  bool
	)

	cmd := &cobra.Command{
		Use:   "import <file>",
		Short: "Import secrets from a file",
		Long: `Import secrets from a JSON or .env file.

Examples:
  mav import backup.json
  mav import --format env .env`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]
			data, err := os.ReadFile(filePath)
			if err != nil {
				printError(fmt.Sprintf("Cannot read file: %s", err), "Check the file path exists and is readable.")
				return err
			}

			return withVault(func(ctx context.Context, v vault.Vault) error {
				var imported int

				switch strings.ToLower(format) {
				case "env", "dotenv":
					entries := parseEnvFile(string(data))
					if !force && len(entries) > 0 {
						fmt.Fprintf(os.Stderr, "Import %d secret(s) from %s? [y/N]: ", len(entries), filePath)
						scanner := bufio.NewScanner(os.Stdin)
						if scanner.Scan() {
							answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
							if answer != "y" && answer != "yes" {
								fmt.Println("Aborted.")
								return nil
							}
						}
					}
					var err error
					imported, err = v.Import(ctx, entries)
					if err != nil {
						return err
					}
				default:
					if !force {
						var entries []vault.ExportEntry
						if jsonErr := json.Unmarshal(data, &entries); jsonErr == nil {
							fmt.Fprintf(os.Stderr, "Import %d secret(s) from %s? [y/N]: ", len(entries), filePath)
							scanner := bufio.NewScanner(os.Stdin)
							if scanner.Scan() {
								answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
								if answer != "y" && answer != "yes" {
									fmt.Println("Aborted.")
									return nil
								}
							}
						}
					}
					var err error
					imported, err = v.ImportJSON(ctx, data)
					if err != nil {
						return err
					}
				}

				printSuccess(fmt.Sprintf("Imported %d secret(s).", imported))
				return nil
			})
		},
	}

	cmd.Flags().StringVar(&format, "format", "json", "Import format: json, env")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")
	cmd.Flags().StringP("env", "e", "", "Environment (e.g. dev, staging, prod)")
	return cmd
}

func newDestroyCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "destroy",
		Short: "Destroy the vault completely",
		Long:  "Permanently removes the database and master key. This is irreversible.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force {
				fmt.Fprint(os.Stderr, colorize("WARNING: ", ansiRed+ansiBold)+
					"This will permanently destroy the vault and all secrets.\n")
				fmt.Fprint(os.Stderr, "Continue? [y/N]: ")
				scanner := bufio.NewScanner(os.Stdin)
				if scanner.Scan() {
					answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
					if answer != "y" && answer != "yes" {
						fmt.Println("Aborted.")
						return nil
					}
				}
			}

			if err := vault.Destroy(); err != nil {
				return err
			}

			printSuccess("Vault destroyed.")
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")
	return cmd
}

func newTUICmd() *cobra.Command {
	var vimMode bool

	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Launch interactive terminal UI",
		Long: `Opens a full-screen interactive interface for managing secrets.
Colors automatically adapt to your terminal theme.
Use --vim for Normal/Visual/Insert modes with full vim motions.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withVault(func(ctx context.Context, v vault.Vault) error {
				opts := tui.Opts{VimMode: vimMode}
				p := tea.NewProgram(tui.New(v, opts), tea.WithAltScreen())
				if _, err := p.Run(); err != nil {
					return fmt.Errorf("TUI error: %w", err)
				}
				return nil
			})
		},
	}

	cmd.Flags().BoolVar(&vimMode, "vim", false, "Enable vim modes (Normal/Visual/Insert) with mode indicator")
	return cmd
}

// ── serve command ─────────────────────────────────────────────

func newServeCmd() *cobra.Command {
	var socketPath string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the REST API server on a Unix socket",
		Long: `Starts the MaestroVault REST API server, listening on a Unix domain socket.

The server provides a full CRUD REST API for secrets management, scoped token
authentication, password generation, and vault info. All requests (except
health checks) require a valid Bearer token.

Default socket: ~/.maestrovault/maestrovault.sock

Use 'mav token create' to generate API tokens before starting.`,
		Example: `  mav serve
  mav serve --socket /tmp/mav.sock`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			// Load config and check TouchID.
			cfg, err := vault.LoadConfig()
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
			if cfg.TouchID {
				auth := touchid.New()
				if err := auth.Authenticate("MaestroVault API server"); err != nil {
					return fmt.Errorf("authentication required: %w", err)
				}
			}

			v, err := vault.Open(ctx)
			if err != nil {
				printError(err.Error(), "Run 'mav init' to create a new vault.")
				return err
			}

			srv, err := api.NewServer(v, api.ServerOpts{
				SocketPath: socketPath,
				DB:         v.DB(),
			})
			if err != nil {
				printError(err.Error(), "")
				return err
			}

			printSuccess(fmt.Sprintf("Starting API server on %s", srv.SocketPath()))
			fmt.Printf("  %s\n", colorize("Press Ctrl+C to stop", ansiDim))

			if err := srv.Start(); err != nil {
				printError(err.Error(), "")
				return err
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&socketPath, "socket", "", "Custom Unix socket path (default: ~/.maestrovault/maestrovault.sock)")
	return cmd
}

// ── token command group ───────────────────────────────────────

func newTokenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Manage API tokens",
		Long: `Create, list, and revoke API tokens for the REST API server.

Tokens use scoped permissions:
  read      — get, list, search, info
  write     — set, edit, delete, import
  generate  — generate passwords
  admin     — token management (implicitly grants all other scopes)

Tokens are stored as HMAC-SHA256 hashes. The plaintext token is shown only once
at creation time — save it somewhere safe.`,
	}

	cmd.AddCommand(
		newTokenCreateCmd(),
		newTokenListCmd(),
		newTokenRevokeCmd(),
	)

	return cmd
}

func newTokenCreateCmd() *cobra.Command {
	var (
		name    string
		scopes  []string
		expires string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new API token",
		Example: `  mav token create --name "ci-read" --scope read
  mav token create --name "deploy" --scope read,write --expires 24h
  mav token create --name "admin" --scope admin`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			if len(scopes) == 0 {
				return fmt.Errorf("--scope is required (read, write, generate, admin)")
			}

			// Validate scopes.
			parsedScopes := make([]api.Scope, 0, len(scopes))
			for _, raw := range scopes {
				// Support comma-separated: --scope read,write
				for _, s := range strings.Split(raw, ",") {
					s = strings.TrimSpace(s)
					if !api.ValidScope(s) {
						return fmt.Errorf("invalid scope: %q (valid: read, write, generate, admin)", s)
					}
					parsedScopes = append(parsedScopes, api.Scope(s))
				}
			}

			return withVault(func(ctx context.Context, v vault.Vault) error {
				ts := api.NewTokenStore(v.DB())

				var expiresAt *time.Time
				if expires != "" && expires != "0" {
					d, err := time.ParseDuration(expires)
					if err != nil {
						return fmt.Errorf("invalid --expires duration: %w", err)
					}
					t := time.Now().Add(d)
					expiresAt = &t
				}

				plaintext, tok, err := ts.Create(name, parsedScopes, expiresAt)
				if err != nil {
					printError(err.Error(), "")
					return err
				}

				if resolveFormat(outputFormat) == "json" {
					out := map[string]interface{}{
						"token":      plaintext,
						"id":         tok.ID,
						"name":       tok.Name,
						"scopes":     tok.Scopes,
						"created_at": tok.CreatedAt,
					}
					if tok.ExpiresAt != nil {
						out["expires_at"] = tok.ExpiresAt
					}
					enc := json.NewEncoder(os.Stdout)
					enc.SetIndent("", "  ")
					return enc.Encode(out)
				}

				printSuccess("Token created successfully")
				fmt.Println()
				fmt.Printf("  %s  %s\n", colorize("Token:", ansiBold), plaintext)
				fmt.Printf("  %s     %s\n", colorize("ID:", ansiBold), tok.ID)
				fmt.Printf("  %s   %s\n", colorize("Name:", ansiBold), tok.Name)
				scopeStrs := make([]string, len(tok.Scopes))
				for i, s := range tok.Scopes {
					scopeStrs[i] = string(s)
				}
				fmt.Printf("  %s %s\n", colorize("Scopes:", ansiBold), strings.Join(scopeStrs, ", "))
				if tok.ExpiresAt != nil {
					fmt.Printf("  %s %s\n", colorize("Expires:", ansiBold), tok.ExpiresAt.Format(time.RFC3339))
				} else {
					fmt.Printf("  %s %s\n", colorize("Expires:", ansiBold), "never")
				}
				fmt.Println()
				printWarning("Save this token now — it cannot be retrieved again.")

				return nil
			})
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Token name (required)")
	cmd.Flags().StringSliceVar(&scopes, "scope", nil, "Token scopes: read, write, generate, admin (required)")
	cmd.Flags().StringVar(&expires, "expires", "", "Token expiry duration (e.g. 24h, 720h); omit for no expiry")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("scope")

	return cmd
}

func newTokenListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List all API tokens",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return withVault(func(ctx context.Context, v vault.Vault) error {
				ts := api.NewTokenStore(v.DB())
				tokens, err := ts.List()
				if err != nil {
					printError(err.Error(), "")
					return err
				}

				if len(tokens) == 0 {
					fmt.Printf("  %s\n", colorize("No API tokens found. Create one with 'mav token create'.", ansiDim))
					return nil
				}

				if resolveFormat(outputFormat) == "json" {
					enc := json.NewEncoder(os.Stdout)
					enc.SetIndent("", "  ")
					return enc.Encode(tokens)
				}

				// Table output.
				fmt.Printf("  %-18s %-20s %-30s %-22s %s\n",
					colorize("ID", ansiBold),
					colorize("NAME", ansiBold),
					colorize("SCOPES", ansiBold),
					colorize("CREATED", ansiBold),
					colorize("EXPIRES", ansiBold),
				)
				for _, tok := range tokens {
					scopeStrs := make([]string, len(tok.Scopes))
					for i, s := range tok.Scopes {
						scopeStrs[i] = string(s)
					}
					expires := "-"
					if tok.ExpiresAt != nil {
						if time.Now().After(*tok.ExpiresAt) {
							expires = colorize("expired", ansiRed)
						} else {
							expires = tok.ExpiresAt.Format("2006-01-02 15:04")
						}
					}
					fmt.Printf("  %-18s %-20s %-30s %-22s %s\n",
						tok.ID,
						tok.Name,
						strings.Join(scopeStrs, ", "),
						tok.CreatedAt.Format("2006-01-02 15:04"),
						expires,
					)
				}
				fmt.Printf("\n  %s\n", colorize(fmt.Sprintf("%d token(s)", len(tokens)), ansiDim))

				return nil
			})
		},
	}
}

func newTokenRevokeCmd() *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:   "revoke [id]",
		Short: "Revoke an API token by ID",
		Long: `Revoke (delete) an API token by its ID, or use --all to revoke every token.

The token is permanently deleted and can no longer be used to authenticate.`,
		Example: `  mav token revoke abc123
  mav token revoke --all`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !all && len(args) == 0 {
				return fmt.Errorf("provide a token ID or use --all")
			}

			return withVault(func(ctx context.Context, v vault.Vault) error {
				ts := api.NewTokenStore(v.DB())

				if all {
					n, err := ts.RevokeAll()
					if err != nil {
						printError(err.Error(), "")
						return err
					}
					printSuccess(fmt.Sprintf("Revoked %d token(s)", n))
					return nil
				}

				id := args[0]
				if err := ts.Revoke(id); err != nil {
					printError(err.Error(), "")
					return err
				}
				printSuccess(fmt.Sprintf("Token %s revoked", id))
				return nil
			})
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "Revoke all tokens")
	return cmd
}

// ── touchid command group ─────────────────────────────────────

func newTouchIDCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "touchid",
		Short: "Manage TouchID biometric authentication",
		Long: `Enable, disable, or check the status of TouchID integration.

When enabled, MaestroVault requires biometric authentication via TouchID
each time the vault is opened. This adds a hardware-backed security layer
on top of the macOS Keychain.

TouchID must be available on your Mac (Touch ID sensor or Apple Watch).`,
	}

	cmd.AddCommand(
		newTouchIDEnableCmd(),
		newTouchIDDisableCmd(),
		newTouchIDStatusCmd(),
	)

	return cmd
}

func newTouchIDEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable",
		Short: "Enable TouchID for vault access",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Check if TouchID hardware is available.
			available, err := touchid.Available()
			if !available {
				msg := "TouchID is not available on this device"
				if err != nil {
					msg = err.Error()
				}
				printError(msg, "TouchID requires a Mac with Touch ID sensor or paired Apple Watch.")
				return fmt.Errorf("%s", msg)
			}

			// Verify it works by doing a test authentication.
			if err := touchid.Authenticate("MaestroVault is verifying TouchID works"); err != nil {
				printError("TouchID verification failed", err.Error())
				return err
			}

			cfg, err := vault.LoadConfig()
			if err != nil {
				printError(err.Error(), "")
				return err
			}

			if cfg.TouchID {
				printSuccess("TouchID is already enabled")
				return nil
			}

			cfg.TouchID = true
			if err := vault.SaveConfig(cfg); err != nil {
				printError(err.Error(), "")
				return err
			}

			printSuccess("TouchID enabled — biometric authentication is now required to open the vault")
			return nil
		},
	}
}

func newTouchIDDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable",
		Short: "Disable TouchID for vault access",
		RunE: func(cmd *cobra.Command, args []string) error {
			// If TouchID is currently enabled, require auth to disable it.
			cfg, err := vault.LoadConfig()
			if err != nil {
				printError(err.Error(), "")
				return err
			}

			if !cfg.TouchID {
				printSuccess("TouchID is already disabled")
				return nil
			}

			// Require TouchID to disable it (prevent unauthorized disabling).
			available, _ := touchid.Available()
			if available {
				if err := touchid.Authenticate("MaestroVault wants to disable TouchID"); err != nil {
					printError("Authentication required to disable TouchID", err.Error())
					return err
				}
			}

			cfg.TouchID = false
			if err := vault.SaveConfig(cfg); err != nil {
				printError(err.Error(), "")
				return err
			}

			printSuccess("TouchID disabled — vault will open without biometric authentication")
			return nil
		},
	}
}

func newTouchIDStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show TouchID configuration and hardware status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := vault.LoadConfig()
			if err != nil {
				printError(err.Error(), "")
				return err
			}

			available, availErr := touchid.Available()

			if resolveFormat(outputFormat) == "json" {
				out := map[string]interface{}{
					"enabled":   cfg.TouchID,
					"available": available,
				}
				if availErr != nil {
					out["error"] = availErr.Error()
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}

			fmt.Println()
			enabledStr := colorize("disabled", ansiYellow)
			if cfg.TouchID {
				enabledStr = colorize("enabled", ansiGreen)
			}
			fmt.Printf("  %s  %s\n", colorize("TouchID:", ansiBold), enabledStr)

			availStr := colorize("not available", ansiRed)
			if available {
				availStr = colorize("available", ansiGreen)
			}
			fmt.Printf("  %s %s\n", colorize("Hardware:", ansiBold), availStr)

			if availErr != nil && !available {
				fmt.Printf("  %s   %s\n", colorize("Detail:", ansiBold), colorize(availErr.Error(), ansiDim))
			}
			fmt.Println()

			if cfg.TouchID && !available {
				printWarning("TouchID is enabled but hardware is not available — vault access will fail!")
			}

			return nil
		},
	}
}

// --- Helpers ---

// parseMetadata converts "key=value" strings into a map.
func parseMetadata(pairs []string) (map[string]any, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	m := make(map[string]any)
	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid metadata format %q (expected key=value)", pair)
		}
		m[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return m, nil
}

// formatMetadata renders a metadata map as a sorted comma-separated string.
func formatMetadata(m map[string]any) string {
	if len(m) == 0 {
		return colorize("-", ansiDim)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(m))
	for _, k := range keys {
		parts = append(parts, colorize(k, ansiMagenta)+"="+fmt.Sprintf("%v", m[k]))
	}
	return strings.Join(parts, ", ")
}

// toEnvName converts a secret name to an environment variable name:
// uppercase, dashes/dots/spaces to underscores.
func toEnvName(name string) string {
	r := strings.NewReplacer("-", "_", ".", "_", " ", "_", "/", "_")
	return strings.ToUpper(r.Replace(name))
}

// parseEnvFile parses a .env format string into ExportEntries.
func parseEnvFile(content string) []vault.ExportEntry {
	var entries []vault.ExportEntry
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		// Strip surrounding quotes.
		val = strings.Trim(val, "\"'")
		if key != "" {
			entries = append(entries, vault.ExportEntry{
				Name:  strings.ToLower(strings.ReplaceAll(key, "_", "-")),
				Value: val,
			})
		}
	}
	return entries
}
