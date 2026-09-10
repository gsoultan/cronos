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

# Not enabled and not started, still. Starting a server the moment a package
# lands means starting one nobody has decided the address or the tenancy of, on
# a machine whose operator may be part-way through a script.
if [ -d /run/systemd/system ]; then
	systemctl daemon-reload >/dev/null 2>&1 || true
fi

cat <<'MESSAGE'
cronos installed. To configure it:

  systemctl enable --now cronos
  journalctl -u cronos -n 5     # names the setup token file
  cat /var/lib/cronos/setup-token

Then open http://<this-host>:8787/setup, paste the token, and set the
administrator. cronos writes /var/lib/cronos/config.yaml and restarts into it.

Configuring by hand instead: put the variables in /etc/cronos/env, which
overrides anything setup wrote. See /usr/share/doc/cronos/deploying.md.
MESSAGE
