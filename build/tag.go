package build

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

func (b *Builder) setTag(imgID, repo, tag string) error {
	query := make(url.Values, 3)
	query.Set("repo", repo)
	query.Set("tag", tag)
	query.Set("force", "1")

	urlPath := fmt.Sprintf("/images/%s/tag?%s", imgID, query.Encode())
	req, err := http.NewRequest("POST", b.client.URL.String()+urlPath, nil)
	if err != nil {
		return fmt.Errorf("unable to prepare request: %s", err)
	}

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

	return nil
}
