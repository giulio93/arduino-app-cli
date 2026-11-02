// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package appstore

import (
	"context"
	"fmt"
	"os"

	rpc "github.com/arduino/arduino-cli/rpc/cc/arduino/cli/commands/v1"
)

func SetArduinoCliConfig(ctx context.Context, cli rpc.ArduinoCoreServiceServer) error {
	if _, err := cli.SettingsSetValue(ctx, &rpc.SettingsSetValueRequest{
		Key:          "network.connection_timeout",
		EncodedValue: "600s",
		ValueFormat:  "cli",
	}); err != nil {
		return err
	}

	// Set the data dir if specified via the ARDUINO_DIRECTORIES_DATA env var
	if dataDir, ok := os.LookupEnv("ARDUINO_DIRECTORIES_DATA"); ok {
		_, err := cli.SettingsSetValue(ctx, &rpc.SettingsSetValueRequest{
			Key:          "directories.data",
			EncodedValue: dataDir,
			ValueFormat:  "cli",
		})
		if err != nil {
			return fmt.Errorf("could not set data directory: %w", err)
		}
	}

	// Set the additional urls via ARDUINO_BOARD_MANAGER_ADDITIONAL_URLS env var
	if urls, ok := os.LookupEnv("ARDUINO_BOARD_MANAGER_ADDITIONAL_URLS"); ok {
		_, err := cli.SettingsSetValue(ctx, &rpc.SettingsSetValueRequest{
			Key:          "board_manager.additional_urls",
			EncodedValue: urls,
			ValueFormat:  "cli",
		})
		if err != nil {
			return fmt.Errorf("could not set additional urls: %w", err)
		}
	}

	return nil
}
