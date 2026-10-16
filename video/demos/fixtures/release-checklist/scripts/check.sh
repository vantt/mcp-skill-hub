#!/bin/sh
# What a release needs before it is tagged.
if ! git diff --quiet; then
	echo "Uncommitted changes: commit or stash them first."
	exit 1
fi
echo "Ready to tag."
