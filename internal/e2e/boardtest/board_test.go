package boardtest

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const daemonHost = "127.0.0.1:8800"
const sseAppEventsPath = "http://127.0.0.1:8800/v1/apps/events"
const sseAppIDEventsPath = "http://127.0.0.1:8800/v1/apps/%s/events"

func TestBlinkBoard(t *testing.T) {

	//t.Cleanup(func() { exec.Command("arduino-app-cli", "app", "stop", "examples:blink").Run() })
	waitForPort(t, daemonHost, 5*time.Second)

	exec.Command("arduino-app-cli", "app", "start", "examples:blink").Run()

	passed := waitForSSEevent(t, sseAppEventsPath, "Blink LED", "running")
	require.True(t, passed, "Blink app reach running state")

	exec.Command("arduino-app-cli", "app", "stop", "examples:blink").Run()

	passed = waitForSSEevent(t, sseAppEventsPath, "Blink LED", "stopped")
	require.True(t, passed, "Blink app reach running state")

}

func waitForSSEevent(t *testing.T, url, name, status string) bool {
	t.Helper()

	appo := false

	itr := NewSSEClient(t.Context(), "GET", url)
	for event, err := range itr {
		if err != nil {
			require.NoError(t, err)
			fmt.Println("Error receiving event:", err)
		}
		t.Logf("Received event: ID=%s, Event=%s, Data=%s\n", event.ID, event.Event, string(event.Data))
		if event.Event == "app" && event.Data != nil && strings.Contains(string(event.Data), fmt.Sprintf(`"status":"%s"`, status)) && strings.Contains(string(event.Data), fmt.Sprintf(`"name":"%s"`, name)) {
			appo = true
			break
		}
	}

	return appo

}
