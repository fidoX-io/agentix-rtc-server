// Copyright 2024 LiveKit, Inc.
// Copyright 2024 FidoX.io - AgentIX RTC Server modifications
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package audio

// NoiseFilterConfig holds configuration for noise suppression
type NoiseFilterConfig struct {
	Enabled    bool    `json:"enabled" yaml:"enabled"`
	Threshold  float32 `json:"threshold" yaml:"threshold"` // VAD threshold (0.0-1.0)
	Aggressive bool    `json:"aggressive" yaml:"aggressive"` // More aggressive noise suppression
}

// DefaultNoiseFilterConfig returns the default noise filter configuration
func DefaultNoiseFilterConfig() NoiseFilterConfig {
	return NoiseFilterConfig{
		Enabled:    false, // Disabled by default for compatibility
		Threshold:  0.5,   // Moderate VAD threshold
		Aggressive: false,
	}
}