FROM golang:1.4.2

RUN mkdir -p /go/src/github.com/jlhawn/dockramp

COPY . /go/src/github.com/jlhawn/dockramp

RUN sh << EOF
	set -ex
	GOPATH="$GOPATH:/go/src/github.com/jlhawn/dockramp/Godeps/_workspace" \
		go build -o /usr/local/bin/dockramp github.com/jlhawn/dockramp/cmd/dockramp
EOF
