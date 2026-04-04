package workflow

type Definition struct {
	Name    string
	Version string
	Run     func(Context) error
}
