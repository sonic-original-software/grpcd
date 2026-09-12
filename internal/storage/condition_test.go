package storage

import "testing"

func TestConditions(t *testing.T) {
	t.Run("starts healthy with an open announcement", func(t *testing.T) {
		current := NewConditions().Current()

		if current.Lost {
			t.Error("expected the store to start healthy")
		}

		if isClosed(current.Changed) {
			t.Error("expected Changed to be open before anything happened")
		}
	})

	t.Run("losing the store announces it once", func(t *testing.T) {
		c := NewConditions()
		healthy := c.Current()

		c.Lose()

		if !isClosed(healthy.Changed) {
			t.Error("expected the healthy condition to be announced over")
		}

		lost := c.Current()

		if !lost.Lost {
			t.Error("expected the store to be lost")
		}

		c.Lose()

		if got := c.Current(); got != lost {
			t.Error("expected a second loss to change nothing")
		}

		if isClosed(lost.Changed) {
			t.Error("expected the lost condition to stay open through a repeated loss")
		}
	})

	t.Run("recovering announces it and is healthy again", func(t *testing.T) {
		c := NewConditions()
		c.Lose()
		lost := c.Current()

		c.Recover()

		if !isClosed(lost.Changed) {
			t.Error("expected the lost condition to be announced over")
		}

		if c.Current().Lost {
			t.Error("expected the store to be healthy")
		}
	})

	t.Run("recovering while healthy still announces", func(t *testing.T) {
		c := NewConditions()
		healthy := c.Current()

		c.Recover()

		if !isClosed(healthy.Changed) {
			t.Error("expected a recovery with no loss seen to wake sleepers anyway")
		}

		if c.Current().Lost {
			t.Error("expected the store to be healthy")
		}
	})
}
