#!/bin/sh

FAILURES=""
TARGET="github.com/jlhawn/dockramp/cmd/dockramp"

mkdir "$1"

for PLATFORM in $PLATFORMS; do
	OUTPUTDIR="$1/$PLATFORM"
	mkdir -p "$OUTPUTDIR"

	export GOPATH="$PROJ_DIR/Godeps/_workspace:$GOPATH"
	export GOOS="${PLATFORM%/*}"
	export GOARCH="${PLATFORM#*/}"
	
	CMD="go build -o $OUTPUTDIR/dockramp $TARGET"
	
	echo "$CMD" && $CMD || FAILURES="$FAILURES $PLATFORM"
done

if [ -n "$FAILURES" ]; then
	echo "*** build FAILED on $FAILURES ***"
	exit 1
fi
