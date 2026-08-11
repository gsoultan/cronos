package s3

import (
	"fmt"
	"net/url"
	"strings"
)

// destination is a parsed s3:// URL.
//
// One `to:` field for every channel — see definition.DeliverSpec. Object
// storage reads it as a URL, which is how everyone already writes a bucket and
// a key, and means adding sftp later is a new parser rather than two more
// fields in the definition format.
type destination struct {
	Bucket string
	Key    string
}

func parse(to string) (destination, error) {
	u, err := url.Parse(strings.TrimSpace(to))
	if err != nil {
		return destination{}, fmt.Errorf("s3: %q is not a destination", to)
	}
	if u.Scheme != "s3" {
		return destination{}, fmt.Errorf("s3: %q is not an s3:// url", to)
	}
	key := strings.TrimPrefix(u.Path, "/")
	switch {
	case u.Host == "":
		return destination{}, fmt.Errorf("s3: %q names no bucket", to)
	case key == "":
		// A bucket root with no key would put every recipient's statement at
		// the same address, so a burst of five thousand leaves one object.
		return destination{}, fmt.Errorf("s3: %q names no key", to)
	case strings.Contains(key, ".."):
		// Keys are resolved from a customer row. S3 has no directories to
		// escape, but a key containing .. confuses every tool that mirrors a
		// bucket onto a filesystem afterwards.
		return destination{}, fmt.Errorf("s3: key %q contains ..", key)
	}
	return destination{Bucket: u.Host, Key: key}, nil
}
