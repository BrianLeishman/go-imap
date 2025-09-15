package imap

// FlagSet represents the action to take on a flag
type FlagSet int

const (
	FlagUnset FlagSet = iota
	FlagAdd
	FlagRemove
)

// Flags represents standard IMAP message flags
type Flags struct {
	Seen     FlagSet
	Answered FlagSet
	Flagged  FlagSet
	Deleted  FlagSet
	Draft    FlagSet
	Keywords map[string]bool
}
