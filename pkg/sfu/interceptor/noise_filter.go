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

package interceptor

import (
	"encoding/binary"
	"sync"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"github.com/zhangzhao-gg/go-rnnoise/rnnoise"

	"github.com/livekit/livekit-server/pkg/sfu/audio"
	"github.com/livekit/livekit-server/pkg/sfu/utils"
	"github.com/livekit/protocol/logger"
)

const (
	// RNNoise expects 48kHz, 16-bit, mono audio
	rnnoiseSampleRate     = 48000
	rnnoiseFrameSize      = 480 // 10ms at 48kHz
	rnnoiseBytesPerSample = 2
	rnnoiseFrameBytes     = rnnoiseFrameSize * rnnoiseBytesPerSample
)

// NoiseFilterFactory creates noise filter interceptors for audio streams
type NoiseFilterFactory struct {
	config audio.NoiseFilterConfig
	logger logger.Logger
	mu     sync.RWMutex
}

// NewNoiseFilterFactory creates a new noise filter factory
func NewNoiseFilterFactory(config audio.NoiseFilterConfig, logger logger.Logger) *NoiseFilterFactory {
	return &NoiseFilterFactory{
		config: config,
		logger: logger,
	}
}

// UpdateConfig updates the noise filter configuration
func (f *NoiseFilterFactory) UpdateConfig(config audio.NoiseFilterConfig) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.config = config
}

// GetConfig returns the current configuration
func (f *NoiseFilterFactory) GetConfig() audio.NoiseFilterConfig {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.config
}

// NewInterceptor creates a new noise filter interceptor instance
func (f *NoiseFilterFactory) NewInterceptor(id string) (interceptor.Interceptor, error) {
	return &NoiseFilterInterceptor{
		factory: f,
		logger:  f.logger.WithValues("interceptor", "noise_filter", "id", id),
	}, nil
}

// NoiseFilterInterceptor implements the noise suppression interceptor
type NoiseFilterInterceptor struct {
	interceptor.NoOp
	factory *NoiseFilterFactory
	logger  logger.Logger
}

// BindRemoteStream binds the noise filter to incoming audio streams
func (n *NoiseFilterInterceptor) BindRemoteStream(info *interceptor.StreamInfo, reader interceptor.RTPReader) interceptor.RTPReader {
	// Only process audio streams
	if info.RTPHeaderExtensions == nil {
		return reader
	}

	config := n.factory.GetConfig()
	if !config.Enabled {
		return reader
	}

	// Check if this is an audio stream (look for audio level extension)
	audioLevelExtID := utils.GetHeaderExtensionID(info.RTPHeaderExtensions, webrtc.RTPHeaderExtensionCapability{
		URI: "urn:ietf:params:rtp-hdrext:ssrc-audio-level",
	})

	if audioLevelExtID == 0 {
		// Not an audio stream, pass through
		return reader
	}

	n.logger.Debugw("applying noise filter to audio stream", "ssrc", info.SSRC, "config", config)

	return &noiseFilterReader{
		reader:   reader,
		config:   config,
		denoiser: nil, // Will be initialized on first packet
		logger:   n.logger,
		buffer:   make([]byte, 0, rnnoiseFrameBytes*2), // Buffer for incomplete frames
	}
}

// noiseFilterReader processes RTP packets and applies noise suppression
type noiseFilterReader struct {
	reader   interceptor.RTPReader
	config   audio.NoiseFilterConfig
	denoiser *rnnoise.NoiseFilter
	logger   logger.Logger
	buffer   []byte
	mu       sync.Mutex
}

// Read processes an RTP packet and applies noise suppression to audio payload
func (r *noiseFilterReader) Read(b []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {
	n, a, err := r.reader.Read(b, a)
	if err != nil {
		return n, a, err
	}

	// Initialize denoiser on first packet
	r.mu.Lock()
	if r.denoiser == nil {
		var err error
		r.denoiser, err = rnnoise.NewNoiseFilter("")
		if err != nil {
			r.logger.Errorw("failed to initialize RNNoise denoiser", err)
			r.mu.Unlock()
			return n, a, nil // Pass through without processing
		}
		r.logger.Debugw("initialized RNNoise denoiser")
	}
	r.mu.Unlock()

	if a == nil {
		a = make(interceptor.Attributes)
	}

	// Parse RTP header
	packet := &rtp.Packet{}
	if err := packet.Unmarshal(b[:n]); err != nil {
		return n, a, nil // Pass through on parse error
	}

	// Process audio payload
	if len(packet.Payload) > 0 {
		processedPayload := r.processAudioPayload(packet.Payload)

		// Create new packet with processed payload
		newPacket := &rtp.Packet{
			Header:  packet.Header,
			Payload: processedPayload,
		}

		// Marshal the new packet
		newData, err := newPacket.Marshal()
		if err != nil {
			r.logger.Errorw("failed to marshal processed packet", err)
			return n, a, nil // Return original on error
		}

		// Copy processed data back to buffer
		if len(newData) <= len(b) {
			copy(b, newData)
			return len(newData), a, nil
		} else {
			r.logger.Warnw("processed packet too large for buffer", nil)
			return n, a, nil // Return original if too large
		}
	}

	return n, a, nil
}

// processAudioPayload applies noise suppression to audio data
func (r *noiseFilterReader) processAudioPayload(payload []byte) []byte {
	// For now, we'll assume the payload is PCM audio data
	// In a real implementation, you'd need to handle different codecs
	// and potentially decode before processing

	if len(payload) < rnnoiseFrameBytes {
		// Frame too small, pass through
		return payload
	}

	// Add to buffer
	r.buffer = append(r.buffer, payload...)

	var processedData []byte

	// Process complete frames
	for len(r.buffer) >= rnnoiseFrameBytes {
		frame := r.buffer[:rnnoiseFrameBytes]

		// Convert bytes to float32 samples (RNNoise expects float32)
		samples := make([]float32, rnnoiseFrameSize)
		for i := 0; i < rnnoiseFrameSize; i++ {
			// Convert int16 to float32 and normalize
			int16Val := int16(binary.LittleEndian.Uint16(frame[i*2:]))
			samples[i] = float32(int16Val) / 32768.0
		}

		// Apply noise suppression using FilterStream
		r.mu.Lock()
		if r.denoiser != nil {
			denoisedFrame, _, keepFrame, err := r.denoiser.FilterStream(samples, r.config.Threshold)
			if err == nil && keepFrame {
				// Convert back to int16
				for i, sample := range denoisedFrame {
					// Clamp and convert back to int16
					clampedSample := sample * 32768.0
					if clampedSample > 32767 {
						clampedSample = 32767
					} else if clampedSample < -32768 {
						clampedSample = -32768
					}
					samples[i] = clampedSample
				}
			} else if !keepFrame {
				// Apply noise reduction by reducing volume
				for i := range samples {
					samples[i] *= 0.1 // Reduce to 10% volume for noise frames
				}
			}
		}
		r.mu.Unlock()

		// Convert back to bytes
		processedFrame := make([]byte, rnnoiseFrameBytes)
		for i, sample := range samples {
			int16Val := int16(sample)
			binary.LittleEndian.PutUint16(processedFrame[i*2:], uint16(int16Val))
		}

		processedData = append(processedData, processedFrame...)

		// Remove processed frame from buffer
		r.buffer = r.buffer[rnnoiseFrameBytes:]
	}

	// Add remaining buffer back to processed data
	if len(r.buffer) > 0 {
		processedData = append(processedData, r.buffer...)
		r.buffer = r.buffer[:0] // Clear buffer but keep capacity
	}

	return processedData
}
