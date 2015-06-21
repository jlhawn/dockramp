package build

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	log "github.com/Sirupsen/logrus"
	"github.com/jlhawn/dockramp/build/commands"
	"github.com/jlhawn/tarsum"
)

func (b *Builder) handleExtract(args []string, heredoc string) error {
	log.Debugf("handling %s with args: %#v", commands.Extract, args)

	if len(args) != 2 {
		return fmt.Errorf("%s requires exactly two arguments", commands.Extract)
	}

	if b.checkExtractCache(args[0]) {
		return nil
	}

	containerID, err := b.createContainer([]string{"/bin/sh", "-c"}, []string{"#(nop)"}, false)
	if err != nil {
		return fmt.Errorf("unable to create container: %s", err)
	}

	if err := b.extractToContainer(args[0], containerID, args[1]); err != nil {
		return fmt.Errorf("unable to copy to container: %s", err)
	}

	b.containerID = containerID

	return nil
}

func (b *Builder) checkExtractCache(srcPath string) bool {
	srcPath = fmt.Sprintf("%s%c%s", b.contextDirectory, filepath.Separator, srcPath)

	srcArchive, err := os.Open(srcPath)
	if err != nil {
		log.Debugf("unable to open source archive: %s", err)
		return false
	}
	defer srcArchive.Close()

	digester, err := tarsum.NewDigest(tarsum.Version1)
	if err != nil {
		log.Debugf("unable to get new tarsum digester: %s", err)
		return false
	}

	if _, err := io.Copy(digester, srcArchive); err != nil {
		log.Debugf("unable to digest source archive: %s", err)
		return false
	}

	copyDigest := fmt.Sprintf("%x", digester.Sum(nil))
	b.uncommittedCommands = append(b.uncommittedCommands, fmt.Sprintf("EXTRACT digest: %s", copyDigest))

	return b.probeCache()
}

func (b *Builder) extractToContainer(srcPath, dstContainer, dstDir string) (err error) {
	srcPath = fmt.Sprintf("%s%c%s", b.contextDirectory, filepath.Separator, srcPath)

	srcArchive, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("unable to open source archive: %s", err)
	}
	defer srcArchive.Close()

	query := make(url.Values, 1)
	query.Set("path", filepath.ToSlash(dstDir)) // Normalize the paths used in the API.

	urlPath := fmt.Sprintf("/containers/%s/extract-to-dir?%s", dstContainer, query.Encode())
	req, err := http.NewRequest("PUT", b.client.URL.String()+urlPath, srcArchive)
	if err != nil {
		return fmt.Errorf("unable to prepare request: %s", err)
	}

	req.Header.Set("Content-Type", "application/x-tar")

	resp, err := b.client.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("unable to make request: %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read the body if possible.
		buf := bytes.NewBuffer(make([]byte, 0, resp.ContentLength))
		io.Copy(buf, resp.Body) // It's okay if this fails.

		return fmt.Errorf("request failed with status code %d: %s", resp.StatusCode, buf.String())
	}

	return nil
}
