package ralphloop

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"sort"

	"github.com/elentok/gx/config"
)

// Transport names, also the notification-state key of the global destination.
// Keeping that key equal to the transport name is what lets a state file
// written before destinations existed load as global-destination mutes with no
// migration.
const (
	transportTelegram = "telegram"
	transportSlack    = "slack"
)

// NotifyTransports is the ordered set of transports a destination can use.
var NotifyTransports = []string{transportTelegram, transportSlack}

// Destination is one place notifications go (transport + target) and the
// projects that notify it. Key is its entry in NotificationState.
type Destination struct {
	Key       string
	Transport string
	Projects  []string
}

func telegramTarget(token, chatID string) string {
	if token == "" {
		return ""
	}
	return token + "\x00" + chatID
}

func slackTarget(webhookURL string) string { return webhookURL }

func targetOf(transport string, n config.NotificationsConfig) string {
	if transport == transportTelegram {
		return telegramTarget(n.Telegram.BotToken, n.Telegram.ChatID)
	}
	return slackTarget(n.Slack.WebhookURL)
}

// destinationKey names a destination in the notification state. The global
// destination is the bare transport name; any other target gets a short hash
// of itself, so the key never leaks a token or webhook.
func destinationKey(transport, target, globalTarget string) string {
	if target == globalTarget {
		return transport
	}
	sum := sha256.Sum256([]byte(target))
	return transport + ":" + hex.EncodeToString(sum[:4])
}

// NotificationDestinations lists the global destination of each transport
// (even unconfigured, so status can report it) then every project-specific one.
// A project with no override, or an override equal to the global target, notifies the
// global destination. overrides maps project name to its notification block,
// nil for none.
func NotificationDestinations(global config.NotificationsConfig, overrides map[string]*config.NotificationsConfig) []Destination {
	byKey := map[string]*Destination{}
	var out []Destination
	for _, transport := range NotifyTransports {
		byKey[transport] = &Destination{Key: transport, Transport: transport}
	}
	for project, n := range overrides {
		for _, transport := range NotifyTransports {
			var target string
			if n == nil {
				target = targetOf(transport, global)
			} else {
				target = targetOf(transport, *n)
			}
			if target == "" {
				continue
			}
			key := destinationKey(transport, target, targetOf(transport, global))
			d, ok := byKey[key]
			if !ok {
				d = &Destination{Key: key, Transport: transport}
				byKey[key] = d
			}
			d.Projects = append(d.Projects, project)
		}
	}
	for _, transport := range NotifyTransports {
		out = append(out, *byKey[transport])
		delete(byKey, transport)
	}
	var rest []Destination
	for _, d := range byKey {
		rest = append(rest, *d)
	}
	sort.Slice(rest, func(i, j int) bool { return rest[i].Key < rest[j].Key })
	out = append(out, rest...)
	for i := range out {
		slices.Sort(out[i].Projects)
	}
	return out
}
