// MaestroVault CLI — a macOS-first developer-focused secrets management tool.
//
// Usage:
//
//	maestrovault init                  Initialize a new vault
//	maestrovault set <name>            Store a secret
//	maestrovault get <name>            Retrieve a secret
//	maestrovault list                  List all secrets
//	maestrovault delete <name>         Delete a secret
//	maestrovault edit <name>           Edit an existing secret
//	maestrovault copy <name>           Copy a secret to the clipboard
//	maestrovault search <query>        Search secrets by name or labels
//	maestrovault generate              Generate a random password
//	maestrovault env                   Export secrets as environment variables
//	maestrovault exec -- <cmd>         Run a command with secrets injected as env vars
//	maestrovault export                Export vault to file
//	maestrovault import <file>         Import secrets from file
//	maestrovault destroy               Destroy the vault completely
//	maestrovault tui                   Launch interactive TUI
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rmkohlman/MaestroVault/internal/clipboard"
	"github.com/rmkohlman/MaestroVault/internal/crypto"
	"github.com/rmkohlman/MaestroVault/internal/tui"
	"github.com/rmkohlman/MaestroVault/internal/vault"
	"github.com/spf13/cobra"
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
		Use:   "maestrovault",
		Short: "MaestroVault — secure local secrets management for developers",
		Long: `MaestroVault is a lightweight, macOS-first secrets manager for developers.

Secrets are encrypted with AES-256-GCM using envelope encryption.
The master key is stored securely in the macOS Keychain.`,
		SilenceErrors: true,
		SilenceUsage:  true,
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
	)

	return root
}

// --- Secret name completion helper ---

func secretNameCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	v, err := vault.Open()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	defer v.Close()

	names, err := v.Names()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return names, cobra.ShellCompDirectiveNoFileComp
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
			if err := vault.Init(); err != nil {
				printError(err.Error(), "If the vault already exists, use 'maestro destroy' first to reset it.")
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
		labels     []string
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
				fmt.Fprint(os.Stderr, "Enter secret value: ")
				scanner := bufio.NewScanner(os.Stdin)
				if scanner.Scan() {
					value = scanner.Text()
				}
				if err := scanner.Err(); err != nil {
					return fmt.Errorf("reading input: %w", err)
				}
			}

			if value == "" {
				printError("Secret value cannot be empty.", "Use --value, --generate, or pipe via stdin.")
				return fmt.Errorf("empty value")
			}

			labelMap := parseLabels(labels)

			v, err := vault.Open()
			if err != nil {
				printError(err.Error(), "Run 'maestro init' to create a new vault.")
				return err
			}
			defer v.Close()

			if err := v.Set(name, value, labelMap); err != nil {
				return err
			}

			printSuccess(fmt.Sprintf("Secret %q stored.", name))
			if generatePw {
				fmt.Printf("  %s  %s\n", colorize("Generated:", ansiDim), colorize(value, ansiGreen))
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&valueFlag, "value", "v", "", "Secret value (reads from stdin if omitted)")
	cmd.Flags().StringSliceVarP(&labels, "label", "l", nil, "Labels as key=value (repeatable)")
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
			v, err := vault.Open()
			if err != nil {
				printError(err.Error(), "Run 'maestro init' to create a new vault.")
				return err
			}
			defer v.Close()

			entry, err := v.Get(args[0])
			if err != nil {
				printError(err.Error(), "Use 'maestro list' to see available secrets.")
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
			outputKeyValue("Value", colorize(entry.Value, ansiGreen))
			if len(entry.Labels) > 0 {
				outputKeyValue("Labels", formatLabels(entry.Labels))
			}
			outputKeyValue("Created", entry.CreatedAt)
			outputKeyValue("Updated", entry.UpdatedAt)
			return nil
		},
	}

	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Print only the value (for piping)")
	cmd.Flags().BoolVarP(&clip, "clip", "c", false, "Copy value to clipboard (auto-clears in 45s)")
	return cmd
}

