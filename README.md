# dockramp

A Client Driven Docker Image Builder

## Plan

- Context Directory
	- Specify with `-C` flag.
	- Default to current directory.
- Dockerfile
	- Specify with `-f, --file` flag.
	- Default to `Dockerfile` in context directory.
- Repository/tag
	- Specify with `-t` flag.
- Parse Dockerfile
	- Already provided by `docker/builder/parser` package.
- Perform all build steps
	- `FROM` must be first step.
		- Special case `scratch` image.
		- Ensure the image exists, pull if it does not.
	- `ADD`, `COPY`, and `RUN` are special.
		- Commit if pending container uncomitted.
		- Create new container from last committed image.
		- Perform the appropriate action.
			- copy in files/dirs for `ADD`, `COPY`.
			- wait for process to complete if `RUN`.
	- All other commands can be grouped together.
		- Everything else is runconfig/metadata.
		- Set internal state, uncommitted=true.
- If uncommitted at the end (due to trailing metadata commands):
	- create container and commit to new image.

