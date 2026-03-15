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
# npm global prefix on Alpine — ensure codeforge can install packages
NPM_PREFIX="$(npm config get prefix 2>/dev/null || echo /usr/local)"
chown -R codeforge:codeforge "$NPM_PREFIX/lib" "$NPM_PREFIX/bin" 2>/dev/null || true

# Ensure SHELL is set — Claude Code requires a POSIX shell
export SHELL=/bin/bash

# Mark workspace as git safe directory for codeforge user
su-exec codeforge git config --global --add safe.directory '*'

# Drop privileges and run the action binary
exec su-exec codeforge /usr/local/bin/codeforge-action "$@"