func newListCmd() *cobra.Command {
	var (
		filter string
		label  string
	)

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List all secrets",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			v, err := vault.Open()
			if err != nil {
				printError(err.Error(), "Run 'maestro init' to create a new vault.")
				return err
			}
			defer v.Close()

			var entries []vault.SecretEntry

			switch {
			case filter != "":
				entries, err = v.Search(filter)
			case label != "":
				parts := strings.SplitN(label, "=", 2)
				key := parts[0]
				val := ""
				if len(parts) == 2 {
					val = parts[1]
				}
				entries, err = v.ListByLabel(key, val)
			default:
				entries, err = v.List()
			}
			if err != nil {
				return err
			}

			if len(entries) == 0 {
				fmt.Println(colorize("No secrets stored.", ansiDim))
				fmt.Printf("  %s maestro set <name> --value <value>\n", colorize("Hint:", ansiYellow))
				return nil
			}

			format := resolveFormat(outputFormat)
			if format == "json" {
				return outputJSON(entries)
			}

			headers := []string{"NAME", "LABELS", "CREATED", "UPDATED"}
			rows := make([][]string, len(entries))
			for i, e := range entries {
				rows[i] = []string{
					colorize(e.Name, ansiBold),
					formatLabels(e.Labels),
					colorize(e.CreatedAt, ansiDim),
					colorize(e.UpdatedAt, ansiDim),
				}
			}
			outputTable(headers, rows)

			count := colorize(fmt.Sprintf("%d secret(s)", len(entries)), ansiDim)
			fmt.Printf("\n%s\n", count)
			return nil
		},
	}

	cmd.Flags().StringVarP(&filter, "filter", "f", "", "Filter secrets by name or label content")
	cmd.Flags().StringVar(&label, "label", "", "Filter by label (key or key=value)")
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

			v, err := vault.Open()
			if err != nil {
				return err
			}
			defer v.Close()

			if err := v.Delete(name); err != nil {
				printError(err.Error(), "Use 'maestro list' to see available secrets.")
				return err
			}

			printSuccess(fmt.Sprintf("Secret %q deleted.", name))
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")
	return cmd
}

func newEditCmd() *cobra.Command {
	var (
		valueFlag string
		labels    []string
	)

	cmd := &cobra.Command{
		Use:               "edit <name>",
		Short:             "Edit an existing secret",
		Long:              "Update the value and/or labels of an existing secret. Unspecified fields are preserved.",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: secretNameCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			v, err := vault.Open()
			if err != nil {
				return err
			}
			defer v.Close()

			var newLabels map[string]string
			if len(labels) > 0 {
				newLabels = parseLabels(labels)
			}

			if err := v.Edit(name, valueFlag, newLabels); err != nil {
				printError(err.Error(), "Use 'maestro list' to see available secrets.")
				return err
			}

			printSuccess(fmt.Sprintf("Secret %q updated.", name))
			return nil
		},
	}

	cmd.Flags().StringVarP(&valueFlag, "value", "v", "", "New secret value (keeps existing if omitted)")
	cmd.Flags().StringSliceVarP(&labels, "label", "l", nil, "New labels as key=value (replaces all labels)")
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
			v, err := vault.Open()
			if err != nil {
				return err
			}
			defer v.Close()

			entry, err := v.Get(args[0])
			if err != nil {
				printError(err.Error(), "Use 'maestro list' to see available secrets.")
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
		},
	}

	cmd.Flags().IntVar(&clearAfter, "clear", 45, "Seconds before clipboard is auto-cleared (0 to disable)")
	return cmd
}

func newSearchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Search secrets by name or labels",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			v, err := vault.Open()
			if err != nil {
				return err
			}
			defer v.Close()

			entries, err := v.Search(args[0])
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

			headers := []string{"NAME", "LABELS", "CREATED", "UPDATED"}
			rows := make([][]string, len(entries))
			for i, e := range entries {
				rows[i] = []string{
					colorize(e.Name, ansiBold),
					formatLabels(e.Labels),
					colorize(e.CreatedAt, ansiDim),
					colorize(e.UpdatedAt, ansiDim),
				}
			}
			outputTable(headers, rows)

			count := colorize(fmt.Sprintf("%d result(s)", len(entries)), ansiDim)
			fmt.Printf("\n%s\n", count)
			return nil
		},
	}
}

