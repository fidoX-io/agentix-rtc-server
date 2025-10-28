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
	"testing"

	"github.com/livekit/livekit-server/pkg/sfu/audio"
	"github.com/livekit/protocol/logger"
	"github.com/pion/interceptor"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoiseFilterFactory(t *testing.T) {
	testLogger := logger.GetLogger()

	tests := []struct {
		name   string
		config audio.NoiseFilterConfig
	}{
		{
			name: "enabled noise filter",
			config: audio.NoiseFilterConfig{
				Enabled:    true,
				Threshold:  0.5,
				Aggressive: false,
			},
		},
		{
			name: "disabled noise filter",
			config: audio.NoiseFilterConfig{
				Enabled:    false,
				Threshold:  0.5,
				Aggressive: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory := NewNoiseFilterFactory(tt.config, testLogger)
			require.NotNil(t, factory)

			interceptor, err := factory.NewInterceptor("")
			require.NoError(t, err)
			require.NotNil(t, interceptor)
			assert.IsType(t, &NoiseFilterInterceptor{}, interceptor)

			// Verify config is properly stored
			assert.Equal(t, tt.config, factory.GetConfig())
		})
	}
}

func TestNoiseFilterInterceptor_BindRTCPReader(t *testing.T) {
	testLogger := logger.GetLogger()
	config := audio.NoiseFilterConfig{
		Enabled:    true,
		Threshold:  0.5,
		Aggressive: false,
	}

	factory := NewNoiseFilterFactory(config, testLogger)
	i, err := factory.NewInterceptor("")
	require.NoError(t, err)
	require.NotNil(t, i)

	nfInterceptor := i.(*NoiseFilterInterceptor)

	// Test RTCP reader binding (should pass through)
	reader := nfInterceptor.BindRTCPReader(interceptor.RTCPReaderFunc(func(b []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {
		return len(b), a, nil
	}))

	require.NotNil(t, reader)

	// Test that RTCP packets pass through unchanged
	testData := []byte{0x80, 0xc8, 0x00, 0x06} // Simple RTCP header
	n, attrs, err := reader.Read(testData, nil)
	assert.NoError(t, err)
	assert.Equal(t, len(testData), n)
	assert.Nil(t, attrs)
}

func TestNoiseFilterInterceptor_BindLocalStream(t *testing.T) {
	testLogger := logger.GetLogger()
	config := audio.NoiseFilterConfig{
		Enabled:    true,
		Threshold:  0.5,
		Aggressive: false,
	}

	factory := NewNoiseFilterFactory(config, testLogger)
	i, err := factory.NewInterceptor("")
	require.NoError(t, err)
	require.NotNil(t, i)

	nfInterceptor := i.(*NoiseFilterInterceptor)

	// Test local stream binding (should pass through for RTP writer)
	info := &interceptor.StreamInfo{
		SSRC:                12345,
		PayloadType:         111, // Opus
		RTPHeaderExtensions: []interceptor.RTPHeaderExtension{},
	}

	writer := nfInterceptor.BindLocalStream(info, interceptor.RTPWriterFunc(func(header *rtp.Header, payload []byte, a interceptor.Attributes) (int, error) {
		return len(payload), nil
	}))

	require.NotNil(t, writer)

	// Test RTP packet writing (should pass through unchanged for local streams)
	header := &rtp.Header{
		Version:     2,
		PayloadType: 111,
		SSRC:        12345,
		Timestamp:   1000,
	}
	testPayload := make([]byte, 160) // 20ms of audio at 8kHz

	n, err := writer.Write(header, testPayload, nil)
	assert.NoError(t, err)
	assert.Equal(t, len(testPayload), n)
}

func TestNoiseFilterInterceptor_BindRemoteStream(t *testing.T) {
	testLogger := logger.GetLogger()
	config := audio.NoiseFilterConfig{
		Enabled:    true,
		Threshold:  0.5,
		Aggressive: false,
	}

	factory := NewNoiseFilterFactory(config, testLogger)
	i, err := factory.NewInterceptor("")
	require.NoError(t, err)
	require.NotNil(t, i)

	nfInterceptor := i.(*NoiseFilterInterceptor)

	// Test remote stream binding for audio (with audio level extension)
	info := &interceptor.StreamInfo{
		SSRC:        12345,
		PayloadType: 111, // Opus
		RTPHeaderExtensions: []interceptor.RTPHeaderExtension{
			{
				ID:  1,
				URI: "urn:ietf:params:rtp-hdrext:ssrc-audio-level",
			},
		},
	}

	callCount := 0
	reader := nfInterceptor.BindRemoteStream(info, interceptor.RTPReaderFunc(func(b []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {
		callCount++
		return len(b), a, nil
	}))

	require.NotNil(t, reader)
	assert.IsType(t, &noiseFilterReader{}, reader)

	// Test that the reader was created properly
	nfReader := reader.(*noiseFilterReader)
	assert.NotNil(t, nfReader.reader)
	assert.Equal(t, config, nfReader.config)
}

func TestNoiseFilterInterceptor_BindRemoteStream_NonAudio(t *testing.T) {
	testLogger := logger.GetLogger()
	config := audio.NoiseFilterConfig{
		Enabled:    true,
		Threshold:  0.5,
		Aggressive: false,
	}

	factory := NewNoiseFilterFactory(config, testLogger)
	i, err := factory.NewInterceptor("")
	require.NoError(t, err)
	require.NotNil(t, i)

	nfInterceptor := i.(*NoiseFilterInterceptor)

	// Test remote stream binding for video (should pass through)
	info := &interceptor.StreamInfo{
		SSRC:                12345,
		PayloadType:         96, // H.264
		RTPHeaderExtensions: []interceptor.RTPHeaderExtension{},
	}

	originalReader := interceptor.RTPReaderFunc(func(b []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {
		return len(b), a, nil
	})

	reader := nfInterceptor.BindRemoteStream(info, originalReader)
	require.NotNil(t, reader)

	// For non-audio streams, should return the original reader
	// We can't use direct comparison for function types, so test behavior instead
	testBuffer := make([]byte, 100)
	n, _, err := reader.Read(testBuffer, nil)
	assert.NoError(t, err)
	assert.Equal(t, 100, n) // Should read the full buffer size
}

func TestNoiseFilterReader_Read(t *testing.T) {
	testLogger := logger.GetLogger()
	config := audio.NoiseFilterConfig{
		Enabled:    true,
		Threshold:  0.5,
		Aggressive: false,
	}

	// Create a mock reader that returns test RTP packets
	mockReader := interceptor.RTPReaderFunc(func(b []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {
		// Create a mock Opus RTP packet
		header := &rtp.Header{
			Version:        2,
			PayloadType:    111,
			SSRC:           12345,
			Timestamp:      48000, // 1 second at 48kHz
			SequenceNumber: 1,
		}

		// Create mock Opus payload (should be at least 120 bytes for 480 samples)
		payload := make([]byte, 120)
		for i := range payload {
			payload[i] = byte(i % 256) // Some test data
		}

		packet, err := header.Marshal()
		if err != nil {
			return 0, nil, err
		}
		packet = append(packet, payload...)

		copy(b, packet)
		return len(packet), a, nil
	})

	reader := &noiseFilterReader{
		reader: mockReader,
		config: config,
		logger: testLogger,
		buffer: make([]byte, 0, rnnoiseFrameBytes*2),
	}

	// Test reading a packet
	buffer := make([]byte, 1500)
	n, attrs, err := reader.Read(buffer, nil)

	// Should succeed (even if RNNoise processing fails, packet should pass through)
	assert.NoError(t, err)
	assert.Greater(t, n, 0)
	// Attributes should be created if they were nil
	if attrs != nil {
		assert.IsType(t, interceptor.Attributes{}, attrs)
	}
}

// Benchmark tests for performance
func BenchmarkNoiseFilterReader_Read(b *testing.B) {
	testLogger := logger.GetLogger()
	config := audio.NoiseFilterConfig{
		Enabled:    true,
		Threshold:  0.5,
		Aggressive: false,
	}

	// Create a mock reader that returns test RTP packets
	mockReader := interceptor.RTPReaderFunc(func(buf []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {
		// Create a realistic Opus RTP packet
		header := &rtp.Header{
			Version:        2,
			PayloadType:    111,
			SSRC:           12345,
			Timestamp:      1000,
			SequenceNumber: 1,
		}

		payload := make([]byte, 160) // Typical Opus frame size
		for i := range payload {
			payload[i] = byte(i % 256)
		}

		packet, err := header.Marshal()
		if err != nil {
			return 0, nil, err
		}
		packet = append(packet, payload...)

		copy(buf, packet)
		return len(packet), a, nil
	})

	reader := &noiseFilterReader{
		reader: mockReader,
		config: config,
		logger: testLogger,
		buffer: make([]byte, 0, rnnoiseFrameBytes*2),
	}

	buffer := make([]byte, 1500)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := reader.Read(buffer, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}
