package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kanon1343/fsegit/object"
	"github.com/kanon1343/fsegit/sha"
	"github.com/kanon1343/fsegit/store"
	"github.com/spf13/cobra"
)

var commitMessage string

var commitCmd = &cobra.Command{
	Use:   "commit",
	Short: "ステージされた変更をコミットします",
	RunE: func(cmd *cobra.Command, args []string) error {
		if commitMessage == "" {
			return errors.New("コミットメッセージを指定してください (-m)")
		}
		return runCommit(commitMessage)
	},
}

func init() {
	commitCmd.Flags().StringVarP(&commitMessage, "message", "m", "", "コミットメッセージ")
	rootCmd.AddCommand(commitCmd)
}

func runCommit(message string) error {
	client, err := store.NewClient("./")
	if err != nil {
		return err
	}

	idx, err := store.LoadIndex(client.RootDir())
	if err != nil {
		return err
	}
	if len(idx.Entries) == 0 {
		return errors.New("ステージされた変更がありません")
	}

	treeHash, err := idx.BuildTree(client)
	if err != nil {
		return err
	}

	refPath, parentHash, err := readHead(client.RootDir())
	if err != nil {
		return err
	}

	now := time.Now()
	authorNameValue := authorName()
	authorEmailValue := authorEmail()
	author := buildSignature(authorNameValue, authorEmailValue, now)
	committer := buildSignature(committerName(authorNameValue), committerEmail(authorEmailValue), now)

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("tree %s\n", treeHash.String()))
	if parentHash != "" {
		builder.WriteString(fmt.Sprintf("parent %s\n", parentHash))
	}
	builder.WriteString(fmt.Sprintf("author %s\n", author))
	builder.WriteString(fmt.Sprintf("committer %s\n", committer))
	builder.WriteString("\n")
	builder.WriteString(message)
	builder.WriteString("\n")

	commitHash, err := client.WriteObject(object.CommitObject, []byte(builder.String()))
	if err != nil {
		return err
	}

	if err := updateHead(client.RootDir(), refPath, commitHash); err != nil {
		return err
	}

	idx.Clear()
	if err := idx.Save(client.RootDir()); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "[%s] %s\n", commitHash.String(), message)
	return nil
}

func readHead(root string) (string, string, error) {
	headPath := filepath.Join(root, ".git", "HEAD")
	data, err := os.ReadFile(headPath)
	if err != nil {
		return "", "", err
	}

	head := strings.TrimSpace(string(data))
	if strings.HasPrefix(head, "ref: ") {
		ref := filepath.Join(root, ".git", head[5:])
		refData, err := os.ReadFile(ref)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return ref, "", nil
			}
			return "", "", err
		}
		return ref, strings.TrimSpace(string(refData)), nil
	}

	return "", head, nil
}

func updateHead(root, refPath string, hash sha.SHA1) error {
	hashString := hash.String()
	if refPath != "" {
		if err := os.MkdirAll(filepath.Dir(refPath), 0o755); err != nil {
			return err
		}
		return os.WriteFile(refPath, []byte(hashString+"\n"), 0o644)
	}
	headPath := filepath.Join(root, ".git", "HEAD")
	return os.WriteFile(headPath, []byte(hashString+"\n"), 0o644)
}

func authorName() string {
	return firstNonEmpty(os.Getenv("GIT_AUTHOR_NAME"), os.Getenv("GIT_COMMITTER_NAME"), os.Getenv("USER"), "Anonymous")
}

func authorEmail() string {
	return firstNonEmpty(os.Getenv("GIT_AUTHOR_EMAIL"), os.Getenv("GIT_COMMITTER_EMAIL"), "unknown@example.com")
}

func committerName(fallback string) string {
	return firstNonEmpty(os.Getenv("GIT_COMMITTER_NAME"), fallback)
}

func committerEmail(fallback string) string {
	return firstNonEmpty(os.Getenv("GIT_COMMITTER_EMAIL"), fallback)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func buildSignature(name, email string, t time.Time) string {
	if name == "" {
		name = "Anonymous"
	}
	if email == "" {
		email = "unknown@example.com"
	}
	_, offset := t.Zone()
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	hours := offset / 3600
	minutes := (offset % 3600) / 60
	return fmt.Sprintf("%s <%s> %d %s%02d%02d", name, email, t.Unix(), sign, hours, minutes)
}
