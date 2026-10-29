package server

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"html"
	"net/http"
	"strings"
	"text/template"
)

const (
	webRootProfilePath = "/gx-root.mobileconfig"
	webRootCerPath     = "/gx-root.cer"
)

// webRootProfileTmpl is an unsigned configuration profile with one root
// payload. Every interpolated value is XML-escaped by the xml func.
var webRootProfileTmpl = template.Must(template.New("profile").Funcs(template.FuncMap{"xml": html.EscapeString}).Parse(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>PayloadContent</key>
  <array>
    <dict>
      <key>PayloadCertificateFileName</key><string>gx-root.cer</string>
      <key>PayloadContent</key><data>{{.Cert}}</data>
      <key>PayloadDisplayName</key><string>gx root</string>
      <key>PayloadIdentifier</key><string>{{xml .Identifier}}.cert</string>
      <key>PayloadType</key><string>com.apple.security.root</string>
      <key>PayloadUUID</key><string>{{.CertUUID}}</string>
      <key>PayloadVersion</key><integer>1</integer>
    </dict>
  </array>
  <key>PayloadDescription</key><string>Lets this device trust gx's HTTPS certificate. SHA-256 fingerprint: {{xml .Fingerprint}}</string>
  <key>PayloadDisplayName</key><string>gx web UI</string>
  <key>PayloadIdentifier</key><string>{{xml .Identifier}}</string>
  <key>PayloadType</key><string>Configuration</string>
  <key>PayloadUUID</key><string>{{.UUID}}</string>
  <key>PayloadVersion</key><integer>1</integer>
</dict>
</plist>
`))

// webRootProfile builds the profile for a root's DER. The identifier is stable
// per root so a later profile replaces the old one; the UUIDs are fresh.
func webRootProfile(der []byte) ([]byte, error) {
	var b strings.Builder
	data := struct{ Cert, Identifier, Fingerprint, UUID, CertUUID string }{
		Cert:        base64.StdEncoding.EncodeToString(der),
		Identifier:  "com.elentok.gx.root." + strings.ReplaceAll(webRootFingerprint(der), " ", "")[:8],
		Fingerprint: webRootFingerprint(der),
		UUID:        randomUUID(),
		CertUUID:    randomUUID(),
	}
	if err := webRootProfileTmpl.Execute(&b, data); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}

func randomUUID() string {
	var u [16]byte
	_, _ = rand.Read(u[:])
	u[6] = u[6]&0x0f | 0x40
	u[8] = u[8]&0x3f | 0x80
	return fmt.Sprintf("%X-%X-%X-%X-%X", u[0:4], u[4:6], u[6:8], u[8:10], u[10:])
}

// webRootProfileRead serves the root as an iOS/macOS profile. No HSTS header:
// it would block the browser's "visit anyway" step.
func (s *Server) webRootProfileRead(w http.ResponseWriter, _ *http.Request) {
	body, err := webRootProfile(s.webCerts.Root().Raw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-apple-aspen-config")
	w.Header().Set("Content-Disposition", `attachment; filename="gx-root.mobileconfig"`)
	_, _ = w.Write(body)
}

// webRootCerRead serves the root as DER.
func (s *Server) webRootCerRead(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/pkix-cert")
	w.Header().Set("Content-Disposition", `attachment; filename="gx-root.cer"`)
	_, _ = w.Write(s.webCerts.Root().Raw)
}
