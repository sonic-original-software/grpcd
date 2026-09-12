package storage

import "testing"

// isClosed reports whether done has been closed without blocking.
func isClosed(done chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}

func TestAdditions(t *testing.T) {
	t.Run("starts with an open announcement of nothing", func(t *testing.T) {
		latest := NewAdditions().Latest()

		if latest.Method != "" || latest.Address != "" {
			t.Errorf("latest = %+v, want empty", latest)
		}

		if isClosed(latest.Done) {
			t.Error("expected Done to be open before anything is announced")
		}
	})

	t.Run("wakes the previous announcement and carries the new one", func(t *testing.T) {
		a := NewAdditions()
		first := a.Latest()

		a.Announce("/pkg.Service/Method", "10.0.0.1:50054")

		if !isClosed(first.Done) {
			t.Error("expected the previous Done to be closed")
		}

		latest := a.Latest()

		if latest.Method != "/pkg.Service/Method" || latest.Address != "10.0.0.1:50054" {
			t.Errorf("latest = %+v, want the announcement", latest)
		}

		if isClosed(latest.Done) {
			t.Error("expected the new Done to be open")
		}
	})

	t.Run("closes each announcement in turn", func(t *testing.T) {
		a := NewAdditions()

		a.Announce("/pkg.Service/One", "10.0.0.1:50054")
		second := a.Latest()

		a.Announce("/pkg.Service/Two", "10.0.0.2:50054")

		if !isClosed(second.Done) {
			t.Error("expected the second announcement to be closed by the third")
		}

		if got := a.Latest().Address; got != "10.0.0.2:50054" {
			t.Errorf("latest address = %q, want 10.0.0.2:50054", got)
		}
	})
}
