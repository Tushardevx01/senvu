// Package output provides structured output and logging for Senvu.
// It supports text and JSON output modes, leveled logging, progress
// reporting, and brand-color terminal output.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/fatih/color"
)

// Brand colors for Senvu terminal output.
var (
	// DeepInk is the primary dark color (#172D35).
	DeepInk = color.New(color.FgHiBlack)
	// Mint is the accent/success color (#83DCC3).
	Mint = color.New(color.FgHiGreen)
	// Steel is the secondary/muted color (#647985).
	Steel = color.New(color.FgHiBlue)
)

// Level represents a log severity level.
type Level int

const (
	// LevelDebug is for verbose diagnostic messages.
	LevelDebug Level = iota
	// LevelInfo is for general informational messages.
	LevelInfo
	// LevelWarn is for warning messages.
	LevelWarn
	// LevelError is for error messages.
	LevelError
)

// String returns the human-readable name of the log level.
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Output defines the interface for all Senvu output operations.
type Output interface {
	// Print writes a message to standard output.
	Print(msg string)
	// Printf writes a formatted message to standard output.
	Printf(format string, args ...any)
	// PrintJSON writes a value as JSON to standard output.
	PrintJSON(v any) error
	// Log writes a leveled log message.
	Log(level Level, msg string)
	// Logf writes a formatted leveled log message.
	Logf(level Level, format string, args ...any)
	// Success prints a success message using the Mint brand color.
	Success(msg string)
	// Warn prints a warning message.
	Warn(msg string)
	// Error prints an error message to stderr.
	Error(msg string)
}

// Progress defines the interface for reporting progress of long operations.
type Progress interface {
	// Start begins a new progress operation with the given description.
	Start(description string)
	// Update reports progress with a current/total count.
	Update(current, total int64)
	// Finish marks the progress operation as complete.
	Finish(summary string)
}

// Options configures the behavior of the output handler.
type Options struct {
	// JSONMode outputs all data as JSON instead of human-readable text.
	JSONMode bool
	// Quiet suppresses all non-error output.
	Quiet bool
	// Verbose enables debug-level logging.
	Verbose bool
	// Writer is the output destination. Defaults to os.Stdout.
	Writer io.Writer
	// ErrWriter is the error output destination. Defaults to os.Stderr.
	ErrWriter io.Writer
}

// Handler implements the Output and Progress interfaces.
type Handler struct {
	opts Options
	mu   sync.Mutex
}

// New creates a new output Handler with the given options.
func New(opts Options) *Handler {
	if opts.Writer == nil {
		opts.Writer = os.Stdout
	}
	if opts.ErrWriter == nil {
		opts.ErrWriter = os.Stderr
	}
	return &Handler{opts: opts}
}

// Print writes a message to standard output. Suppressed in quiet mode.
func (h *Handler) Print(msg string) {
	if h.opts.Quiet {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	fmt.Fprintln(h.opts.Writer, msg)
}

// Printf writes a formatted message to standard output. Suppressed in quiet mode.
func (h *Handler) Printf(format string, args ...any) {
	if h.opts.Quiet {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	fmt.Fprintf(h.opts.Writer, format+"\n", args...)
}

// PrintJSON writes a value as JSON to standard output. Not suppressed in quiet mode.
func (h *Handler) PrintJSON(v any) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	enc := json.NewEncoder(h.opts.Writer)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode JSON output: %w", err)
	}
	return nil
}

// Log writes a leveled log message. Debug messages require verbose mode.
// All non-error messages are suppressed in quiet mode.
func (h *Handler) Log(level Level, msg string) {
	if level == LevelDebug && !h.opts.Verbose {
		return
	}
	if h.opts.Quiet && level < LevelError {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.opts.JSONMode {
		logEntry := map[string]string{
			"level":   level.String(),
			"message": msg,
		}
		enc := json.NewEncoder(h.opts.ErrWriter)
		enc.Encode(logEntry) //nolint:errcheck
		return
	}

	w := h.opts.Writer
	if level >= LevelError {
		w = h.opts.ErrWriter
	}

	switch level {
	case LevelDebug:
		fmt.Fprintf(w, "%s %s\n", Steel.Sprint("[DEBUG]"), msg)
	case LevelInfo:
		fmt.Fprintf(w, "%s %s\n", Mint.Sprint("[INFO]"), msg)
	case LevelWarn:
		fmt.Fprintf(w, "%s %s\n", color.YellowString("[WARN]"), msg)
	case LevelError:
		fmt.Fprintf(w, "%s %s\n", color.RedString("[ERROR]"), msg)
	}
}

// Logf writes a formatted leveled log message.
func (h *Handler) Logf(level Level, format string, args ...any) {
	h.Log(level, fmt.Sprintf(format, args...))
}

// Success prints a success message using the Mint brand color.
func (h *Handler) Success(msg string) {
	if h.opts.Quiet {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	fmt.Fprintln(h.opts.Writer, Mint.Sprint("✓ "+msg))
}

// Warn prints a warning message.
func (h *Handler) Warn(msg string) {
	h.Log(LevelWarn, msg)
}

// Error prints an error message to stderr.
func (h *Handler) Error(msg string) {
	h.Log(LevelError, msg)
}

// Start begins a new progress operation with the given description.
func (h *Handler) Start(description string) {
	if h.opts.Quiet {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	fmt.Fprintf(h.opts.Writer, "%s %s...\n", Steel.Sprint("⟳"), description)
}

// Update reports progress with a current/total count.
func (h *Handler) Update(current, total int64) {
	if h.opts.Quiet {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if total > 0 {
		pct := float64(current) / float64(total) * 100
		fmt.Fprintf(h.opts.Writer, "\r  %d/%d (%.0f%%)", current, total, pct)
	} else {
		fmt.Fprintf(h.opts.Writer, "\r  %d processed", current)
	}
}

// Finish marks the progress operation as complete.
func (h *Handler) Finish(summary string) {
	if h.opts.Quiet {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	fmt.Fprintf(h.opts.Writer, "\n%s %s\n", Mint.Sprint("✓"), summary)
}
