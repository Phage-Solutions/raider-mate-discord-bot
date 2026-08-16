package discord

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

// panicking is the shape recoverPanic exists for: a goroutine body that dereferences
// something Discord did not send. discordgo runs each handler in its own goroutine, so
// before the recover this ended the process rather than the call.
func panicking(ctx context.Context, b *Bot) (survived bool) {
	defer b.recoverPanic(ctx, "test goroutine")

	var member *struct{ ID string }
	_ = member.ID

	return true
}

func TestRecoverPanicKeepsTheProcessAlive(t *testing.T) {
	var buf bytes.Buffer
	b := &Bot{logger: slog.New(slog.NewJSONHandler(&buf, nil))}

	survived := panicking(context.Background(), b)

	// False because the deferred recover resumes after the panicking call rather than
	// at the return, which is the point: the goroutine unwinds and the process does not.
	if survived {
		t.Error("survived = true, want the panicking call abandoned")
	}

	logged := buf.String()
	if !strings.Contains(logged, "recovered panic") {
		t.Fatalf("log = %s, want the panic reported through slog and not the runtime's stderr trace", logged)
	}
	if !strings.Contains(logged, "test goroutine") {
		t.Errorf("log = %s, want the goroutine named: a stack says where it broke, not which loop stopped", logged)
	}
	// Without the stack the log names a nil dereference and no line, which is the same
	// dead end as having no log at all.
	if !strings.Contains(logged, "runtime/debug.Stack") {
		t.Errorf("log = %s, want a stack trace attached", logged)
	}
}

func TestRecoverPanicLogsNothingWithoutAPanic(t *testing.T) {
	var buf bytes.Buffer
	b := &Bot{logger: slog.New(slog.NewJSONHandler(&buf, nil))}

	func() {
		defer b.recoverPanic(context.Background(), "test goroutine")
	}()

	if buf.Len() != 0 {
		t.Errorf("log = %s, want nothing on the ordinary path", buf.String())
	}
}
