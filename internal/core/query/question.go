package query

// Question writes a positional question mark. MySQL, SQLite.
type Question struct{}

func (Question) At(int) string { return "?" }
