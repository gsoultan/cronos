package config

// SFTP is the server documents are dropped onto, if one is configured.
//
// One per deployment, like the mail relay: a schedule's `to:` names a folder
// on it and never a host, so a value resolved from a customer's row cannot
// send a statement to a server somebody else chose.
type SFTP struct {
	// Addr is host:port. Empty means no channel.
	Addr     string
	User     string
	Password string
	// KeyFile is a PEM private key, for a server that refuses passwords.
	KeyFile string
	// KnownHosts is a known_hosts file holding the server's key. Required:
	// a server whose key is not known is not delivered to.
	KnownHosts string
	// Root is the folder every `to:` is under.
	Root string
}

// Configured reports whether the deployment asked for the channel. Whether
// what it gave is enough is the channel's to say, at startup.
func (s SFTP) Configured() bool { return s.Addr != "" }
