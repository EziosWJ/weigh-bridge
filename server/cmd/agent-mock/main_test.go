package main

import "testing"

func TestMockConnectsToConfiguredAPIPort(t *testing.T) {
	for _, test := range []struct{ listen, target string }{
		{":8900", "http://127.0.0.1:8900"},
		{"0.0.0.0:8900", "http://127.0.0.1:8900"},
		{"[::]:8900", "http://127.0.0.1:8900"},
		{"127.0.0.1:8900", "http://127.0.0.1:8900"},
		{"localhost:8900", "http://localhost:8900"},
		{"[::1]:8900", "http://[::1]:8900"},
	} {
		t.Run(test.listen, func(t *testing.T) {
			if got := localServerURL(test.listen); got != test.target {
				t.Fatalf("mock target = %q, want %q", got, test.target)
			}
		})
	}
}
