package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

// TestResolveConvoyBaseBranchFn verifies the function variable indirection
// works correctly — tests can inject a mock to control convoy base_branch
// lookup without shelling out to bd.
func TestResolveConvoyBaseBranchFn(t *testing.T) {
	prev := resolveConvoyBaseBranchFn
	t.Cleanup(func() { resolveConvoyBaseBranchFn = prev })

	resolveConvoyBaseBranchFn = func(beadID string) string {
		if beadID == "gt-abc" {
			return "feat/my-feature"
		}
		return ""
	}

	if got := resolveConvoyBaseBranch("gt-abc"); got != "feat/my-feature" {
		t.Errorf("resolveConvoyBaseBranch(gt-abc) = %q, want %q", got, "feat/my-feature")
	}
	if got := resolveConvoyBaseBranch("gt-other"); got != "" {
		t.Errorf("resolveConvoyBaseBranch(gt-other) = %q, want empty", got)
	}
}

// TestExecuteSlingUsesConvoyBaseBranch verifies that executeSling resolves
// base_branch from the convoy when params.BaseBranch is empty (gt-wg6).
func TestExecuteSlingUsesConvoyBaseBranch(t *testing.T) {
	townRoot := t.TempDir()

	// Set up workspace structure
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor", "rig"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	// Write minimal rigs config
	rigsJSON := `{"rigs":{"gastown":{"path":"` + filepath.Join(townRoot, "gastown") + `","prefix":"gt-"}}}`
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "rigs.json"), []byte(rigsJSON), 0644); err != nil {
		t.Fatalf("write rigs.json: %v", err)
	}

	// Create rig directory structure
	if err := os.MkdirAll(filepath.Join(townRoot, "gastown", "polecats"), 0755); err != nil {
		t.Fatalf("mkdir gastown: %v", err)
	}

	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}

	// bd stub that returns open bead info
	bdScript := `#!/bin/sh
cmd="$1"
shift || true
case "$cmd" in
  show)
    echo '[{"title":"Test issue","status":"open","assignee":"","description":"","labels":[]}]'
    ;;
  update|dep|create)
    exit 0
    ;;
esac
exit 0
`
	writeBDStub(t, binDir, bdScript, "")

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(EnvGTRole, "mayor")
	t.Setenv("GT_POLECAT", "")
	t.Setenv("GT_CREW", "")
	t.Setenv("TMUX_PANE", "")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(filepath.Join(townRoot, "mayor", "rig")); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	// Mock resolveConvoyBaseBranchFn to return a feature branch
	prevConvoyFn := resolveConvoyBaseBranchFn
	t.Cleanup(func() { resolveConvoyBaseBranchFn = prevConvoyFn })
	resolveConvoyBaseBranchFn = func(beadID string) string {
		if beadID == "gt-test-convoy" {
			return "feat/convoy-branch"
		}
		return ""
	}

	// Mock spawnPolecatForSling to capture the BaseBranch passed
	prevSpawn := spawnPolecatForSling
	t.Cleanup(func() { spawnPolecatForSling = prevSpawn })

	var capturedBaseBranch string
	spawnPolecatForSling = func(rigName string, opts SlingSpawnOptions) (*SpawnedPolecatInfo, error) {
		capturedBaseBranch = opts.BaseBranch
		// Return error to stop execution after spawn (we only care about the BaseBranch)
		return nil, errTestStopAfterSpawn
	}

	prevDeadFn := isHookedAgentDeadFn
	t.Cleanup(func() { isHookedAgentDeadFn = prevDeadFn })
	isHookedAgentDeadFn = func(assignee string) bool { return false }

	// Call executeSling with empty BaseBranch — should resolve from convoy
	_, _ = executeSling(SlingParams{
		BeadID:   "gt-test-convoy",
		RigName:  "gastown",
		NoConvoy: true,
		TownRoot: townRoot,
	})

	if capturedBaseBranch != "feat/convoy-branch" {
		t.Errorf("executeSling did not propagate convoy base_branch to spawn: got %q, want %q",
			capturedBaseBranch, "feat/convoy-branch")
	}
}

