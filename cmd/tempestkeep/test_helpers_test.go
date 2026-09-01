package main

import "testing"

type testCloser interface {
	Close() error
}

func closeOnCleanup(t testing.TB, closer testCloser) {
	t.Helper()
	t.Cleanup(func() {
		if err := closer.Close(); err != nil {
			t.Errorf("cleanup close: %v", err)
		}
	})
}
