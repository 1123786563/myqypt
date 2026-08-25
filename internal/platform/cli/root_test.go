package cli_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/1123786563/myqypt/internal/platform/cli"
)

func TestVersionDoesNotRunServer(t *testing.T) {
	calls := 0
	command := cli.NewRoot("be4cc10", func(context.Context) error {
		calls++
		return nil
	})
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"version"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("serve calls=%d", calls)
	}
	if got := output.String(); got != "be4cc10\n" {
		t.Fatalf("output=%q want=%q", got, "be4cc10\n")
	}
}
