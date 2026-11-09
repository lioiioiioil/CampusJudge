//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package store_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/model"
	"github.com/lioiioiioil/CampusJudge/internal/store"
)

func TestExclusiveStateOwnership(t *testing.T) {
	const contenderPath = "CAMPUSJUDGE_TEST_CONTENDER_STATE"
	if path := os.Getenv(contenderPath); path != "" {
		contender, err := store.OpenExclusive(path)
		if contender != nil {
			_ = contender.Close()
		}
		if !errors.Is(err, store.ErrStateInUse) {
			t.Fatalf("contender error = %v, want ErrStateInUse", err)
		}
		return
	}

	path := filepath.Join(t.TempDir(), "state.json")
	owner, err := store.OpenExclusive(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close() })
	if err := owner.Update(func(state *model.State) error {
		state.Groups = append(state.Groups, model.Group{ID: "saved"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// A symlink alias must contend for the same ownership even after a commit
	// replaced the JSON inode.
	alias := filepath.Join(t.TempDir(), "state-alias.json")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestExclusiveStateOwnership$")
	cmd.Env = append(os.Environ(), contenderPath+"="+alias)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("second process: %v\n%s", err, output)
	}

	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	if err := owner.Update(func(*model.State) error { return nil }); err == nil {
		t.Fatal("closed owner can still update the file")
	}
	reopened, err := store.OpenExclusive(alias)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if groups := reopened.Snapshot().Groups; len(groups) != 1 || groups[0].ID != "saved" {
		t.Fatalf("reopened state lost the committed group: %+v", groups)
	}
}

func TestExclusiveOpenReleasesOwnershipOnReadFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("invalid JSON"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.OpenExclusive(path); err == nil {
		t.Fatal("invalid state opened successfully")
	}
	if err := os.WriteFile(path, []byte(`{"version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	owner, err := store.OpenExclusive(path)
	if err != nil {
		t.Fatalf("failed read left ownership behind: %v", err)
	}
	t.Cleanup(func() { _ = owner.Close() })
}
