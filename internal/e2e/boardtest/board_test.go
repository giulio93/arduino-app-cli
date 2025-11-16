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

func TestBlinkBoard(t *testing.T) {

	t.Cleanup(func() { exec.Command("arduino-app-cli", "app", "stop", "examples:blink").Run() })
	waitForPort(t, daemonHost, 5*time.Second)

	cmd := exec.Command("arduino-app-cli", "app", "start", "examples:blink")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to start Blink example: %v\nOutput: %s", err, string(output))
	}
	fmt.Println("start output", string(output))

	passed := waitForAppToStart(t, daemonHost, "running")
	require.True(t, passed, "Blink app reach running state")
	time.Sleep(3 * time.Second)

	cmd = exec.Command("arduino-app-cli", "app", "stop", "examples:blink-with-ui")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to stop Blink example: %v\nOutput: %s", err, string(output))
	}
	fmt.Println("stop output", string(output))

	passed = waitForAppToStart(t, daemonHost, "running")
	require.True(t, passed, "Blink app reach running state")

	cmd = exec.Command("arduino-app-cli", "app", "stop", "examples:blink")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to stop Blink example: %v\nOutput: %s", err, string(output))
	}
	fmt.Println("stop output", string(output))

	passed = waitForAppToStart(t, daemonHost, "stopped")
	require.True(t, passed, "Blink app reach running state")

	cmd = exec.Command("arduino-app-cli", "app", "start", "examples:blink-with-ui")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to start Blink example: %v\nOutput: %s", err, string(output))
	}
	fmt.Println("start output", string(output))
	passed = waitForAppToStart(t, daemonHost, "running")
	require.True(t, passed, "Blink app reach running state")

	cmd = exec.Command("arduino-app-cli", "app", "stop", "examples:blink-with-ui")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to stop Blink example: %v\nOutput: %s", err, string(output))
	}
	fmt.Println("stop output", string(output))

	passed = waitForAppToStart(t, daemonHost, "stopped")
	require.True(t, passed, "Blink app reach running state")

}

func waitForAppToStart(t *testing.T, host string, status string) bool {
	t.Helper()

	appo := false

	url := fmt.Sprintf("http://%s/v1/apps/events", host)

	itr := NewSSEClient(t.Context(), "GET", url)
	for event, err := range itr {
		if err != nil {
			require.NoError(t, err)
			fmt.Println("Error receiving event:", err)
		}
		t.Logf("Received event: ID=%s, Event=%s, Data=%s\n", event.ID, event.Event, string(event.Data))
		if event.Event == "app" && event.Data != nil && strings.Contains(string(event.Data), fmt.Sprintf(`"status":"%s"`, status)) {
			appo = true
			break
		}
	}

	return appo

}
