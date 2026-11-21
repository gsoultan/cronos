/*
Package explain says, in plain language, what a report shows.

A reader who opens a report and does not know what "net revenue retention" is,
or whether a 4% drop is a lot, asks the report rather than a colleague. The
answer comes from a language model, and the whole of this package is the two
decisions that make that safe to offer.

What the model is told is the view the reader already has. The caller renders
the report as the person asking — their grants, their row scope, the filters
pinned for them — and hands this package the result. Nothing here reads a
dataset, so there is no path by which an explanation knows a number its reader
could not have seen. What leaves is bounded too: see digest.

What the model says is presented as a reading of those numbers and nothing
more. The prompt tells it to use only the figures it was given, to say so when
they cannot answer the question, and never to guess at causes the data does
not show — and the portal labels every answer as generated.

The model is a port. Which provider answers is the deployment's choice, and
this package neither knows nor imports one.
*/
package explain
