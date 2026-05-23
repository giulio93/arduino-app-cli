// This file is part of arduino-app-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package modelsindex

import (
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Inline fixture matching the new deployment shape introduced by
// arduino/app-bricks-py#227.
const deploymentFixtureYAML = `
models:
 - "genie:qwen3_4b_instruct_2507":
    name: "Qwen 3-4B Instruct"
    description: "test"
    supported_boards: ["ventunoq"]
    bricks:
      - id: "arduino:llm"
    deployment:
      handler: "ai-hub-handler"
      platforms:
        - ventunoq:
            variables:
              model_type: "genie"
              model_name: "qwen3_4b_instruct_2507"
              models_repository: "/var/lib/arduino-app-cli/models/genai"
              model_directory: "qwen3_4b_instruct_2507-genie-w4a16-qualcomm_qcs8275"
              quantization: "w4a16"
              chipset: "qualcomm-qcs8275"
              version: "0.51.0"
        - other-board:
            variables:
              model_type: "genie"
              model_name: "qwen3_4b_instruct_2507"
              quantization: "w8a16"
    metadata:
      source: "qualcomm-ai-hub"
 - hf-test-model:
    name: "HF Test"
    description: "test"
    bricks:
      - id: "arduino:llm"
    deployment:
      handler: "hf-handler"
      platforms:
        - ventunoq:
            variables:
              model_key: "llamacpp:unsloth/gemma-4-E4B-it-GGUF:Q4_0"
              models_repository: "/var/lib/arduino-app-cli/models/genai"
 - legacy-no-deployment:
    name: "Legacy"
    description: "test"
    runner: "brick"
    bricks:
      - id: "arduino:object_detection"
`

func TestUnmarshalDeploymentBlock(t *testing.T) {
	var list assetsModelList
	require.NoError(t, yaml.Unmarshal([]byte(deploymentFixtureYAML), &list))
	require.Len(t, list.Models, 3)

	// Pull out the three entries by ID.
	byID := map[string]AIModel{}
	for _, m := range list.Models {
		for id, model := range m {
			model.ID = id
			byID[id] = model
		}
	}

	t.Run("AI Hub model parses handler + variables", func(t *testing.T) {
		m := byID["genie:qwen3_4b_instruct_2507"]
		require.NotNil(t, m.Deployment, "deployment block should be parsed")
		assert.Equal(t, "ai-hub-handler", m.Deployment.Handler)
		require.Len(t, m.Deployment.Platforms, 2)

		// First platform: ventunoq with the full variable set.
		p := m.Deployment.Platforms[0]
		assert.Equal(t, "ventunoq", p.Name)
		assert.Equal(t, "genie", p.Variables["model_type"])
		assert.Equal(t, "qwen3_4b_instruct_2507", p.Variables["model_name"])
		assert.Equal(t, "w4a16", p.Variables["quantization"])
		assert.Equal(t, "qualcomm-qcs8275", p.Variables["chipset"])
		assert.Equal(t, "0.51.0", p.Variables["version"])
		assert.Equal(t, "/var/lib/arduino-app-cli/models/genai", p.Variables["models_repository"])

		// Second platform: other-board with a different quantization.
		assert.Equal(t, "other-board", m.Deployment.Platforms[1].Name)
		assert.Equal(t, "w8a16", m.Deployment.Platforms[1].Variables["quantization"])
	})

	t.Run("HF model parses model_key", func(t *testing.T) {
		m := byID["hf-test-model"]
		require.NotNil(t, m.Deployment)
		assert.Equal(t, "hf-handler", m.Deployment.Handler)
		require.Len(t, m.Deployment.Platforms, 1)
		assert.Equal(t, "llamacpp:unsloth/gemma-4-E4B-it-GGUF:Q4_0",
			m.Deployment.Platforms[0].Variables["model_key"])
	})

	t.Run("legacy model with no deployment block still parses", func(t *testing.T) {
		m := byID["legacy-no-deployment"]
		assert.Nil(t, m.Deployment, "absent deployment block must yield nil pointer")
		assert.Equal(t, "brick", m.Runner)
	})
}

func TestVariablesFor(t *testing.T) {
	m := &AIModel{
		Deployment: &Deployment{
			Handler: "ai-hub-handler",
			Platforms: []DeploymentPlatform{
				{Name: "ventunoq", Variables: map[string]string{"k": "v1"}},
				{Name: "other", Variables: map[string]string{"k": "v2"}},
			},
		},
	}

	assert.Equal(t, "v1", m.VariablesFor("ventunoq")["k"])
	assert.Equal(t, "v2", m.VariablesFor("other")["k"])
	assert.Nil(t, m.VariablesFor("unknown-board"))

	// Nil-safe.
	var nilModel *AIModel
	assert.Nil(t, nilModel.VariablesFor("anything"))

	// No deployment block.
	assert.Nil(t, (&AIModel{}).VariablesFor("anything"))
}

func TestDeploymentRoundTrip(t *testing.T) {
	// Marshalling a Deployment back to YAML should produce the same list-of-
	// single-key-maps shape that the on-disk file uses.
	d := Deployment{
		Handler: "ai-hub-handler",
		Platforms: []DeploymentPlatform{
			{Name: "ventunoq", Variables: map[string]string{"model_name": "qwen3"}},
		},
	}
	out, err := yaml.Marshal(d)
	require.NoError(t, err)

	// Round-trip through unmarshal: same structure.
	var back Deployment
	require.NoError(t, yaml.Unmarshal(out, &back))
	require.Len(t, back.Platforms, 1)
	assert.Equal(t, "ventunoq", back.Platforms[0].Name)
	assert.Equal(t, "qwen3", back.Platforms[0].Variables["model_name"])
}
