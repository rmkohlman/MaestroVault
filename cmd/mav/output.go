package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"golang.org/x/term"
)

// colorEnabled returns true if colored output should be used.
func colorEnabled() bool {
	if noColor {
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// ANSI color codes.
const (
	ansiReset   = "\033[0m"
	ansiBold    = "\033[1m"
	ansiRed     = "\033[31m"
	ansiGreen   = "\033[32m"
	ansiYellow  = "\033[33m"
	ansiBlue    = "\033[34m"
	ansiMagenta = "\033[35m"
	ansiCyan    = "\033[36m"
	ansiDim     = "\033[2m"
)

func colorize(s, color string) string {
	if !colorEnabled() {
		return s
	}
	return color + s + ansiReset
}

func bold(s string) string {
	return colorize(s, ansiBold)
}

// printError prints a colored error with an optional hint.
func printError(msg string, hint string) {
	prefix := colorize("Error:", ansiRed+ansiBold)
	fmt.Fprintf(os.Stderr, "%s %s\n", prefix, msg)
	if hint != "" {
		hintPrefix := colorize("Hint:", ansiYellow)
		fmt.Fprintf(os.Stderr, "%s %s\n", hintPrefix, hint)
	}
}

// printSuccess prints a colored success message.
func printSuccess(msg string) {
	check := colorize("✓", ansiGreen)
	fmt.Printf("%s %s\n", check, msg)
}

// printWarning prints a colored warning.
func printWarning(msg string) {
	w := colorize("Warning:", ansiYellow+ansiBold)
	fmt.Fprintf(os.Stderr, "%s %s\n", w, msg)
}

// resolveFormat returns the effective output format.
// If --format is set explicitly, use it. Otherwise, auto-detect:
// TTY -> table, non-TTY -> json.
func resolveFormat(explicit string) string {
	if explicit != "" {
		return strings.ToLower(explicit)
	}
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return "json"
	}
	return "table"
}

// outputJSON marshals v as indented JSON to stdout.
func outputJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// outputTable writes tabulated data to stdout.
func outputTable(headers []string, rows [][]string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	if colorEnabled() {
		colored := make([]string, len(headers))
		for i, h := range headers {
			colored[i] = colorize(h, ansiBold+ansiCyan)
		}
		fmt.Fprintln(w, strings.Join(colored, "\t"))
	} else {
		fmt.Fprintln(w, strings.Join(headers, "\t"))
	}
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	w.Flush()
}

// outputKeyValue prints a key-value pair with formatting.
func outputKeyValue(key, value string) {
	if colorEnabled() {
		fmt.Printf("%s %s\n", colorize(key+":", ansiCyan+ansiBold), value)
	} else {
		fmt.Printf("%s %s\n", key+":", value)
	}
}
