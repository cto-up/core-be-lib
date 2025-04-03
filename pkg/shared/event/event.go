package event

type Event struct {
	EventType string `json:"eventType"`
	Message   string `json:"message"`
}

type ProgressEvent struct {
	Event
	Progress int `json:"progress"`
}

func NewProgressEvent(eventType string, message string, progress int) ProgressEvent {
	return ProgressEvent{
		Event: Event{EventType: eventType,
			Message: message,
		},
		Progress: progress,
	}
}
