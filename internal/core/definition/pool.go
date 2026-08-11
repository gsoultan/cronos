package definition

// Pool bounds the connections cronos holds open to one source.
//
// Somebody else operates that database. A reporting tool that opens two
// hundred connections during a burst is a reporting tool their DBA turns off.
type Pool struct {
	MaxOpen     int      `json:"maxOpen,omitempty" yaml:"maxOpen,omitempty"`
	MaxIdleTime Duration `json:"maxIdleTime,omitempty" yaml:"maxIdleTime,omitempty"`
}
