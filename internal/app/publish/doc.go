// Package publish stores a definition after proving it will work.
//
// Everything expensive to be wrong about happens here rather than at run time.
// A dataset that cannot compile, a report binding a filter to a field nobody
// publishes, a report reading a dataset that does not exist — each of those
// fails identically at 6am in the middle of a burst, with the author asleep
// and the only evidence a line in a delivery log. Publishing is the moment
// somebody is looking.
package publish
