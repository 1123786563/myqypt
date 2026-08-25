package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/1123786563/myqypt/internal/platform/cli"
)

func TestVersionDoesNotRunServer(t *testing.T) {
	calls := 0
	migrateCalls := 0
	command := cli.NewRoot("be4cc10",
		func(context.Context) error {
			calls++
			return nil
		},
		cli.MigrateFuncs{
			Up: func(context.Context) error {
				migrateCalls++
				return nil
			},
			DownOne: func(context.Context) error {
				migrateCalls++
				return nil
			},
		},
	)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"version"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("serve calls=%d", calls)
	}
	if migrateCalls != 0 {
		t.Fatalf("migrate calls=%d", migrateCalls)
	}
	if got := output.String(); got != "be4cc10\n" {
		t.Fatalf("output=%q want=%q", got, "be4cc10\n")
	}
}

func TestMigrateUpRequiresDatabaseURL(t *testing.T) {
	serveCalls := 0
	migrateCalls := 0
	command := cli.NewRoot("dev",
		func(context.Context) error {
			serveCalls++
			return nil
		},
		cli.MigrateFuncs{
			Up: func(context.Context) error {
				migrateCalls++
				return nil
			},
			DownOne: func(context.Context) error {
				migrateCalls++
				return nil
			},
		},
	)
	t.Setenv("DATABASE_URL", "")
	command.SetArgs([]string{"migrate", "up"})

	err := command.Execute()
	if err == nil {
		t.Fatal("expected an error when DATABASE_URL is not set")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("error %q does not mention DATABASE_URL", err.Error())
	}
	if serveCalls != 0 {
		t.Fatalf("serve calls=%d", serveCalls)
	}
	if migrateCalls != 0 {
		t.Fatalf("migrate calls=%d", migrateCalls)
	}
}
