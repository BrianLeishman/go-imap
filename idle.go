package imap

type ExistsEvent struct {
	MessageIndex int
}

type ExpungeEvent struct {
	MessageIndex int
}

type FetchEvent struct {
	MessageIndex int
	UID          uint32
	Flags        []string
}

type IdleHandler struct {
	OnExists  func(event ExistsEvent)
	OnExpunge func(event ExpungeEvent)
	OnFetch   func(event FetchEvent)
}

const (
	IdleEventExists  = "EXISTS"
	IdleEventExpunge = "EXPUNGE"
	IdleEventFetch   = "FETCH"
)
