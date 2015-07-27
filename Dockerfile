FROM golang:1.4.2-cross

MAINTAINER 'Josh Hawn <jlhawn@docker.com> (github:jlhawn)'

ENV PROJ_DIR /go/src/github.com/jlhawn/dockramp

ENV PLATFORMS '
	darwin/386   darwin/amd64
	freebsd/386  freebsd/amd64  freebsd/arm
	linux/386    linux/amd64    linux/arm
	windows/386  windows/amd64
'

RUN sh -c 'mkdir -p $PROJ_DIR'

COPY . $PROJ_DIR

RUN sh -c 'cp "$PROJ_DIR/make_binaries.sh" /usr/local/bin/make_binaries.sh'

ENTRYPOINT /usr/local/bin/make_binaries.sh
CMD /bundles
