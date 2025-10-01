package cmd

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kanon1343/fsegit/object"
	"github.com/kanon1343/fsegit/store"
)

func TestAddAndCommit(t *testing.T) {
	repoDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoDir, ".git", "objects"), 0o755); err != nil {
		t.Fatalf("failed to prepare objects dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoDir, ".git", "refs", "heads"), 0o755); err != nil {
		t.Fatalf("failed to prepare refs dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatalf("failed to write HEAD: %v", err)
	}

	filePath := filepath.Join(repoDir, "hello.txt")
	if err := os.WriteFile(filePath, []byte("hello world\n"), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	t.Setenv("GIT_AUTHOR_NAME", "Test User")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test User")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")

	if err := runAdd([]string{"hello.txt"}); err != nil {
		t.Fatalf("runAdd failed: %v", err)
	}

	if err := runCommit("Initial commit"); err != nil {
		t.Fatalf("runCommit failed: %v", err)
	}

	refPath := filepath.Join(repoDir, ".git", "refs", "heads", "main")
	refData, err := os.ReadFile(refPath)
	if err != nil {
		t.Fatalf("failed to read ref: %v", err)
	}
	commitHash := strings.TrimSpace(string(refData))
	if len(commitHash) != 40 {
		t.Fatalf("invalid commit hash: %s", commitHash)
	}

	client, err := store.NewClient(repoDir)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	hashBytes, err := hex.DecodeString(commitHash)
	if err != nil {
		t.Fatalf("failed to decode commit hash: %v", err)
	}
	obj, err := client.GetObject(hashBytes)
	if err != nil {
		t.Fatalf("failed to read commit object: %v", err)
	}
	commit, err := object.NewCommit(obj)
	if err != nil {
		t.Fatalf("failed to parse commit object: %v", err)
	}
	if commit.Message != "Initial commit" {
		t.Fatalf("unexpected commit message: %s", commit.Message)
	}

	treeObj, err := client.GetObject(commit.Tree)
	if err != nil {
		t.Fatalf("failed to read tree object: %v", err)
	}
	if treeObj.Type.String() != "tree" {
		t.Fatalf("expected tree object, got %s", treeObj.Type)
	}
	if !strings.Contains(string(treeObj.Data), "hello.txt") {
		t.Fatalf("tree does not contain staged file entry")
	}
}
