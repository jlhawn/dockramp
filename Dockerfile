FROM "debian:jessie"

MAINTAINER "Josh Hawn <josh.hawn@docker.com>"

COPY . /stuff

# This is a comment
RUN /bin/sh << EOF
	set -ex
	apt-get update
	apt-get install -y python
EOF

ENTRYPOINT /bin/bash -c
CMD "echo 'hello, world!'"

EXPOSE 80
LABEL key "multi-part value"

