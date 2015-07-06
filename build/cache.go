package build

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
)

func (b *Builder) probeCache() bool {
	imageID, cacheHit := b.cache[b.getCacheKey()]
	if !cacheHit {
		return false
	}

	if _, err := b.client.InspectImage(imageID); err != nil {
		return false
	}

	b.imageID = imageID
	b.uncommitted = false
	b.uncommittedCommands = nil

	fmt.Fprintf(b.out, " cache hit ---> %s\n", b.imageID)

	return true
}

func (b *Builder) getCacheKey() string {
	hasher := sha256.New()

	// Note: hash.Hash never returns an error.
	hasher.Write([]byte(b.imageID))

	for _, command := range b.uncommittedCommands {
		hasher.Write([]byte(command))
	}

	return fmt.Sprintf("%x", hasher.Sum(nil))
}

func (b *Builder) setCache(imageID string) error {
	b.cache[b.getCacheKey()] = imageID

	return b.saveCache()
}

func (b *Builder) loadCache() (err error) {
	b.cache = map[string]string{}

	usr, err := user.Current()
	if err != nil {
		return fmt.Errorf("unable to get current user: %s", err)
	}

	cacheFilename := fmt.Sprintf("%s%c%s", usr.HomeDir, filepath.Separator, ".dockrampcache")
	cacheFile, err := os.Open(cacheFilename)
	if os.IsNotExist(err) {
		// No cache file exists to load.
		return nil
	}
	if err != nil {
		return fmt.Errorf("unable to open cache file: %s", err)
	}
	defer func() {
		if closeErr := cacheFile.Close(); err == nil {
			err = closeErr
		}
	}()

	if err := json.NewDecoder(cacheFile).Decode(&b.cache); err != nil {
		return fmt.Errorf("unable to decode build cache: %s", err)
	}

	return nil
}

func (b *Builder) saveCache() (err error) {
	usr, err := user.Current()
	if err != nil {
		return fmt.Errorf("unable to get current user: %s", err)
	}

	cacheFilename := fmt.Sprintf("%s%c%s", usr.HomeDir, filepath.Separator, ".dockrampcache")
	cacheFile, err := os.OpenFile(cacheFilename, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(0600))
	if err != nil {
		return fmt.Errorf("unable to open cache file: %s", err)
	}
	defer func() {
		if closeErr := cacheFile.Close(); err == nil {
			err = closeErr
		}
	}()

	if err := json.NewEncoder(cacheFile).Encode(b.cache); err != nil {
		return fmt.Errorf("unable to encode build cache: %s", err)
	}

	return nil
}
