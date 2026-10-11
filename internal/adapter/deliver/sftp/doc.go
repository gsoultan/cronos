// Package sftp delivers rendered documents to a directory on an SFTP server.
//
// The channel a bank, a payroll bureau or a customer's ERP actually polls: a
// statement dropped into an inbound folder is picked up by software, not read
// in a mailbox. Two properties matter for that and both are kept here.
//
// The server is who it says it is. A known_hosts file is required, and a host
// key that is missing from it or does not match is a refusal, never a prompt
// and never "trust on first use" — a statement handed to an impostor is a
// breach, and nobody is watching at six in the morning to answer a prompt.
//
// A file appears whole. Each document is written under a dot-prefixed name
// and renamed into place, so software polling the folder never picks up half a
// PDF. One SSH connection is shared by every delivery of a burst and opened
// again when it drops, because a handshake per recipient is five thousand
// handshakes.
package sftp
