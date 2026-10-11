package config

import "strings"

// Webhook is who documents may be posted to, and the key they are signed with.
type Webhook struct {
	Secret string
	// Hosts are the only hosts a delivery may go to.
	Hosts []string
}

// Configured reports whether the deployment asked for the channel. Half a
// configuration counts, so it is refused at startup rather than ignored.
func (w Webhook) Configured() bool { return w.Secret != "" || len(w.Hosts) > 0 }

func hosts(raw string) []string {
	var out []string
	for _, h := range strings.Split(raw, ",") {
		if h = strings.TrimSpace(h); h != "" {
			out = append(out, h)
		}
	}
	return out
}
