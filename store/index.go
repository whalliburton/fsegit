package store

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kanon1343/fsegit/object"
	"github.com/kanon1343/fsegit/sha"
)

// IndexEntry represents a staged file entry.
type IndexEntry struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
	Mode string `json:"mode"`
}

// Index keeps track of files staged for the next commit.
type Index struct {
	Entries map[string]IndexEntry `json:"entries"`
}

// LoadIndex reads the staging index from disk. If the index file does not
// exist an empty index is returned.
func LoadIndex(root string) (*Index, error) {
	indexPath := filepath.Join(root, ".git", "fseindex")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Index{Entries: map[string]IndexEntry{}}, nil
		}
		return nil, err
	}

	idx := &Index{}
	if err := json.Unmarshal(data, idx); err != nil {
		return nil, err
	}
	if idx.Entries == nil {
		idx.Entries = map[string]IndexEntry{}
	}
	return idx, nil
}

// Save writes the current index to disk.
func (i *Index) Save(root string) error {
	if i == nil {
		return nil
	}
	if i.Entries == nil {
		i.Entries = map[string]IndexEntry{}
	}
	data, err := json.MarshalIndent(i, "", "  ")
	if err != nil {
		return err
	}
	indexPath := filepath.Join(root, ".git", "fseindex")
	return os.WriteFile(indexPath, data, 0o644)
}

// Set adds or updates a staged entry.
func (i *Index) Set(path string, hash sha.SHA1, mode string) {
	if i.Entries == nil {
		i.Entries = map[string]IndexEntry{}
	}
	normalized := filepath.ToSlash(path)
	i.Entries[normalized] = IndexEntry{Path: normalized, Hash: hash.String(), Mode: mode}
}

// Clear removes all entries from the index.
func (i *Index) Clear() {
	i.Entries = map[string]IndexEntry{}
}

type treeNode struct {
	files map[string]IndexEntry
	dirs  map[string]*treeNode
}

// BuildTree writes tree objects for the staged files and returns the root tree
// hash.
func (i *Index) BuildTree(client *Client) (sha.SHA1, error) {
	root := &treeNode{files: map[string]IndexEntry{}, dirs: map[string]*treeNode{}}
	for path, entry := range i.Entries {
		parts := strings.Split(path, "/")
		node := root
		for _, part := range parts[:len(parts)-1] {
			if node.dirs == nil {
				node.dirs = map[string]*treeNode{}
			}
			if node.dirs[part] == nil {
				node.dirs[part] = &treeNode{files: map[string]IndexEntry{}, dirs: map[string]*treeNode{}}
			}
			node = node.dirs[part]
		}
		if node.files == nil {
			node.files = map[string]IndexEntry{}
		}
		node.files[parts[len(parts)-1]] = entry
	}

	return writeTree(client, root)
}

type treeEntry struct {
	mode string
	name string
	hash sha.SHA1
}

func writeTree(client *Client, node *treeNode) (sha.SHA1, error) {
	var entries []treeEntry

	for name, child := range node.dirs {
		hash, err := writeTree(client, child)
		if err != nil {
			return nil, err
		}
		entries = append(entries, treeEntry{mode: "40000", name: name, hash: hash})
	}

	for name, file := range node.files {
		hashBytes, err := decodeHash(file.Hash)
		if err != nil {
			return nil, err
		}
		entries = append(entries, treeEntry{mode: file.Mode, name: name, hash: hashBytes})
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })

	var buf bytes.Buffer
	for _, entry := range entries {
		buf.WriteString(entry.mode)
		buf.WriteByte(' ')
		buf.WriteString(entry.name)
		buf.WriteByte(0)
		buf.Write(entry.hash)
	}

	return client.WriteObject(object.TreeObject, buf.Bytes())
}

func decodeHash(s string) (sha.SHA1, error) {
	decoded, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}
	hash := make(sha.SHA1, len(decoded))
	copy(hash, decoded)
	return hash, nil
}
