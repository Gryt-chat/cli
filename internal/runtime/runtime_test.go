package runtime

import (
	"errors"
	"strings"
	"testing"
)

// Both of these are real output shapes seen while testing the shared stack.
func TestComposeReasonFindsTheErrorPastTheProgress(t *testing.T) {
	cases := []struct {
		name, output, want string
	}{
		{
			name: "network failure under progress lines",
			output: ` Network gryt  Creating
 Network gryt  Error
Error response from daemon: failed to set up container networking: driver failed`,
			want: "Error response from daemon: failed to set up container networking: driver failed",
		},
		{
			name: "the reason is the last line",
			output: ` Container gryt-combined  Creating
 Container gryt-combined  Created
 Container gryt-combined  Starting
cannot start service server: port is already allocated`,
			want: "cannot start service server: port is already allocated",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := composeReason(c.output, errors.New("exit status 1"))
			if got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
			// The bug: the first progress line was what got shown.
			if strings.Contains(got, "Creating") {
				t.Fatalf("reported a progress line: %q", got)
			}
		})
	}
}

func TestComposeReasonFallsBackToTheLastThingItSaid(t *testing.T) {
	output := " Container one  Creating\n Container one  Created"
	if got := composeReason(output, errors.New("exit status 1")); got != "Container one  Created" {
		t.Fatalf("got %q", got)
	}
}

func TestComposeReasonFallsBackToTheProcessError(t *testing.T) {
	if got := composeReason("   \n\n", errors.New("exit status 127")); got != "exit status 127" {
		t.Fatalf("got %q", got)
	}
}
