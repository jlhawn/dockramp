package build

import (
	"fmt"

	log "github.com/Sirupsen/logrus"
	"github.com/jlhawn/dockramp/build/commands"
	"github.com/samalba/dockerclient"
)

const (
	fromScratch    = "scratch"
	defaultPathEnv = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
)

func (b *Builder) handleFrom(args []string, heredoc string) error {
	log.Debugf("handling %s with args: %#v", commands.From, args)

	if len(args) != 1 {
		return fmt.Errorf("%s requires exactly one argument", commands.From)
	}

	imageName := args[0]

	if imageName == fromScratch {
		log.Debugf("building image from scratch")

		b.imageID = ""
		b.mergeConfig(nil)

		return nil
	}

	// See if it already exists.
	info, err := b.client.InspectImage(imageName)
	if err == nil {
		b.imageID = info.Id
		b.mergeConfig(info.Config)

		log.Debugf("got image ID: %s", b.imageID)

		return nil
	}

	if err != dockerclient.ErrNotFound {
		fmt.Errorf("unable to inspect image: %s", err)
	}

	// Need to pull the image.
	fmt.Fprintln(b.out, "pulling image ...")
	if err := b.client.PullImage(imageName, nil); err != nil {
		return fmt.Errorf("unable to pull image: %s", err)
	}

	// Inspect to get the ID.
	info, err = b.client.InspectImage(imageName)
	if err != nil {
		return fmt.Errorf("unable to inspect image: %s", err)
	}

	b.imageID = info.Id
	b.mergeConfig(info.Config)

	return nil
}

func (b *Builder) mergeConfig(config *dockerclient.ContainerConfig) {
	if config != nil {
		b.config.User = config.User
		b.config.ExposedPorts = config.ExposedPorts
		b.config.Env = config.Env
		b.config.Cmd = config.Cmd
		b.config.Volumes = config.Volumes
		b.config.WorkingDir = config.WorkingDir
		b.config.Entrypoint = config.Entrypoint
		b.config.Labels = config.Labels
	}

	if b.config.ExposedPorts == nil {
		b.config.ExposedPorts = map[string]struct{}{}
	}

	if b.config.Volumes == nil {
		b.config.Volumes = map[string]struct{}{}
	}

	if b.config.Labels == nil {
		b.config.Labels = map[string]string{}
	}

	if len(b.config.Env) == 0 {
		b.config.Env = []string{"PATH=" + defaultPathEnv}
	}
}
