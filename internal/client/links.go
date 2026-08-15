package client

// Link is one transition the service offered: where to go and what verb to use.
type Link struct {
	Href   string `json:"href"`
	Method string `json:"method,omitempty"`
}

// Links is the set of transitions on a response. The bot renders controls from these
// and from nothing else: if the service did not return a lock link, there is no lock
// button, whatever the bot believes about the caller.
type Links map[string]Link

// Has reports whether the service offered a transition by that name.
func (l Links) Has(name string) bool {
	_, ok := l[name]
	return ok
}

// Get returns the named link. The second result is false when the service did not
// offer it, which is the authorization answer, not an error.
func (l Links) Get(name string) (Link, bool) {
	link, ok := l[name]
	return link, ok
}
