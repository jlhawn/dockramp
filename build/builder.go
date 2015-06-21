package build

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/jlhawn/dockramp/build/commands"
	"github.com/jlhawn/dockramp/build/parser"
	"github.com/jlhawn/dockramp/build/util"
	"github.com/samalba/dockerclient"
)

type config struct {
	Cmd          []string
	Entrypoint   []string
	Env          []string
	ExposedPorts map[string]struct{}
	Labels       map[string]string
	User         string
	Volumes      map[string]struct{}
	WorkingDir   string
}

func (c *config) toDocker() *dockerclient.ContainerConfig {
	return &dockerclient.ContainerConfig{
		User:         c.User,
		ExposedPorts: c.ExposedPorts,
		Env:          c.Env,
		Cmd:          c.Cmd,
		Volumes:      c.Volumes,
		WorkingDir:   c.WorkingDir,
		Entrypoint:   c.Entrypoint,
		Labels:       c.Labels,
	}
}

type handlerFunc func(args []string, heredoc string) error

// Builder is able to build docker images from a local context directory, a
// Dockerfile, and a docker client connection.
type Builder struct {
	daemonURL        string
	tlsConfig        *tls.Config
	client           *dockerclient.DockerClient
	contextDirectory string
	dockerfilePath   string
	repo, tag        string

	out io.Writer

	config              *config
	maintainer          string
	imageID             string
	containerID         string
	uncommitted         bool
	uncommittedCommands []string

	handlers map[string]handlerFunc
}

// NewBuilder creates a new builder.
func NewBuilder(daemonURL string, client *dockerclient.DockerClient, tlsConfig *tls.Config, contextDirectory, dockerfilePath, repoTag string) (*Builder, error) {
	// Validate that the context directory exists.
	stat, err := os.Stat(contextDirectory)
	if err != nil {
		return nil, fmt.Errorf("unable to access build context directory: %s", err)
	}
	if !stat.IsDir() {
		return nil, fmt.Errorf("context must be a directory")
	}

	if dockerfilePath == "" {
		// Use Default path.
		dockerfilePath = filepath.Join(contextDirectory, "Dockerfile")
	}

	if _, err := os.Stat(dockerfilePath); err != nil {
		return nil, fmt.Errorf("unable to access build file: %s", err)
	}

	// Validate the repository and tag.
	repo, tag := util.ParseRepositoryTag(repoTag)
	if repo != "" {
		if err := util.ValidateRepositoryName(repo); err != nil {
			return nil, fmt.Errorf("invalid repository name: %s", err)
		}
		if tag != "" {
			if err := util.ValidateTagName(tag); err != nil {
				return nil, fmt.Errorf("invalid tag: %s", err)
			}
		}
	}

	b := &Builder{
		daemonURL:        daemonURL,
		tlsConfig:        tlsConfig,
		client:           client,
		contextDirectory: contextDirectory,
		dockerfilePath:   dockerfilePath,
		repo:             repo,
		tag:              tag,
		out:              os.Stdout,
		config: &config{
			Labels:       map[string]string{},
			ExposedPorts: map[string]struct{}{},
			Volumes:      map[string]struct{}{},
		},
	}

	// Register Dockerfile Directive Handlers
	b.handlers = map[string]handlerFunc{
		commands.Cmd:        b.handleCmd,
		commands.Copy:       b.handleCopy,
		commands.Entrypoint: b.handleEntrypoint,
		commands.Env:        b.handleEnv,
		commands.Expose:     b.handleExpose,
		commands.Extract:    b.handleExtract,
		commands.From:       b.handleFrom,
		commands.Label:      b.handleLabel,
		commands.Maintainer: b.handleMaintainer,
		commands.Run:        b.handleRun,
		commands.User:       b.handleUser,
		commands.Volume:     b.handleVolume,
		commands.Workdir:    b.handleWorkdir,

		// Not implemented for now:
		commands.Add:     b.handleAdd,
		commands.Onbuild: b.handleOnbuild,
	}

	return b, nil
}

// Run executes the build process.
func (b *Builder) Run() error {
	// Parse the Dockerfile.
	dockerfile, err := os.Open(b.dockerfilePath)
	if err != nil {
		return fmt.Errorf("unable to open Dockerfile: %s", err)
	}
	defer dockerfile.Close()

	commands, err := parser.Parse(dockerfile)
	if err != nil {
		return fmt.Errorf("unable to parse Dockerfile: %s", err)
	}

	if len(commands) == 0 {
		return fmt.Errorf("no commands found in Dockerfile")
	}

	for i, command := range commands {
		if err := b.dispatch(i, command); err != nil {
			return err
		}
	}

	// create container and commit if we need to (because of trailing
	// metadata directives).
	if b.uncommitted && !b.probeCache() {

		b.containerID, err = b.createContainer([]string{"/bin/sh", "-c"}, []string{"#(nop)"}, false)
		if err != nil {
			return fmt.Errorf("unable to create container: %s", err)
		}

		if err := b.commit(); err != nil {
			return fmt.Errorf("unable to commit container image: %s", err)
		}
	}

	imageName := b.imageID
	if b.repo != "" {
		if err := b.setTag(imageName, b.repo, b.tag); err != nil {
			return fmt.Errorf("unable to tag built image: %s", err)
		}

		imageName = b.repo
		if b.tag != "" {
			imageName = fmt.Sprintf("%s:%s", imageName, b.tag)
		}
	}

	fmt.Fprintf(b.out, "Successfully built %s\n", imageName)

	return nil
}

func (b *Builder) dispatch(stepNum int, command *parser.Command) error {
	commandStr := strings.Join(command.Args, " ")

	cmd, args := strings.ToUpper(command.Args[0]), command.Args[1:]

	// FROM must be the first and only the first command.
	if (stepNum == 0) != (cmd == commands.From) {
		return fmt.Errorf("FROM must be the first Dockerfile command")
	}

	handler, exists := b.handlers[cmd]
	if !exists {
		return fmt.Errorf("unknown command: %q", cmd)
	}

	// Print the current step.
	fmt.Fprintf(b.out, "Step %d: %s\n", stepNum, commandStr)

	if _, ok := commands.ReplaceEnvAllowed[cmd]; ok {
		// Expand environment variables in the arguments.
		for i, arg := range args {
			arg, err := processShellWord(arg, b.config.Env)
			if err != nil {
				return err
			}

			args[i] = arg
		}
	}

	b.uncommitted = true
	b.uncommittedCommands = append(b.uncommittedCommands, commandStr)

	if err := handler(args, command.Heredoc); err != nil {
		return err
	}

	// We may not need to commit now but we should if the current command may
	// have modified the filesystem. `b.uncommitted` will be set back to false
	// if there was a cache hit.
	if _, needCommit := commands.FilesystemModifierCommands[cmd]; needCommit && b.uncommitted {
		if err := b.commit(); err != nil {
			return fmt.Errorf("unable to commit container image: %s", err)
		}
	}

	return nil
}

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

	cacheRepo, cacheTag := b.getCachedImageName()
	if err := b.setTag(commitResponse.ID, cacheRepo, cacheTag); err != nil {
		return fmt.Errorf("unable to tag committed image: %s", err)
	}

	b.imageID = commitResponse.ID

	fmt.Fprintf(b.out, " ---> %s\n", b.imageID)

	b.uncommitted = false
	b.uncommittedCommands = nil
	b.containerID = ""

	return nil
}
