package build

import (
	"fmt"
	"path/filepath"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/jlhawn/dockramp/build/commands"
)

/**************************
 * Unsupported Directives *
 **************************/

func (b *Builder) handleAdd(args []string, heredoc string) error {
	return fmt.Errorf("ADD not yet supported")
}

func (b *Builder) handleOnbuild(args []string, heredoc string) error {
	return fmt.Errorf("ONBUILD not yet supported")
}

/***********************
 * Metadata Directives *
 ***********************/

func (b *Builder) handleCmd(args []string, heredoc string) error {
	log.Debugf("handling %s with args: %#v", commands.Cmd, args)

	b.config.Cmd = args

	return nil
}

func (b *Builder) handleEntrypoint(args []string, heredoc string) error {
	log.Debugf("handling %s with args: %#v", commands.Entrypoint, args)

	b.config.Entrypoint = args

	return nil
}

func (b *Builder) handleEnv(args []string, heredoc string) error {
	log.Debugf("handling %s with args: %#v", commands.Env, args)

	if len(args) != 2 {
		return fmt.Errorf("%s requires exactly two arguments", commands.Env)
	}

	b.config.Env = append(b.config.Env, fmt.Sprintf("%s=%s", args[0], args[1]))

	return nil
}

func (b *Builder) handleExpose(args []string, heredoc string) error {
	log.Debugf("handling %s with args: %#v", commands.Expose, args)

	if len(args) != 1 {
		return fmt.Errorf("%s requires exactly one argument", commands.Expose)
	}

	b.config.ExposedPorts[args[0]] = struct{}{}

	return nil
}

func (b *Builder) handleLabel(args []string, heredoc string) error {
	log.Debugf("handling %s with args: %#v", commands.Label, args)

	if len(args) != 2 {
		return fmt.Errorf("%s requires exactly two arguments", commands.Label)
	}

	b.config.Labels[args[0]] = args[1]

	return nil
}

func (b *Builder) handleMaintainer(args []string, heredoc string) error {
	log.Debugf("handling %s with args: %#v", commands.Maintainer, args)

	if len(args) < 1 {
		return fmt.Errorf("%s requires at least one argument", commands.Maintainer)
	}

	b.maintainer = strings.Join(args, " ")

	return nil
}

func (b *Builder) handleUser(args []string, heredoc string) error {
	log.Debugf("handling %s with args: %#v", commands.User, args)

	if len(args) != 1 {
		return fmt.Errorf("%s requires exactly one argument", commands.User)
	}

	b.config.User = args[0]

	return nil
}

func (b *Builder) handleVolume(args []string, heredoc string) error {
	log.Debugf("handling %s with args: %#v", commands.Volume, args)

	if len(args) == 0 {
		return fmt.Errorf("%s requires at least one argument", commands.Volume)
	}

	for _, arg := range args {
		vol := strings.TrimSpace(arg)
		if vol == "" {
			return fmt.Errorf("volume specified can not be an empty string")
		}

		b.config.Volumes[vol] = struct{}{}
	}

	return nil
}

func (b *Builder) handleWorkdir(args []string, heredoc string) error {
	log.Debugf("handling %s with args: %#v", commands.Workdir, args)

	if len(args) != 1 {
		return fmt.Errorf("%s requires exactly one argument", commands.Workdir)
	}

	workdir := args[0]
	if !filepath.IsAbs(workdir) {
		workdir = filepath.Join("/", b.config.WorkingDir, workdir)
	}

	b.config.WorkingDir = workdir

	return nil
}
