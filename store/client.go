package store

import (
	"compress/zlib"
	"crypto/sha1"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kanon1343/fsegit/object"
	"github.com/kanon1343/fsegit/sha"
	"github.com/kanon1343/fsegit/util"
)

// ErrStopWalk is used as a return value from WalkFunc to indicate that
// the walk should be stopped.
var ErrStopWalk = errors.New("stop walk")

type Client struct {
	rootDir   string
	objectDir string
}

// pathのリポジトリのルートディレクトリを探す
func NewClient(path string) (*Client, error) {
	rootDir, err := util.FindGitRoot(path)
	if err != nil {
		return nil, err
	}
	return &Client{
		rootDir:   rootDir,
		objectDir: filepath.Join(rootDir, ".git", "objects"),
	}, nil
}

// RootDir returns repository root directory path.
func (c *Client) RootDir() string {
	return c.rootDir
}

// hashで指定したobjectを返す
func (c *Client) GetObject(hash sha.SHA1) (*object.Object, error) {
	hashString := hash.String()
	objectPath := filepath.Join(c.objectDir, hashString[:2], hashString[2:])

	objectFile, err := os.Open(objectPath)
	if err != nil {
		return nil, err
	}
	defer objectFile.Close()

	zr, err := zlib.NewReader(objectFile)
	if err != nil {
		return nil, err
	}

	obj, err := object.ReadObject(zr)
	if err != nil {
		return nil, err
	}
	return obj, nil
}

// WriteObject compresses and writes a Git object to the object store and
// returns the calculated SHA1 hash for the stored data.
func (c *Client) WriteObject(objType object.Type, data []byte) (sha.SHA1, error) {
	header := fmt.Sprintf("%s %d\x00", objType, len(data))
	content := append([]byte(header), data...)

	sum := sha1.Sum(content)
	hash := make(sha.SHA1, len(sum))
	copy(hash, sum[:])

	hashString := hash.String()
	dirPath := filepath.Join(c.objectDir, hashString[:2])
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		return nil, err
	}

	objectPath := filepath.Join(dirPath, hashString[2:])

	if _, err := os.Stat(objectPath); err == nil {
		return hash, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	file, err := os.Create(objectPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	zw := zlib.NewWriter(file)
	if _, err := zw.Write(content); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}

	return hash, nil
}

type WalkFunc func(*object.Commit) error

// hashで指定したコミットから履歴を遡ってそれぞれのコミットにwalkFuncを適用する.
func (c *Client) WalkHistory(hash sha.SHA1, walkFunc WalkFunc) error {
	ancestors := []sha.SHA1{hash}
	cycleCheck := map[string]struct{}{}

	// BFS
	for len(ancestors) > 0 {
		currentHash := ancestors[0]
		if _, ok := cycleCheck[string(currentHash)]; ok {
			ancestors = ancestors[1:]
			continue
		}
		cycleCheck[string(currentHash)] = struct{}{}

		obj, err := c.GetObject(currentHash)
		if err != nil {
			return err
		}

		current, err := object.NewCommit(obj)
		if err != nil {
			return err
		}

		if err := walkFunc(current); err != nil {
			if errors.Is(err, ErrStopWalk) {
				return nil // Stop walking, but not an error for the caller
			}
			return err // Actual error
		}

		ancestors = append(ancestors[1:], current.Parents...)
	}

	return nil
}
