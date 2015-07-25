package build

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/jlhawn/dockramp/archive"
	"github.com/jlhawn/dockramp/build/commands"
	"github.com/jlhawn/tarsum"
)

func (b *Builder) handleCopy(args []string, heredoc string) error {
	log.Debugf("handling %s with args: %#v", commands.Copy, args)

	if len(args) != 2 {
		return fmt.Errorf("%s requires exactly two arguments", commands.Copy)
	}

	if b.checkCopyCache(args[0]) {
		return nil
	}

	containerID, err := b.createContainer([]string{"/bin/sh", "-c"}, []string{"#(nop)"}, false)
	if err != nil {
		return fmt.Errorf("unable to create container: %s", err)
	}

	if err := b.copyToContainer(args[0], containerID, args[1]); err != nil {
		return fmt.Errorf("unable to copy to container: %s", err)
	}

	b.containerID = containerID

	return nil
}

func (b *Builder) checkCopyCache(srcPath string) bool {
	srcPath = fmt.Sprintf("%s%c%s", b.contextDirectory, filepath.Separator, srcPath)
	srcArchive, err := archive.TarResource(srcPath)
	if err != nil {
		log.Debugf("unable to archive source: %s", err)
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
	b.uncommittedCommands = append(b.uncommittedCommands, fmt.Sprintf("COPY digest: %s", copyDigest))

	return b.probeCache()
}

// containerPathStat is used to encode the response from
// 	GET /containers/{name:.*}/stat-path
type containerPathStat struct {
	Name  string      `json:"name"`
	Path  string      `json:"path"`
	Size  int64       `json:"size"`
	Mode  os.FileMode `json:"mode"`
	Mtime time.Time   `json:"mtime"`
}

func (b *Builder) statContainerPath(container, path string) (*containerPathStat, error) {
	query := make(url.Values, 1)
	query.Set("path", filepath.ToSlash(path)) // Normalize the paths used in the API.

	urlPath := fmt.Sprintf("/containers/%s/archive?%s", container, query.Encode())
	req, err := http.NewRequest("HEAD", b.client.URL.String()+urlPath, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to prepare request: %s", err)
	}

	resp, err := b.client.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("unable to make request: %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed with status code: %d", resp.StatusCode)
	}

	encodedStat := resp.Header.Get("X-Docker-Container-Path-Stat")
	statDecoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(encodedStat))

	var stat containerPathStat
	if err = json.NewDecoder(statDecoder).Decode(&stat); err != nil {
		return nil, fmt.Errorf("unable to decode container path stat header: %s", err)
	}

	return &stat, nil
}

func (b *Builder) copyToContainer(srcPath, dstContainer, dstPath string) (err error) {
	// In order to get the copy behavior right, we need to know information
	// about both the source and destination. The API is a simple tar
	// archive/extract API but we can use the stat info header about the
	// destination to be more informed about exactly what the destination is.

	// Prepare destination copy info by stat-ing the container path.
	dstInfo := archive.CopyInfo{Path: dstPath}
	dstStat, err := b.statContainerPath(dstContainer, dstPath)
	if err == nil {
		dstInfo.Exists, dstInfo.IsDir = true, dstStat.Mode.IsDir()
	}
	// Ignore any other error and assume that the parent directory of the
	// destination path exists, in which case the copy may still succeed. If
	// there is any type of conflict (e.g., non-directory overwriting an
	// existing directory or vice versia) the extraction will fail. If the
	// destination simply did not exist, but the parent directory does, the
	// extraction will still succeed.

	srcPath = fmt.Sprintf("%s%c%s", b.contextDirectory, filepath.Separator, srcPath)

	srcArchive, err := archive.TarResource(srcPath)
	if err != nil {
		return err
	}
	defer srcArchive.Close()

	// With the stat info about the local source as well as the
	// destination, we have enough information to know whether we need to
	// alter the archive that we upload so that when the server extracts
	// it to the specified directory in the container we get the disired
	// copy behavior.

	// Prepare source copy info.
	srcInfo, err := archive.CopyInfoStatPath(srcPath, true)
	if err != nil {
		return err
	}

	// See comments in the implementation of `archive.PrepareArchiveCopy`
	// for exactly what goes into deciding how and whether the source
	// archive needs to be altered for the correct copy behavior when it is
	// extracted. This function also infers from the source and destination
	// info which directory to extract to, which may be the parent of the
	// destination that the user specified.
	dstDir, preparedArchive, err := archive.PrepareArchiveCopy(srcArchive, srcInfo, dstInfo)
	if err != nil {
		return err
	}
	defer preparedArchive.Close()

	dstPath = dstDir

	query := make(url.Values, 1)
	query.Set("path", filepath.ToSlash(dstPath)) // Normalize the paths used in the API.
	// Do not allow for an existing directory to be overwritten by a non-directory and vice versa.
	query.Set("noOverwriteDirNonDir", "true")

	urlPath := fmt.Sprintf("/containers/%s/archive?%s", dstContainer, query.Encode())
	req, err := http.NewRequest("PUT", b.client.URL.String()+urlPath, preparedArchive)
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
