package server

import (
	"strings"
	"testing"

	agentsv1 "github.com/agynio/agents/.gen/go/agynio/api/agents/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func tab(shellID string) *agentsv1.SandboxTab {
	return &agentsv1.SandboxTab{ShellId: shellID, Number: 1}
}

func TestSandboxTabsAccepted(t *testing.T) {
	name := "build"
	cwd := "/workspace/api"
	tabs, err := fromProtoSandboxTabs([]*agentsv1.SandboxTab{
		{ShellId: "shell-a", Number: 1, NameOverride: &name, Cwd: &cwd},
		{ShellId: "shell-b", Number: 2},
	})
	if err != nil {
		t.Fatalf("rejected a valid layout: %v", err)
	}
	if len(tabs) != 2 {
		t.Fatalf("got %d tabs, want 2", len(tabs))
	}
	if tabs[0].NameOverride == nil || *tabs[0].NameOverride != "build" {
		t.Fatal("name override dropped")
	}
	if tabs[0].CWD == nil || *tabs[0].CWD != "/workspace/api" {
		t.Fatal("cwd dropped")
	}
	// A derived name is never stored: it goes stale the moment the shell
	// changes directory, and a stored one would outlive what it described.
	if tabs[1].NameOverride != nil {
		t.Fatal("an absent name override became a stored name")
	}
}

// A layout may only name shells a ticket could actually be issued for.
// Otherwise the tab exists and cannot be opened, which is a worse outcome than
// refusing to write it.
func TestSandboxTabsRejectUnusableShellIDs(t *testing.T) {
	for _, id := range []string{"", "shell.a", "shell:a", "shell a", strings.Repeat("a", 65)} {
		if _, err := fromProtoSandboxTabs([]*agentsv1.SandboxTab{tab(id)}); err == nil {
			t.Fatalf("accepted shell_id %q", id)
		}
	}
}

// Two tabs on one shell both attach to it, and the second displaces the first
// inside the same strip -- a tab fighting its neighbour for a PTY.
func TestSandboxTabsRejectDuplicateShells(t *testing.T) {
	_, err := fromProtoSandboxTabs([]*agentsv1.SandboxTab{tab("shell-a"), tab("shell-a")})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("duplicate shell accepted: %v", err)
	}
}

func TestSandboxTabsRejectRelativeCwd(t *testing.T) {
	cwd := "workspace"
	_, err := fromProtoSandboxTabs([]*agentsv1.SandboxTab{{ShellId: "shell-a", Cwd: &cwd}})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("relative cwd accepted: %v", err)
	}
}

// Every tab costs memory in the container for as long as it holds a shell, so
// the document a client can write is bounded even though no person will reach
// the bound.
func TestSandboxTabsBounded(t *testing.T) {
	many := make([]*agentsv1.SandboxTab, 0, maxSandboxTabs+1)
	for i := range maxSandboxTabs + 1 {
		many = append(many, &agentsv1.SandboxTab{ShellId: "shell-" + string(rune('a'+i%26)) + string(rune('a'+i/26)), Number: int32(i)})
	}
	if _, err := fromProtoSandboxTabs(many); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("unbounded layout accepted: %v", err)
	}
}

func TestEmptyLayoutIsValid(t *testing.T) {
	tabs, err := fromProtoSandboxTabs(nil)
	if err != nil {
		t.Fatalf("rejected an empty layout: %v", err)
	}
	if len(tabs) != 0 {
		t.Fatalf("got %d tabs, want 0", len(tabs))
	}
}
