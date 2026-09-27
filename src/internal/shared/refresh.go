package shared

// RefreshGate keeps a view's reloads of one resource from overlapping or
// arriving out of order. Every fetch takes a sequence number from Start and
// tags its response with it; background ticks are skipped while a fetch is
// in flight, and Accept drops any response that is not from the newest
// fetch, so a slow older request can never overwrite newer data.
//
// The zero value is ready to use. It is a plain value, so it is copied
// along with the (value-receiver) Bubble Tea model that holds it and must
// only be used from Update.
type RefreshGate struct {
	seq      uint64
	inflight bool
}

// Start marks a fetch as in flight and returns the sequence number its
// response must carry.
func (g *RefreshGate) Start() uint64 {
	g.seq++
	g.inflight = true
	return g.seq
}

// Seq returns the newest sequence number without starting a fetch. Init
// methods (whose model copy is discarded) tag their first fetch with it.
func (g RefreshGate) Seq() uint64 {
	return g.seq
}

// Busy reports whether a fetch is still in flight; background ticks should
// not start another one.
func (g RefreshGate) Busy() bool {
	return g.inflight
}

// Accept reports whether a response tagged seq is from the newest fetch and
// should be applied. Accepting it ends the in-flight period; stale responses
// are rejected and leave the newer fetch in flight.
func (g *RefreshGate) Accept(seq uint64) bool {
	if seq != g.seq {
		return false
	}
	g.inflight = false
	return true
}
