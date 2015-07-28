# Dockramp

A Client-driven Docker Container Image Builder

![truck-ramp](https://cloud.githubusercontent.com/assets/1441929/8937404/0371f5da-350d-11e5-9636-f0a8e3dae620.jpg)

## Features

Docker 1.8.0 will introduce a new API endpoint for copying files and
directories into a container. With this addition, anyone can now implement
their own build system using the Docker Remote API. **Dockramp** is the first
proof of concept for an alternative to `docker build`.

- **No context uploads**

  Builds will no longer wait to begin while your client uploads a (sometimes
  large) context directory to the Docker daemon. In **Dockramp**, files and
  directories are only transfered when they need to be: on a `COPY` or
  `EXTRACT` instruction. You'll notice that builds start much faster than they
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

## Installation

Visit the [release page](https://github.com/jlhawn/dockramp/releases) to get
a binary for the latest version on your OS/Arch. If you'd rather compile it
yourself (like I would), you just need `git` and `go`:

```bash
project_dir="$HOME/dockramp/src/github.com/jlhawn/dockramp"
mkdir -p $project_dir
git clone https://github.com/jlhawn/dockramp.git $project_dir
GOPATH="$project_dir/Godeps/_workspace:$HOME/dockramp" go build -o /usr/local/bin/dockramp github.com/jlhawn/dockramp/cmd/dockramp
```

## Usage

Invoking `dockramp` with no other arguments will use the current directory as
the build context and use the file named "Dockerfile" in the current directory
for build intstructions.

```bash
$ # Executes the Dockerfile in this repository.
$ dockramp 
Step 0: FROM golang:1.4.2
Step 1: MAINTAINER "Josh Hawn <jlhawn@docker.com> (github:jlhawn)"
Step 2: ENV PROJ_DIR /go/src/github.com/jlhawn/dockramp
Step 3: RUN sh -c "mkdir -p $PROJ_DIR"
 ---> 0744c1be2f5fb40c355bace595171c180f2a7b2d19ec15e0c92da2fa0c1d7198
Step 4: COPY . /go/src/github.com/jlhawn/dockramp
 ---> 8a40aa285e0ecc70ba5361085ef2faf0e755a8dbf7f2224b377c5c0035dc22a5
Step 5: RUN sh
Input:
	export GOPATH="$PROJ_DIR/Godeps/_workspace:$GOPATH"
	go build -o /usr/local/bin/dockramp github.com/jlhawn/dockramp/cmd/dockramp

 ---> 029e66e2587118f5f6c5176da65ffbde3b501b25136a637c3d700ee369104374
Successfully built 029e66e2587118f5f6c5176da65ffbde3b501b25136a637c3d700ee369104374
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

While the original Dockerfile parser used by `docker build` simply scans for
complete (unescaped) lines and parses each instruction arbitrarily based on the
command, the Dockerfile parser in Dockramp is a more traditional parser which
processes tokens from the input Dockerfile into instructions of positional
arguments.

The Dockerfile syntax used by Dockramp aims to be simple and familiar: a subset
of unix shell syntax. Each instruction has a name, optional flag arguments, and
positional arguments.

### Tokens

- **Whitespace**

  Unquoted whitespace is ignored, but is used to delimit arguments to
  instructions. Whitespace includes: literal space (` `), form feed (`\f`),
  carriage return (`\r`), horizontal tab (`\t`), vertical tab (`\v`), or a
  newline (`\n`) character escaped using a backslash (`\`).

- **Comments**

  Comments are specified using a hash or pound (`#`) character and cause the
  remainder of that line to be ignored.

- **Newlines**

  A newline character (`\n`) is used to end an instruction. Empty lines and
  lines containing only whitespace are ignored.

- **Arguments**

  Instructions are whitespace delimited and arguments can can be single or
  double quoted strings allowing for arguments which contain whitespace.

  - **Single Quoted String**

    A single quoted string is evaluated to be the literal value of the sequence
    of characters between two single quotes (`'`). Any character (including a
    newline) that is not a single quote may be included in the string. No
    escape processing is done on single quoted strings.

  - **Double Quoted String**

    A double quoted string is evaluated to be the value of the sequence of
    characters between two double quotes (`"`) with some escape sequence
    processing. Any character (including a newline) that is not a double quote
    (unless it is escaped first as in `\"`) may appear between the double
    quotes. An escaped double quote character is replaced with a double quote.
    An escaped backslash is replaced with a single backslash. An escaped
    newline is replaced with nothing. No other escape processing is done.

  - **Raw Argument**

    A raw argument is a sequence of one or more characters that are not already
    specified above meaning no hash (`#`), single (`'`) or double (`"`) quote
    character, `<` (used by heredocs), and any whitespace character. However,
    any of these characters may be escaped by a backslash `\` character.
    Escaped sequences are replaced with the escaped character, except for
    escaped newlines which are replaced with nothing.

  Quoted tokens that are not separated by whitespace are joined into a single
  argument value. For example, `hel"lo wo"'rld!'` is evaluated into a single
  argument `hello world!`.

- **Heredocs**

  Some instructions, such as `RUN`, can accept an input stream in the form of
  a [heredoc](https://en.wikipedia.org/wiki/Here_document). A heredoc is
  specified as the last argument to an instruction in the form `<<` and
  followed immediately by an alphanumeric delimiting term such as `<< EOF` or
  `<< END`. This opens the heredoc.

  The following lines will contain the literal text to be used as input to the
  instruction. If the heredoc was opened using `<<-` rather than `<<` then
  leading tab characters of each line will be ignored, allowing for indentation
  of a heredoc without changing its value.

  A heredoc is closed by the delimiting term appearing alone on its own line
  (no leading or trailing whitespace).

### Instructions

All instruction names are case insensitive, i.e, `RUN` and `run` are considered
equivalent. However, all-caps is still the preferred form.

- **`ADD`**

  Not supported. For extracting a tar archive to a directory in the container
  use the `EXTRACT` instruction instead. Downloading a resource from a URL into
  the container is planned to be supported soon via some other instruction.

- **`CMD`**

  Provide default arguments to a container's Entrypoint.

  ```
  CMD arg ...
  ```

  - Accepts zero or more arguments.

- **`COPY`**

  Copy a file or directory from the build context to a location in the
  container.

  ```
  COPY source destination
  ```

  - Requires exactly 2 arguments.
  - `source` is relative to the build context directory.
  - `destination` is an absolute path in the container.

- **`ENTRYPOINT`**

  Specify the file to execute and any permanent arguments when running the
  container.

  ```
  ENTRYPOINT arg ...
  ```

  - Accepts zero or more arguments.

- **`ENV`**

  Set an environment variable for when the container runs.

  ```
  ENV key value
  ```

  - Requires exactly 2 arguments.

- **`EXPOSE`**

  Expose a tcp or udp port in the container to the network.

  ```
  EXPOSE portspec
  ```

  - Requires exactly 1 argument.

- **`EXTRACT`**

  Extract a tar archive to a directory inside the container.

  ```
  EXTRACT source destination
  ```

  - Requires exactly 2 arguments.
  - `source` is relative to the build context directory.
  - `destination` is an absolute path in the container and must be an existing
    directory.

- **`FROM`**

  Use the specified image as the base container image to build from.

  ```
  FROM imagespec
  ```

  - Requires exactly 1 argument.
  - `scratch` is used to indicate that the build should start with an empty
    container filesystem.

- **`LABEL`**

  Add metadata to the container image.

  ```
  LABEL key value
  ```

  - Requires exactly 2 arguments.

- **`MAINTAINER`**

  Specify the author of the resulting container image.

  ```
  MAINTAINER name_and_info
  ```

  - Requires exactly 1 argument.

- **`ONBUILD`**

  Not supported.

- **`RUN`**

  Execute a command inside of a container.

  ```
  RUN arg ...
  ```

  - Requires at least 1 argument.
  - Can use a heredoc to specify `stdin` to the command.

- **`USER`**

  Set the username or UID to use when running the container.

  ```
  USER userspec
  ```

  - Requires exactly 1 argument.

- **`VOLUME`**

  Create a mount point inside the container.

  ```
  VOLUME path
  ```

  - Requires exactly 1 argument.

- **`WORKDIR`**

  Set the working directory for the container.

  ```
  WORKDIR directory
  ```

  - Requires exactly 1 argument.
  - `directory` is an absolute path inside the container.


## TODO

- Handle `.dockerignore`.
- Resolve tag references to digest references using Notary before image pulls.
- Implement various options (via flags) to many Dockerfile instructions.
