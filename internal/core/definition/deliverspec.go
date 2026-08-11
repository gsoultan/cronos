package definition

// DeliverSpec is one destination a rendered document goes to.
//
// Via is a channel name resolved through a registry, so adding SFTP is a
// registration rather than a change to this type. To, Subject, Body and Attach
// are templates over {{ .row }} and {{ .run }}, because a burst's whole purpose
// is that each recipient's delivery is addressed to them.
type DeliverSpec struct {
	Via string `json:"via" yaml:"via"`
	// To is the destination, in the channel's own terms: an address for email,
	// an s3:// URL for object storage. One field rather than bucket-and-key
	// here and host-and-path there, which would grow this type a field per
	// channel cronos ever adds.
	To      string     `json:"to" yaml:"to"`
	Subject string     `json:"subject,omitempty" yaml:"subject,omitempty"`
	Body    Furniture  `json:"body,omitzero" yaml:"body,omitempty"`
	Attach  AttachSpec `json:"attach,omitzero" yaml:"attach,omitempty"`
	// Options are whatever else a channel needs. A map rather than fields, so
	// registering a channel is not a change to core.
	Options map[string]string `json:"options,omitempty" yaml:"options,omitempty"`
}

// AttachSpec names the file a recipient receives.
//
// The filename matters more than it looks: a mailbox with forty attachments
// all called statement.pdf is a mailbox nobody can search.
type AttachSpec struct {
	Filename string `json:"filename,omitempty" yaml:"filename,omitempty"`
}
