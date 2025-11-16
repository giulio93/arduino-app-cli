package boardtest

import (
	"os/exec"
	"strings"
	"testing"
)

func TestBlinkBoard(t *testing.T) {

	cmd := exec.Command("arduino-app-cli", "app", "start", "examples:blink")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to start Blink example: %v\nOutput: %s", err, string(output))
	}

	expectedOutput := "Blink example started successfully"
	if !strings.Contains(string(output), expectedOutput) {
		t.Errorf("Unexpected output. Got: %s, Want to contain: %s", string(output), expectedOutput)
	}

	cmd = exec.Command("arduino-app-cli", "app", "stop", "examples:blink")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to stop Blink example: %v\nOutput: %s", err, string(output))
	}

	expectedOutput = "Blink example stopped successfully"
	if !strings.Contains(string(output), expectedOutput) {
		t.Errorf("Unexpected output. Got: %s, Want to contain: %s", string(output), expectedOutput)
	}
}
