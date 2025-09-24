package channel_helper

func WriteToChannelAndBufferLatest[T any](ch chan T, v T) {
	// First, try to send without blocking.
	select {
	case ch <- v:
		return // success – nothing else to do
	default:
		// Channel is full.  Drop the oldest entry (if any) and try again.
		select {
		case <-ch: // discard one element
		default: // nothing to discard – should be very rare
		}

		// Second attempt – this one should succeed because we just freed a slot.
		// If it still fails we just give up (same as the original code).
		select {
		case ch <- v:
		default:
		}
	}
}