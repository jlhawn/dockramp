package util

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/docker/distribution/registry/api/v2"
)

// ParseRepositoryTag parses the given name into a reposName + tag|digest
// The tag can be confusing because of a port in a repository name.
//     Ex: localhost.localdomain:5000/samalba/hipache:latest
//     Digest ex: localhost:5000/foo/bar@sha256:bc8813ea7b3603864987522f02a76101c17ad122e1c46d790efc0fca78ca7bfb
func ParseRepositoryTag(repos string) (string, string) {
	n := strings.Index(repos, "@")
	if n >= 0 {
		parts := strings.Split(repos, "@")
		return parts[0], parts[1]
	}
	n = strings.LastIndex(repos, ":")
	if n < 0 {
		return repos, ""
	}
	if tag := repos[n+1:]; !strings.Contains(tag, "/") {
		return repos[:n], tag
	}
	return repos, ""
}

var (
	validTagName = regexp.MustCompile(`^[\w][\w.-]{0,127}$`)
)

// ValidateTagName validates the name of a tag
func ValidateTagName(name string) error {
	if name == "" {
		return fmt.Errorf("tag name can't be empty")
	}
	if !validTagName.MatchString(name) {
		return fmt.Errorf("illegal tag name (%s): only [A-Za-z0-9_.-] are allowed, minimum 1, maximum 128 in length", name)
	}
	return nil
}

func validateNoSchema(reposName string) error {
	if strings.Contains(reposName, "://") {
		// It cannot contain a scheme!
		return fmt.Errorf("invalid repository name")
	}
	return nil
}

// splitReposName breaks a reposName into an index name and remote name
func splitReposName(reposName string) (string, string) {
	nameParts := strings.SplitN(reposName, "/", 2)
	var indexName, remoteName string
	if len(nameParts) == 1 || (!strings.Contains(nameParts[0], ".") &&
		!strings.Contains(nameParts[0], ":") && nameParts[0] != "localhost") {
		// This is a Docker Index repos (ex: samalba/hipache or ubuntu)
		indexName = ""
		remoteName = reposName
	} else {
		indexName = nameParts[0]
		remoteName = nameParts[1]
	}
	return indexName, remoteName
}

// ValidateRepositoryName validates a repository name.
func ValidateRepositoryName(reposName string) error {
	if err := validateNoSchema(reposName); err != nil {
		return err
	}

	_, remoteName := splitReposName(reposName)

	return v2.ValidateRepositoryName(remoteName)
}
