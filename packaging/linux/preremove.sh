#!/bin/sh
# Stops the service before the binary goes, so systemd is not left supervising
# a path that no longer exists.
set -e

if [ -d /run/systemd/system ]; then
	systemctl stop cronos >/dev/null 2>&1 || true
	systemctl disable cronos >/dev/null 2>&1 || true
fi
