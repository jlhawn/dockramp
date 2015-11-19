package build

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/jlhawn/dockramp/build/commands"
)

func (b *Builder) handleRun(args []string, heredoc string) error {
	log.Debugf("handling %s with args: %#v", commands.Run, args)

	if len(args) < 1 {
		return fmt.Errorf("%s requires at least one argument", commands.Run)
	}

	if heredoc != "" {
		fmt.Fprintf(b.out, "Input:\n%s\n", heredoc)
		b.uncommittedCommands = append(b.uncommittedCommands, fmt.Sprintf("RUN input: %q", heredoc))
	}

	if b.probeCache() {
		return nil
	}

	containerID, err := b.createContainer(args[:1], args[1:], true)
	if err != nil {
		return fmt.Errorf("unable to create container: %s", err)
	}

	errC, err := b.attachContainer(containerID, strings.NewReader(heredoc))
	if err != nil {
		return fmt.Errorf("unable to attach to container: %s", err)
	}

	if err := b.client.StartContainer(containerID, nil); err != nil {
		return fmt.Errorf("unable to start container: %s", err)
	}

	// Wait for the container hijack to end.
	if err := <-errC; err != nil {
		return fmt.Errorf("unable to end hijack stream: %s", err)
	}

	if err := b.client.StopContainer(containerID, 1); err != nil {
		return fmt.Errorf("unable to stop/kill container: %s", err)
	}

	info, err := b.client.InspectContainer(containerID)
	if err != nil {
		return fmt.Errorf("unable to inspect container: %s", err)
	}

	if info.State.ExitCode != 0 {
		return fmt.Errorf("non-zero exit code: %d", info.State.ExitCode)
	}

	b.containerID = containerID

	return nil
}

func (b *Builder) createContainer(entryPoint, cmd []string, openStdin bool) (containerID string, err error) {
	config := b.config.toDocker()
	config.Entrypoint = entryPoint
	config.Cmd = cmd
	config.Image = b.imageID
	config.OpenStdin = openStdin
	config.StdinOnce = openStdin

	return b.client.CreateContainer(config, "", nil)
}

func (b *Builder) attachContainer(container string, input io.Reader) (chan error, error) {
	query := make(url.Values, 4)
	query.Set("stream", "true")
	query.Set("stdin", "true")
	query.Set("stdout", "true")
	query.Set("stderr", "true")

	urlPath := fmt.Sprintf("/containers/%s/attach?%s", container, query.Encode())

	hijackStarted := make(chan int, 1)
	hijackErr := make(chan error, 1)

	// The output from /attach will be a multiplexed stream of stdout and
	// stderr. We need to use a pipe to copy this output into a stdcopy
	// de-multiplexer and into the build output.
	pipeReader, pipeWriter := io.Pipe()
	go func() {
		defer pipeReader.Close()
		stdcopy.StdCopy(b.out, b.out, pipeReader)
	}()

	go func() {
		hijackErr <- b.hijack("POST", urlPath, input, pipeWriter, hijackStarted)
	}()

	// Wait for the hijack to succeeed or fail.
	select {
	case <-hijackStarted:
		return hijackErr, nil
	case err := <-hijackErr:
		return nil, fmt.Errorf("unable to hijack attach tcp stream: %s", err)
	}
}
