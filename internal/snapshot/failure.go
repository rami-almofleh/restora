package snapshot

// Failure is an error with a stable code and a hint for the user, shown in the interface and in the Snapshot status.
type Failure struct {
	Code    string
	Message string
	Hint    string
}

func (f *Failure) Error() string { return f.Code + ": " + f.Message }
