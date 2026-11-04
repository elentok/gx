package wire

const (
	ReasonWrongCredentials  = "wrong-credentials"
	ReasonSetupCodeWrong    = "setup-code-wrong"
	ReasonSetupCodeRequired = "setup-code-required"
	ReasonSetupNotPending   = "setup-not-pending"
	ReasonTOTPMismatch      = "totp-mismatch"
	ReasonPasswordTooShort  = "password-too-short"
	ReasonTOTPOff           = "totp-off"
	ReasonLoginNotFound     = "login-not-found"
	ReasonLoginNameInvalid  = "login-name-invalid"
)