func newGenerateCmd() *cobra.Command {
	var (
		length     int
		uppercase  bool
		lowercase  bool
		digits     bool
		symbols    bool
		name       string
		labels     []string
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
				v, err := vault.Open()
				if err != nil {
					return err
				}
				defer v.Close()

				labelMap := parseLabels(labels)
				if err := v.Set(name, result, labelMap); err != nil {
					return err
				}
				printSuccess(fmt.Sprintf("Generated and stored as %q.", name))
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
	cmd.Flags().StringSliceVarP(&labels, "label", "l", nil, "Labels for stored secret (with --name)")
	cmd.Flags().BoolVarP(&clip, "clip", "c", false, "Copy to clipboard")
	cmd.Flags().BoolVar(&passphrase, "passphrase", false, "Generate a passphrase instead of a password")
	cmd.Flags().IntVar(&words, "words", 5, "Number of words in passphrase (with --passphrase)")
	cmd.Flags().StringVar(&delimiter, "delimiter", "-", "Word delimiter for passphrase (with --passphrase)")

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
  eval $(maestro env)
  eval $(maestro env --prefix APP_)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			v, err := vault.Open()
			if err != nil {
				return err
			}
			defer v.Close()

			entries, err := v.Export()
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
  maestro exec -- env
  maestro exec --prefix DB_ -- psql`,
		DisableFlagParsing: false,
		Args:               cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			v, err := vault.Open()
			if err != nil {
				return err
			}
			defer v.Close()

			entries, err := v.Export()
			if err != nil {
				return err
			}

			env := os.Environ()
			for _, e := range entries {
				if filter != "" && !strings.Contains(strings.ToLower(e.Name), strings.ToLower(filter)) {
					continue
				}
				envName := prefix + toEnvName(e.Name)
				env = append(env, fmt.Sprintf("%s=%s", envName, e.Value))
			}

			binary, err := exec.LookPath(args[0])
			if err != nil {
				return fmt.Errorf("command not found: %s", args[0])
			}

			return syscall.Exec(binary, args, env)
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
  maestro export > backup.json
  maestro export --format env > .env`,
		RunE: func(cmd *cobra.Command, args []string) error {
			v, err := vault.Open()
			if err != nil {
				return err
			}
			defer v.Close()

			entries, err := v.Export()
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
		},
	}

	cmd.Flags().StringVar(&format, "format", "json", "Export format: json, env")
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
  maestro import backup.json
  maestro import --format env .env`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]
			data, err := os.ReadFile(filePath)
			if err != nil {
				printError(fmt.Sprintf("Cannot read file: %s", err), "Check the file path exists and is readable.")
				return err
			}

			v, err := vault.Open()
			if err != nil {
				return err
			}
			defer v.Close()

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
				imported, err = v.Import(entries)
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
				imported, err = v.ImportJSON(data)
			}

			if err != nil {
				return err
			}

			printSuccess(fmt.Sprintf("Imported %d secret(s).", imported))
			return nil
		},
	}

	cmd.Flags().StringVar(&format, "format", "json", "Import format: json, env")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")
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
			v, err := vault.Open()
			if err != nil {
				printError(err.Error(), "Run 'maestro init' to create a new vault.")
				return err
			}
			defer v.Close()

			opts := tui.Opts{VimMode: vimMode}
			p := tea.NewProgram(tui.New(v, opts), tea.WithAltScreen())
			if _, err := p.Run(); err != nil {
				return fmt.Errorf("TUI error: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&vimMode, "vim", false, "Enable vim modes (Normal/Visual/Insert) with mode indicator")
	return cmd
}

// --- Helpers ---

// parseLabels converts "key=value" strings into a map.
func parseLabels(labels []string) map[string]string {
	result := make(map[string]string)
	for _, l := range labels {
		parts := strings.SplitN(l, "=", 2)
		if len(parts) == 2 {
			result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return result
}

// formatLabels renders a label map as a comma-separated string.
func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return colorize("-", ansiDim)
	}
	parts := make([]string, 0, len(labels))
	for k, v := range labels {
		parts = append(parts, colorize(k, ansiMagenta)+"="+v)
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
