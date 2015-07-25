package build

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	log "github.com/Sirupsen/logrus"
)

type containerCommitResponse struct {
	ID string `json:"Id"`
}

func (b *Builder) commit() error {
	log.Debugf("committing container: %s", b.containerID)

	if b.containerID == "" {
		return fmt.Errorf("no container to commit")
	}

	// Encode the uncommited commands as a JSON array to use as a comment.
	comment, err := json.Marshal(b.uncommittedCommands)
	if err != nil {
		return fmt.Errorf("unable to encode comment for commit: %s", err)
	}

	query := make(url.Values, 2)
	query.Set("container", b.containerID)
	query.Set("author", b.maintainer)
	query.Set("comment", string(comment))

	data, err := json.Marshal(b.config.toDocker())
	if err != nil {
		return fmt.Errorf("unable to encode config: %s", err)
	}

	path := fmt.Sprintf("/commit?%s", query.Encode())
	req, err := http.NewRequest("POST", b.client.URL.String()+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("unable to prepare request: %s", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("unable to make request: %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		// Read the body if possible.
		buf := bytes.NewBuffer(make([]byte, 0, resp.ContentLength))
		io.Copy(buf, resp.Body) // It's okay if this fails.

		return fmt.Errorf("request failed with status code %d: %s", resp.StatusCode, buf.String())
	}

	var commitResponse containerCommitResponse
	if err := json.NewDecoder(resp.Body).Decode(&commitResponse); err != nil {
		return fmt.Errorf("unable to decode commit response: %s", err)
	}

	if err := b.client.RemoveContainer(b.containerID, true, true); err != nil {
		return fmt.Errorf("unable to remove container: %s", err)
	}

	if err := b.setCache(commitResponse.ID); err != nil {
		return fmt.Errorf("unable to cache commited image: %s", err)
	}

	b.imageID = commitResponse.ID

	fmt.Fprintf(b.out, " ---> %s\n", b.imageID)

	b.uncommitted = false
	b.uncommittedCommands = nil
	b.containerID = ""

	return nil
}
