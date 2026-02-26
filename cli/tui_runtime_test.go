package cli

import "testing"

func TestShouldUseWatchUI(t *testing.T) {
	cases := []struct {
		name       string
		isTTY      bool
		noUI       bool
		background bool
		status     bool
		stop       bool
		workspace  string
		want       bool
	}{
		{"tty foreground", true, false, false, false, false, "", true},
		{"non tty", false, false, false, false, false, "", false},
		{"explicit no ui", true, true, false, false, false, "", false},
		{"background mode", true, false, true, false, false, "", false},
		{"status mode", true, false, false, true, false, "", false},
		{"stop mode", true, false, false, false, true, "", false},
		{"workspace mode", true, false, false, false, false, "ws", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldUseWatchUI(tc.isTTY, tc.noUI, tc.background, tc.status, tc.stop, tc.workspace)
			if got != tc.want {
				t.Fatalf("shouldUseWatchUI() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestShouldUseStatusUI(t *testing.T) {
	if !shouldUseStatusUI(true, false) {
		t.Fatal("expected status UI to be enabled in tty mode")
	}
	if shouldUseStatusUI(true, true) {
		t.Fatal("expected --no-ui to disable status UI")
	}
	if shouldUseStatusUI(false, false) {
		t.Fatal("expected non-tty to disable status UI")
	}
}
