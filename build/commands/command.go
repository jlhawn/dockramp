// Package commands contains the set of Dockerfile commands.
package commands

// List of Dockerfile commands.
const (
	Add        = "ADD"
	Cmd        = "CMD"
	Copy       = "COPY"
	Entrypoint = "ENTRYPOINT"
	Env        = "ENV"
	Expose     = "EXPOSE"
	Extract    = "EXTRACT"
	From       = "FROM"
	Label      = "LABEL"
	Maintainer = "MAINTAINER"
	Onbuild    = "ONBUILD"
	Run        = "RUN"
	User       = "USER"
	Volume     = "VOLUME"
	Workdir    = "WORKDIR"
)

// Commands is a set of all Dockerfile commands.
var Commands = map[string]struct{}{
	Add:        {},
	Cmd:        {},
	Copy:       {},
	Entrypoint: {},
	Env:        {},
	Expose:     {},
	Extract:    {},
	From:       {},
	Label:      {},
	Maintainer: {},
	Onbuild:    {},
	Run:        {},
	User:       {},
	Volume:     {},
	Workdir:    {},
}

// FilesystemModifierCommands is a subset of commands that typically modify the
// filesystem of a container and require a commit.
var FilesystemModifierCommands = map[string]struct{}{
	Add:     {},
	Copy:    {},
	Extract: {},
	Run:     {},
}

// ReplaceEnvAllowed is a subset of commands for which environment variable
// interpolation will happen.
var ReplaceEnvAllowed = map[string]struct{}{
	Add:     {},
	Copy:    {},
	Env:     {},
	Expose:  {},
	Extract: {},
	Label:   {},
	User:    {},
	Volume:  {},
	Workdir: {},
}
