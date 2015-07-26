FROM golang:1.4.2

MAINTAINER 'Josh Hawn <jlhawn@docker.com> (github:jlhawn)'

ENV PROJ_DIR /go/src/github.com/jlhawn/dockramp

RUN sh -c 'mkdir -p $PROJ_DIR'

COPY . $PROJ_DIR

RUN sh << EOF
	export GOPATH="$PROJ_DIR/Godeps/_workspace:$GOPATH"
	go build -o /usr/local/bin/dockramp github.com/jlhawn/dockramp/cmd/dockramp
EOF
