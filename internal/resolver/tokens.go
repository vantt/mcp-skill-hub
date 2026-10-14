package resolver

// Tokenize splits text into the lowercase search tokens the resolver matches
// on, with stop words removed. Callers outside the package use it to mirror the
// resolver's query tokens.
func Tokenize(value string) []string { return tokenize(value) }
