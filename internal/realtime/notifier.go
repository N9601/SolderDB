package realtime

import "time"

// CollectionsNotifier adapts a *Hub to the collections.Notifier interface so
// the collections package doesn't need to import this package (avoids cycles).
type CollectionsNotifier struct {
	Hub *Hub
}

func (c CollectionsNotifier) Publish(topic, kind, collection, id string, data interface{}) {
	if c.Hub == nil {
		return
	}
	c.Hub.Publish(topic, Event{
		Kind:       EventKind(kind),
		Collection: collection,
		ID:         id,
		Data:       data,
		Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
	})
}
