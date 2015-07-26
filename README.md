# Dockramp

A Client-driven Docker Container Image Builder

## Features

Docker 1.8.0 will introduce a new API endpoint for copying files and
directories into a container. With this addition, anyone can now implement
their own build system using the Docker Remote API. **Dockramp** is the first
proof of concept for an alternative to `docker build`.

- **No context uploads**

  Builds will no longer wait to begin while your client uploads a (sometimes
  large) context directory to the Docker daemon. In **Dockramp**, files and
  directories are only transfered when they need to be: on a `COPY` or
  `EXTRACT` instruction. You'll notice that builds start much faster then they
  ever would have using `docker build`.

- **Efficient layering and caching**

  Most build instructions are only used to specify metadata and other options
  for how to run your container, but `docker build` creates a new filesystem
  layer for each of these instructions - requiring you to wait while a
  potentially expensive filesystem commit is performed and an unnecessary
  filesystem layer is created.

  **Dockramp** differentiates these instructions from others and only performs
  filesystem commits after instructions which typically do modify the
  filesystem: `COPY`, `EXTRACT`, and `RUN`. All other instructions may be
  combined together and expensive commits are only performed when needed.

  Cache lookups are also more efficient: build cache data is stored locally
  rather than on the Docker daemon so that the client can decide what set
  of changes map to a specific image ID. These lookups can be done in constant
  time, while `docker build` cache lookups iterate over all image layers and
  get noticeably slower the more images you have installed.

- **More expressive and extensible Dockerfile syntax**

  The existing Dockerfile syntax has served many developers well for over 2
  years, but is also limitting to those who wish to have more options for
  some build instructions.

  **Dockramp** rethinks some of the design decisions of Docker's Dockerfile
  syntax and offers a more familiar way of scripting a build for anyone who has
  ever written a shell script: no more cascading line continuations or
  alternative JSON syntax, just simple space separated or quoted arguments and
  even [heredoc](https://en.wikipedia.org/wiki/Here_document) support for more
  complex `RUN` instructions.

  See the **Dockerfile Syntax** section below for examples and notable
  differences from Docker's Dockerfile syntax.

## Usage

Invoking `dockramp` with no other arguments will use the current directory as
the build context and use the file named "Dockerfile" in the current directory
for build intstructions.

```bash
$ # Executes the Dockerfile in this repository.
$ dockramp 
Step 0: FROM golang:1.4.2
Step 1: RUN mkdir -p /go/src/github.com/jlhawn/dockramp
 ---> b49a5a6950c7800683009cbd2675f06b7206cddfa6650559070854b47d777bee
Step 2: COPY . /go/src/github.com/jlhawn/dockramp
 ---> 0fc2aa6107083295be07b7d3e529ca5b7b7363294c4f4ffb515f69f1c1dbe80f
Step 3: RUN sh
Input:
	set -ex
	GOPATH="$GOPATH:/go/src/github.com/jlhawn/dockramp/Godeps/_workspace" \
		go build -o /usr/local/bin/dockramp github.com/jlhawn/dockramp/cmd/dockramp

+ GOPATH=/go:/go/src/github.com/jlhawn/dockramp/Godeps/_workspace go build -o /usr/local/bin/dockramp github.com/jlhawn/dockramp/cmd/dockramp
 ---> 3962de550c8044986a33f9da1f6936d6d2912676208b1207a62da859e389e629
Successfully built 3962de550c8044986a33f9da1f6936d6d2912676208b1207a62da859e389e629
```

You can use the `-C` flag to specify a directory to use as the build context.
You can also specify any Dockerfile with the `-f` flag (this file *does not*
need to be within the context directory!).

`dockramp` also supports many of the standard options used by `docker` and uses
many of the same environment variables and configuration files used by `docker`
as well. Here is the full list of currently supported arguments:

```bash
$ dockramp --help
Usage of dockramp:
  --cacert="": Trust certs signed only by this CA
  --cert="": TLS client certificate
  --key="": TLS client key
  --tls=false: Use TLS client cert/key (implied by --tlsverify)
  --tlsverify=true: Use TLS and verify the remote server certificate
  -C=".": Build context directory
  -H="": Docker daemon socket/host to connect to
  -d=false: enable debug output
  -f="": Path to Dockerfile
  -t="": Repository name (and optionally a tag) for the image
```

## Dockerfile Syntax

> TODO (jlhawn): write this next.

## TODO

- Handle `.dockerignore`
- Implement various options (via flags) to many Dockerfile instructions.
