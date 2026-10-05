package herness

// Features are declarative flags for NewAgent (DeerFlow RuntimeFeatures).
type Features struct {
	Sandbox       bool
	LoopDetection bool
	BuiltinTools  bool
}

// DefaultFeatures returns sensible SDK defaults.
func DefaultFeatures() Features {
	return Features{
		Sandbox:       false,
		LoopDetection: true,
		BuiltinTools:  true,
	}
}