// TestExecuteSlingExplicitBaseBranchOverridesConvoy verifies that an explicit
// --base-branch flag takes precedence over the convoy's base_branch.
func TestExecuteSlingExplicitBaseBranchOverridesConvoy(t *testing.T) {
	townRoot := t.TempDir()

	if err := os.MkdirAll(filepath.Join(townRoot, "mayor", "rig"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	rigsJSON := `{"rigs":{"gastown":{"path":"` + filepath.Join(townRoot, "gastown") + `","prefix":"gt-"}}}`
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "rigs.json"), []byte(rigsJSON), 0644); err != nil {
		t.Fatalf("write rigs.json: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, "gastown", "polecats"), 0755); err != nil {
		t.Fatalf("mkdir gastown: %v", err)
	}

	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	bdScript := `#!/bin/sh
cmd="$1"
case "$cmd" in
  show)
    echo '[{"title":"Test","status":"open","assignee":"","description":"","labels":[]}]'
    ;;
esac
exit 0
`
	writeBDStub(t, binDir, bdScript, "")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(EnvGTRole, "mayor")
	t.Setenv("GT_POLECAT", "")
	t.Setenv("GT_CREW", "")
	t.Setenv("TMUX_PANE", "")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(filepath.Join(townRoot, "mayor", "rig")); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	// Mock convoy to return a branch
	prevConvoyFn := resolveConvoyBaseBranchFn
	t.Cleanup(func() { resolveConvoyBaseBranchFn = prevConvoyFn })
	resolveConvoyBaseBranchFn = func(beadID string) string {
		return "feat/convoy-branch"
	}

	// Capture spawn BaseBranch
	prevSpawn := spawnPolecatForSling
	t.Cleanup(func() { spawnPolecatForSling = prevSpawn })
	var capturedBaseBranch string
	spawnPolecatForSling = func(rigName string, opts SlingSpawnOptions) (*SpawnedPolecatInfo, error) {
		capturedBaseBranch = opts.BaseBranch
		return nil, errTestStopAfterSpawn
	}

	prevDeadFn := isHookedAgentDeadFn
	t.Cleanup(func() { isHookedAgentDeadFn = prevDeadFn })
	isHookedAgentDeadFn = func(assignee string) bool { return false }

	// Explicit base_branch should win over convoy
	_, _ = executeSling(SlingParams{
		BeadID:     "gt-test-explicit",
		RigName:    "gastown",
		BaseBranch: "release/v2",
		NoConvoy:   true,
		TownRoot:   townRoot,
	})

	if capturedBaseBranch != "release/v2" {
		t.Errorf("explicit BaseBranch not preserved: got %q, want %q",
			capturedBaseBranch, "release/v2")
	}
}

// TestExecuteSlingNoConvoyBaseBranch verifies that when the convoy has no
// base_branch, executeSling proceeds with empty BaseBranch (default to main).
func TestExecuteSlingNoConvoyBaseBranch(t *testing.T) {
	townRoot := t.TempDir()

	if err := os.MkdirAll(filepath.Join(townRoot, "mayor", "rig"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	rigsJSON := `{"rigs":{"gastown":{"path":"` + filepath.Join(townRoot, "gastown") + `","prefix":"gt-"}}}`
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "rigs.json"), []byte(rigsJSON), 0644); err != nil {
		t.Fatalf("write rigs.json: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, "gastown", "polecats"), 0755); err != nil {
		t.Fatalf("mkdir gastown: %v", err)
	}

	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	bdScript := `#!/bin/sh
cmd="$1"
case "$cmd" in
  show)
    echo '[{"title":"Test","status":"open","assignee":"","description":"","labels":[]}]'
    ;;
esac
exit 0
`
	writeBDStub(t, binDir, bdScript, "")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(EnvGTRole, "mayor")
	t.Setenv("GT_POLECAT", "")
	t.Setenv("GT_CREW", "")
	t.Setenv("TMUX_PANE", "")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(filepath.Join(townRoot, "mayor", "rig")); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	// Mock convoy to return empty (no base_branch)
	prevConvoyFn := resolveConvoyBaseBranchFn
	t.Cleanup(func() { resolveConvoyBaseBranchFn = prevConvoyFn })
	resolveConvoyBaseBranchFn = func(beadID string) string { return "" }

	prevSpawn := spawnPolecatForSling
	t.Cleanup(func() { spawnPolecatForSling = prevSpawn })
	var capturedBaseBranch string
	spawnPolecatForSling = func(rigName string, opts SlingSpawnOptions) (*SpawnedPolecatInfo, error) {
		capturedBaseBranch = opts.BaseBranch
		return nil, errTestStopAfterSpawn
	}

	prevDeadFn := isHookedAgentDeadFn
	t.Cleanup(func() { isHookedAgentDeadFn = prevDeadFn })
	isHookedAgentDeadFn = func(assignee string) bool { return false }

	_, _ = executeSling(SlingParams{
		BeadID:   "gt-test-no-convoy-branch",
		RigName:  "gastown",
		NoConvoy: true,
		TownRoot: townRoot,
	})

	if capturedBaseBranch != "" {
		t.Errorf("expected empty BaseBranch when convoy has none, got %q", capturedBaseBranch)
	}
}

// TestConvoyFieldsBaseBranchInConvoyCreation verifies that createAutoConvoy
// stores base_branch in the convoy's description fields.
func TestConvoyFieldsBaseBranchInConvoyCreation(t *testing.T) {
	// Test that ConvoyFields round-trips base_branch through
	// SetConvoyFields → ParseConvoyFields correctly when used
	// in the pattern that createAutoConvoy uses.
	prose := "Auto-created convoy tracking gt-abc"
	fields := &beads.ConvoyFields{
		Merge:      "mr",
		BaseBranch: "feat/my-feature",
	}
	description := beads.SetConvoyFields(&beads.Issue{Description: prose}, fields)

	// Verify base_branch is in the description
	if !strings.Contains(description, "base_branch: feat/my-feature") {
		t.Errorf("convoy description missing base_branch, got:\n%s", description)
	}

	// Verify it round-trips
	parsed := beads.ParseConvoyFields(&beads.Issue{Description: description})
	if parsed == nil {
		t.Fatal("ParseConvoyFields returned nil")
	}
	if parsed.BaseBranch != "feat/my-feature" {
		t.Errorf("round-trip BaseBranch: got %q, want %q", parsed.BaseBranch, "feat/my-feature")
	}
	if parsed.Merge != "mr" {
		t.Errorf("round-trip Merge: got %q, want %q", parsed.Merge, "mr")
	}

	// Verify prose is preserved
	if !strings.Contains(description, prose) {
		t.Errorf("convoy description lost prose, got:\n%s", description)
	}
}

// TestConvoyFieldsBaseBranchOmittedWhenEmpty verifies that an empty base_branch
// is not stored in the convoy description (keeps descriptions clean).
func TestConvoyFieldsBaseBranchOmittedWhenEmpty(t *testing.T) {
	fields := &beads.ConvoyFields{
		Merge: "direct",
	}
	description := beads.SetConvoyFields(&beads.Issue{Description: "tracking gt-abc"}, fields)
	if strings.Contains(description, "base_branch") {
		t.Errorf("convoy description should not contain base_branch when empty, got:\n%s", description)
	}
}

// TestScheduleBeadResolvesConvoyBaseBranch verifies that the scheduler path
// resolves base_branch from convoy when not explicitly set (gt-wg6).
// Since scheduleBead requires a full workspace with Dolt, we test the
// resolution logic directly by verifying the code pattern:
//   effectiveBaseBranch := opts.BaseBranch
//   if effectiveBaseBranch == "" { effectiveBaseBranch = resolveConvoyBaseBranch(beadID) }
func TestScheduleBeadResolvesConvoyBaseBranch(t *testing.T) {
	prev := resolveConvoyBaseBranchFn
	t.Cleanup(func() { resolveConvoyBaseBranchFn = prev })

	tests := []struct {
		name             string
		optBaseBranch    string
		convoyBaseBranch string
		wantBaseBranch   string
	}{
		{
			name:             "convoy provides base_branch when opts empty",
			optBaseBranch:    "",
			convoyBaseBranch: "feat/scheduled-branch",
			wantBaseBranch:   "feat/scheduled-branch",
		},
		{
			name:             "explicit opts override convoy",
			optBaseBranch:    "release/v2",
			convoyBaseBranch: "feat/convoy-branch",
			wantBaseBranch:   "release/v2",
		},
		{
			name:             "no convoy no opts stays empty",
			optBaseBranch:    "",
			convoyBaseBranch: "",
			wantBaseBranch:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolveConvoyBaseBranchFn = func(beadID string) string {
				return tt.convoyBaseBranch
			}

			// Replicate the resolution logic from scheduleBead
			effectiveBaseBranch := tt.optBaseBranch
			if effectiveBaseBranch == "" {
				effectiveBaseBranch = resolveConvoyBaseBranch("gt-test")
			}

			if effectiveBaseBranch != tt.wantBaseBranch {
				t.Errorf("effective base_branch = %q, want %q", effectiveBaseBranch, tt.wantBaseBranch)
			}
		})
	}
}

// TestConvoyInfoBaseBranchFromDescription verifies that getConvoyInfoForIssue
// (via ParseConvoyFields) correctly extracts BaseBranch from a convoy's
// description alongside other fields.
func TestConvoyInfoBaseBranchFromDescription(t *testing.T) {
	// Simulate what getConvoyInfoForIssue does internally:
	// parse convoy description to extract fields including BaseBranch
	description := "Auto-created convoy tracking gt-abc\nOwner: mayor/\nMerge: mr\nbase_branch: feat/auth"

	fields := beads.ParseConvoyFields(&beads.Issue{Description: description})
	if fields == nil {
		t.Fatal("ParseConvoyFields returned nil")
	}

	// Verify all fields
	if fields.Owner != "mayor/" {
		t.Errorf("Owner: got %q, want %q", fields.Owner, "mayor/")
	}
	if fields.Merge != "mr" {
		t.Errorf("Merge: got %q, want %q", fields.Merge, "mr")
	}
	if fields.BaseBranch != "feat/auth" {
		t.Errorf("BaseBranch: got %q, want %q", fields.BaseBranch, "feat/auth")
	}
}

// errTestStopAfterSpawn is a sentinel error used to stop executeSling
// after the spawn step so we can inspect what was passed.
var errTestStopAfterSpawn = errSentinel("test: stop after spawn")

type errSentinel string

func (e errSentinel) Error() string { return string(e) }
