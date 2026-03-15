#!/bin/sh
set -e

# Fix workspace permissions so the non-root codeforge user can access
# files mounted by GitHub Actions / GitLab CI.
WORKSPACE="${GITHUB_WORKSPACE:-${CI_PROJECT_DIR:-/github/workspace}}"
if [ -d "$WORKSPACE" ]; then
    chown -R codeforge:codeforge "$WORKSPACE"
fi

# npm global installs need to be writable by codeforge user
mkdir -p /home/codeforge/.npm
chown -R codeforge:codeforge /home/codeforge
chown -R codeforge:codeforge /usr/local/lib/node_modules /usr/local/bin

# Mark workspace as git safe directory for codeforge user
su-exec codeforge git config --global --add safe.directory '*'

# Drop privileges and run the action binary
exec su-exec codeforge /usr/local/bin/codeforge-action "$@"
