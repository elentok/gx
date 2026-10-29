package server

import "strings"

const (
	deviceNameMaxLen   = 40
	deviceNameFallback = "Unknown device"
)

// Each table is checked in order, so more specific markers come first
// (Edge and Opera also say Chrome, Chrome on iOS also says Safari).
var deviceBrowsers = []struct{ marker, name string }{
	{"Edg/", "Edge"},
	{"EdgA/", "Edge"},
	{"EdgiOS/", "Edge"},
	{"OPR/", "Opera"},
	{"Firefox/", "Firefox"},
	{"FxiOS/", "Firefox"},
	{"Chrome/", "Chrome"},
	{"CriOS/", "Chrome"},
	{"Safari/", "Safari"},
}

// iPhone and Android come before Mac and Linux: their user agents mention both.
var deviceSystems = []struct{ marker, name string }{
	{"iPhone", "iPhone"},
	{"iPad", "iPad"},
	{"Android", "Android"},
	{"Macintosh", "Mac"},
	{"Windows", "Windows"},
	{"CrOS", "Chromebook"},
	{"Linux", "Linux"},
}

// deviceName turns a user agent into a label such as "Safari on iPhone".
func deviceName(userAgent string) string {
	browser := firstMarker(userAgent, deviceBrowsers)
	system := firstMarker(userAgent, deviceSystems)
	var name string
	switch {
	case browser != "" && system != "":
		name = browser + " on " + system
	case browser != "":
		name = browser
	case system != "":
		name = system
	default:
		return deviceNameFallback
	}
	if len(name) > deviceNameMaxLen {
		name = name[:deviceNameMaxLen]
	}
	return name
}

func firstMarker(ua string, table []struct{ marker, name string }) string {
	for _, e := range table {
		if strings.Contains(ua, e.marker) {
			return e.name
		}
	}
	return ""
}
