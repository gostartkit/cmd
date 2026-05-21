package cmd

// LexLine converts a REPL input line into argv-style tokens.
func LexLine(line string) ([]string, error) {
	return SplitLine(line)
}

// LexLineForCompletion tokenizes a partial REPL line for cursor-aware completion.
func LexLineForCompletion(line string) ([]string, string, error) {
	return SplitLineForCompletion(line)
}
