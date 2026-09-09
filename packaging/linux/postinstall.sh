#!/bin/sh
# Runs after install and after upgrade.
#
# Creates the account the unit runs as and the directory it is allowed to write
# to, and does neither if they are already there — an upgrade must not reset a
# deployment's ownership or take away a directory somebody moved.
set -e

if ! getent passwd cronos >/dev/null 2>&1; then
	# A system account with no login and no home: it runs one binary and reads
	# one environment file, and a shell on it is a shell somebody can get.
	useradd --system --no-create-home --shell /usr/sbin/nologin cronos \
		2>/dev/null || adduser --system --no-create-home --shell /usr/sbin/nologin cronos \
		2>/dev/null || true
fi

install -d -o cronos -g cronos -m 0750 /var/lib/cronos
install -d -o cronos -g cronos -m 0750 /var/lib/cronos/definitions

# The environment file holds the signing key, so it is readable by the service
# account and nobody else.
if [ -f /etc/cronos/env ]; then
	chown root:cronos /etc/cronos/env
	chmod 0640 /etc/cronos/env
fi

# Not enabled and not started. A package that starts a server the moment it is
# installed starts one with no signing key, which fails, and leaves an operator
# reading a crash loop instead of a configuration file.
if [ -d /run/systemd/system ]; then
	systemctl daemon-reload >/dev/null 2>&1 || true
fi

echo "cronos installed. Set CRONOS_SIGNING_KEY in /etc/cronos/env, then:"
echo "  systemctl enable --now cronos"
