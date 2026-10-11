// Package webhook delivers rendered documents by HTTPS POST.
//
// For the system that wants a statement handed to it rather than fetched: a
// customer portal, a document archive, an integration platform. The request
// body is the document itself, so a receiver needs no decoding to store it.
//
// # What a receiver can rely on
//
// Every request is signed. `Cronos-Signature: t=<unix seconds>,v1=<hex>` is an
// HMAC-SHA256, under the deployment's webhook secret, of the timestamp, a dot
// and the body. A receiver recomputes it, compares in constant time, and
// refuses a timestamp more than a few minutes old — which is what stops a
// captured request being replayed.
//
// `Idempotency-Key` is the same for every attempt at one delivery, so a
// receiver that already has it can answer 200 and keep one copy. A delivery is
// retried on a 5xx, a 408, a 429 or no answer; any other 4xx is final, because
// asking again will be refused again.
//
// # Where it may go
//
// Only to hosts the deployment lists in CRONOS_WEBHOOK_HOSTS. The URL is
// resolved from a schedule and often from a customer's row, and a channel
// that POSTs wherever a string says is a way to make the server send its
// customers' statements, and requests carrying its secret's signature, to
// anybody — including services inside the network the server sits in. Plain
// http is refused except to the loopback interface, and a redirect is never
// followed, since following one is going somewhere the list did not name.
package webhook
