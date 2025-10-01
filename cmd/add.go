package cmd

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/kanon1343/fsegit/object"
	"github.com/kanon1343/fsegit/store"
	"github.com/spf13/cobra"
)

const defaultFileMode = "100644"

var addCmd = &cobra.Command{
	Use:   "add [file...]",
	Short: "ステージにファイルを追加します",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAdd(args)
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
}

func runAdd(paths []string) error {
	client, err := store.NewClient("./")
	if err != nil {
		return err
	}

	idx, err := store.LoadIndex(client.RootDir())
	if err != nil {
		return err
	}

	for _, p := range paths {
		fullPath := p
		if !filepath.IsAbs(fullPath) {
			fullPath = filepath.Join(client.RootDir(), p)
		}

		info, err := os.Stat(fullPath)
		if err != nil {
			return err
		}

		if info.IsDir() {
			if err := filepath.WalkDir(fullPath, func(path string, d fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if d.IsDir() {
					return nil
				}
				return stageFile(client, idx, path)
			}); err != nil {
				return err
			}
			continue
		}

		if err := stageFile(client, idx, fullPath); err != nil {
			return err
		}
	}

	if err := idx.Save(client.RootDir()); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "added %d file(s)\n", len(idx.Entries))
	return nil
}

func stageFile(client *store.Client, idx *store.Index, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("unsupported file type: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	hash, err := client.WriteObject(object.BlobObject, data)
	if err != nil {
		return err
	}

	rel, err := filepath.Rel(client.RootDir(), path)
	if err != nil {
		return err
	}
	if rel == "." || rel == "" || rel == ".." || strings.HasPrefix(rel, "../") {
		return fmt.Errorf("invalid path to stage: %s", path)
	}

	mode := defaultFileMode
	if info.Mode()&0o111 != 0 {
		mode = "100755"
	}

	idx.Set(rel, hash, mode)
	return nil
}
