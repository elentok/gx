package wire

// DevState is where Dev mode is in its life.
type DevState string

const (
	DevStateOff        DevState = "off"
	DevStateInstalling DevState = "installing"
	DevStateStarting   DevState = "starting"
	DevStateRunning    DevState = "running"
	DevStateDown       DevState = "down"
)

// DevStatus is the Dev block of the snapshot.
type DevStatus struct {
	State DevState `json:"state"`
	// HasCheckout is whether gx found a Dev checkout.
	HasCheckout bool `json:"has_checkout"`
	// Reason says why Dev mode cannot run or is down; empty when nothing is wrong.
	Reason string `json:"reason,omitempty"`
	// LogTail is the last Vite log lines.
	LogTail []string `json:"log_tail,omitempty"`
	// Checkout is the Dev checkout root; empty without one.
	Checkout string `json:"checkout,omitempty"`
}
