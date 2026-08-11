// Package email delivers documents as attachments over SMTP.
//
// net/smtp and mime/multipart from the standard library, because a MIME
// message with one attachment is a well-specified thing and a dependency for
// it is a dependency to keep patched.
//
// The message is assembled in memory. A statement is a few hundred kilobytes,
// and streaming it would mean holding an SMTP conversation open across a
// render — a connection to somebody's mail server held for the duration of a
// PDF is how a burst gets rate-limited.
package email
