package state

type Mode int

const (
	ModeChat Mode = iota
	ModeCodeInput
	ModeHelp
	ModeMemory
)

type UIState struct {
	Width      int
	Height     int
	Mode       Mode
	Focused    string
	AutoScroll bool
}
