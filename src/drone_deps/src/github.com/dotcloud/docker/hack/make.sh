#!/usr/bin/env bash
set -e

# This script builds various binary artifacts from a checkout of the docker
# source code.
#
# Requirements:
# - The current directory should be a checkout of the docker source code
#   (http://github.com/dotcloud/docker). Whatever version is checked out
#   will be built.
# - The VERSION file, at the root of the repository, should exist, and
#   will be used as Docker binary version and package version.
# - The hash of the git commit will also be included in the Docker binary,
#   with the suffix -dirty if the repository isn't clean.
# - The script is intented to be run inside the docker container specified
#   in the Dockerfile at the root of the source. In other words:
#   DO NOT CALL THIS SCRIPT DIRECTLY.
# - The right way to call this script is to invoke "make" from
#   your checkout of the Docker repository.
#   the Makefile will do a "docker build -t docker ." and then
#   "docker run hack/make.sh" in the resulting container image.
#

set -o pipefail

# We're a nice, sexy, little shell script, and people might try to run us;
# but really, they shouldn't. We want to be in a container!
if [ "$(pwd)" != '/go/src/github.com/dotcloud/docker' ] || [ -z "$DOCKER_CROSSPLATFORMS" ]; then
	{
		echo "# WARNING! I don't seem to be running in the Docker container."
		echo "# The result of this command might be an incorrect build, and will not be"
		echo "#   officially supported."
		echo "#"
		echo "# Try this instead: make all"
		echo "#"
	} >&2
fi

echo

# List of bundles to create when no argument is passed
DEFAULT_BUNDLES=(
	binary
	test
	test-integration
	dynbinary
	dyntest
	dyntest-integration
	cover
	cross
	tgz
	ubuntu
)

VERSION=$(cat ./VERSION)
if command -v git &> /dev/null && git rev-parse &> /dev/null; then
	GITCOMMIT=$(git rev-parse --short HEAD)
	if [ -n "$(git status --porcelain --untracked-files=no)" ]; then
		GITCOMMIT="$GITCOMMIT-dirty"
	fi
elif [ "$DOCKER_GITCOMMIT" ]; then
	GITCOMMIT="$DOCKER_GITCOMMIT"
else
	echo >&2 'error: .git directory missing and DOCKER_GITCOMMIT not specified'
	echo >&2 '  Please either build with the .git directory accessible, or specify the'
	echo >&2 '  exact (--short) commit hash you are building using DOCKER_GITCOMMIT for'
	echo >&2 '  future accountability in diagnosing build issues.  Thanks!'
	exit 1
fi

if [ "$AUTO_GOPATH" ]; then
	rm -rf .gopath
	mkdir -p .gopath/src/github.com/dotcloud
	ln -sf ../../../.. .gopath/src/github.com/dotcloud/docker
	export GOPATH="$(pwd)/.gopath:$(pwd)/vendor"
fi

if [ ! "$GOPATH" ]; then
	echo >&2 'error: missing GOPATH; please see http://golang.org/doc/code.html#GOPATH'
	echo >&2 '  alternatively, set AUTO_GOPATH=1'
	exit 1
fi

# Use these flags when compiling the tests and final binary
LDFLAGS='-X github.com/dotcloud/docker/dockerversion.GITCOMMIT "'$GITCOMMIT'" -X github.com/dotcloud/docker/dockerversion.VERSION "'$VERSION'" -w'
LDFLAGS_STATIC='-X github.com/dotcloud/docker/dockerversion.IAMSTATIC true -linkmode external -extldflags "-lpthread -static -Wl,--unresolved-symbols=ignore-in-object-files"'
BUILDFLAGS='-tags netgo -a'

HAVE_GO_TEST_COVER=
if \
	go help testflag | grep -- -cover > /dev/null \
	&& go tool -n cover > /dev/null 2>&1 \
; then
	HAVE_GO_TEST_COVER=1
fi

# If $TESTFLAGS is set in the environment, it is passed as extra arguments to 'go test'.
# You can use this to select certain tests to run, eg.
#
#   TESTFLAGS='-run ^TestBuild$' ./hack/make.sh test
#
go_test_dir() {
	dir=$1
	coverpkg=$2
	testcover=()
	if [ "$HAVE_GO_TEST_COVER" ]; then
		# if our current go install has -cover, we want to use it :)
		mkdir -p "$DEST/coverprofiles"
		coverprofile="docker${dir#.}"
		coverprofile="$DEST/coverprofiles/${coverprofile//\//-}"
		testcover=( -cover -coverprofile "$coverprofile" $coverpkg )
	fi
	(
		set -x
		cd "$dir"
		go test ${testcover[@]} -ldflags "$LDFLAGS" $BUILDFLAGS $TESTFLAGS
	)
}

# This helper function walks the current directory looking for directories
# holding certain files ($1 parameter), and prints their paths on standard
# output, one per line.
find_dirs() {
	find -not \( \
		\( -wholename './vendor' -o -wholename './integration' -o -wholename './contrib' -o -wholename './pkg/mflag/example' \) \
		-prune \
	\) -name "$1" -print0 | xargs -0n1 dirname | sort -u
}

bundle() {
	bundlescript=$1
	bundle=$(basename $bundlescript)
	echo "---> Making bundle: $bundle (in bundles/$VERSION/$bundle)"
	mkdir -p bundles/$VERSION/$bundle
	source $bundlescript $(pwd)/bundles/$VERSION/$bundle
}

main() {
	# We want this to fail if the bundles already exist and cannot be removed.
	# This is to avoid mixing bundles from different versions of the code.
	mkdir -p bundles
	if [ -e "bundles/$VERSION" ]; then
		echo "bundles/$VERSION already exists. Removing."
		rm -fr bundles/$VERSION && mkdir bundles/$VERSION || exit 1
		echo
	fi
	SCRIPTDIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
	if [ $# -lt 1 ]; then
		bundles=(${DEFAULT_BUNDLES[@]})
	else
		bundles=($@)
	fi
	for bundle in ${bundles[@]}; do
		bundle $SCRIPTDIR/make/$bundle
		echo
	done
}

main "$@"
