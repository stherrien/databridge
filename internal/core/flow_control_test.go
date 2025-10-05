package core

import (
	"testing"
	"time"

	"github.com/shawntherrien/databridge/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultBackPressureConfig(t *testing.T) {
	config := DefaultBackPressureConfig()

	assert.True(t, config.Enabled)
	assert.Equal(t, BackPressureBlock, config.Strategy)
	assert.Equal(t, 80, config.ThresholdPct)
	assert.Equal(t, 1*time.Second, config.PenaltyDuration)
}

func TestBackPressureConfig_ShouldTrigger(t *testing.T) {
	tests := []struct {
		name        string
		config      BackPressureConfig
		currentSize int64
		maxSize     int64
		expected    bool
	}{
		{
			name:        "below threshold",
			config:      BackPressureConfig{Enabled: true, ThresholdPct: 80},
			currentSize: 50,
			maxSize:     100,
			expected:    false,
		},
		{
			name:        "at threshold",
			config:      BackPressureConfig{Enabled: true, ThresholdPct: 80},
			currentSize: 80,
			maxSize:     100,
			expected:    true,
		},
		{
			name:        "above threshold",
			config:      BackPressureConfig{Enabled: true, ThresholdPct: 80},
			currentSize: 90,
			maxSize:     100,
			expected:    true,
		},
		{
			name:        "disabled",
			config:      BackPressureConfig{Enabled: false, ThresholdPct: 80},
			currentSize: 90,
			maxSize:     100,
			expected:    false,
		},
		{
			name:        "max size zero",
			config:      BackPressureConfig{Enabled: true, ThresholdPct: 80},
			currentSize: 50,
			maxSize:     0,
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.ShouldTrigger(tt.currentSize, tt.maxSize)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFlowFileQueue_EnqueueWithBackPressure_Block(t *testing.T) {
	queue := &FlowFileQueue{
		flowFiles:   make([]*types.FlowFile, 0),
		maxSize:     10,
		currentSize: 0,
		metrics:     &BackPressureMetrics{},
	}

	config := BackPressureConfig{
		Enabled:      true,
		Strategy:     BackPressureBlock,
		ThresholdPct: 80,
	}

	// Enqueue below threshold - should succeed immediately
	for i := 0; i < 7; i++ {
		ff := types.NewFlowFile()
		err := queue.EnqueueWithBackPressure(ff, config)
		require.NoError(t, err)
	}

	assert.Equal(t, int64(7), queue.currentSize)
	assert.Equal(t, int64(0), queue.metrics.TriggeredCount)

	// Enqueue at threshold - should block but eventually succeed
	// We'll use a goroutine to dequeue and make space
	go func() {
		time.Sleep(100 * time.Millisecond)
		queue.Dequeue()
	}()

	ff := types.NewFlowFile()
	err := queue.EnqueueWithBackPressure(ff, config)
	require.NoError(t, err)

	assert.Equal(t, int64(1), queue.metrics.TriggeredCount)
}

func TestFlowFileQueue_EnqueueWithBackPressure_Drop(t *testing.T) {
	queue := &FlowFileQueue{
		flowFiles:   make([]*types.FlowFile, 0),
		maxSize:     10,
		currentSize: 0,
		metrics:     &BackPressureMetrics{},
	}

	config := BackPressureConfig{
		Enabled:      true,
		Strategy:     BackPressureDrop,
		ThresholdPct: 80,
	}

	// Fill queue below threshold (7 items = 70%)
	for i := 0; i < 7; i++ {
		ff := types.NewFlowFile()
		err := queue.EnqueueWithBackPressure(ff, config)
		require.NoError(t, err)
	}

	assert.Equal(t, int64(7), queue.currentSize)

	// Enqueue at threshold (8 items = 80%) - should trigger and drop oldest
	ff := types.NewFlowFile()
	err := queue.EnqueueWithBackPressure(ff, config)
	require.NoError(t, err)

	assert.Equal(t, int64(7), queue.currentSize) // Still 7 after drop+add
	assert.Equal(t, int64(1), queue.metrics.DroppedCount)
	assert.Equal(t, int64(1), queue.metrics.TriggeredCount)

	// Enqueue one more - should trigger again and drop oldest
	ff = types.NewFlowFile()
	err = queue.EnqueueWithBackPressure(ff, config)
	require.NoError(t, err)

	assert.Equal(t, int64(7), queue.currentSize) // Still 7 after drop+add
	assert.Equal(t, int64(2), queue.metrics.DroppedCount)
	assert.Equal(t, int64(2), queue.metrics.TriggeredCount)
}

func TestFlowFileQueue_EnqueueWithBackPressure_Fail(t *testing.T) {
	queue := &FlowFileQueue{
		flowFiles:   make([]*types.FlowFile, 0),
		maxSize:     10,
		currentSize: 0,
		metrics:     &BackPressureMetrics{},
	}

	config := BackPressureConfig{
		Enabled:      true,
		Strategy:     BackPressureFail,
		ThresholdPct: 80,
	}

	// Fill queue below threshold
	for i := 0; i < 7; i++ {
		ff := types.NewFlowFile()
		err := queue.EnqueueWithBackPressure(ff, config)
		require.NoError(t, err)
	}

	assert.Equal(t, int64(7), queue.currentSize)

	// Enqueue at threshold - should fail
	ff := types.NewFlowFile()
	err := queue.EnqueueWithBackPressure(ff, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "back pressure triggered")

	assert.Equal(t, int64(7), queue.currentSize)
	assert.Equal(t, int64(1), queue.metrics.TriggeredCount)
}

func TestFlowFileQueue_EnqueueWithBackPressure_Penalty(t *testing.T) {
	conn := &Connection{
		Name: "test-connection",
	}

	queue := &FlowFileQueue{
		connection:  conn,
		flowFiles:   make([]*types.FlowFile, 0),
		maxSize:     10,
		currentSize: 0,
		metrics:     &BackPressureMetrics{},
	}

	config := BackPressureConfig{
		Enabled:         true,
		Strategy:        BackPressurePenalty,
		ThresholdPct:    80,
		PenaltyDuration: 100 * time.Millisecond,
	}

	// Fill queue below threshold
	for i := 0; i < 7; i++ {
		ff := types.NewFlowFile()
		err := queue.EnqueueWithBackPressure(ff, config)
		require.NoError(t, err)
	}

	assert.Equal(t, int64(7), queue.currentSize)

	// Enqueue at threshold - should succeed but apply penalty
	ff := types.NewFlowFile()
	err := queue.EnqueueWithBackPressure(ff, config)
	require.NoError(t, err)

	assert.Equal(t, int64(8), queue.currentSize)
	assert.Equal(t, int64(1), queue.metrics.PenaltyCount)
	assert.True(t, conn.IsPenalized())

	// Wait for penalty to expire
	time.Sleep(150 * time.Millisecond)
	assert.False(t, conn.IsPenalized())
}

func TestFlowFileQueue_GetBackPressureMetrics(t *testing.T) {
	queue := &FlowFileQueue{
		flowFiles:   make([]*types.FlowFile, 0),
		maxSize:     10,
		currentSize: 0,
		metrics:     &BackPressureMetrics{},
	}

	config := BackPressureConfig{
		Enabled:      true,
		Strategy:     BackPressureDrop,
		ThresholdPct: 80,
	}

	// Trigger back pressure
	for i := 0; i < 9; i++ {
		ff := types.NewFlowFile()
		queue.EnqueueWithBackPressure(ff, config)
	}

	metrics := queue.GetBackPressureMetrics()
	assert.Equal(t, int64(2), metrics.TriggeredCount) // Items 8 and 9 trigger
	assert.Equal(t, int64(2), metrics.DroppedCount) // Items 8 and 9 drop oldest
	assert.False(t, metrics.LastTriggered.IsZero())
}

func TestFlowFileQueue_ResetBackPressureMetrics(t *testing.T) {
	queue := &FlowFileQueue{
		flowFiles:   make([]*types.FlowFile, 0),
		maxSize:     10,
		currentSize: 0,
		metrics:     &BackPressureMetrics{},
	}

	config := BackPressureConfig{
		Enabled:      true,
		Strategy:     BackPressureDrop,
		ThresholdPct: 80,
	}

	// Trigger back pressure
	for i := 0; i < 9; i++ {
		ff := types.NewFlowFile()
		queue.EnqueueWithBackPressure(ff, config)
	}

	// Reset metrics
	queue.ResetBackPressureMetrics()

	metrics := queue.GetBackPressureMetrics()
	assert.Equal(t, int64(0), metrics.TriggeredCount)
	assert.Equal(t, int64(0), metrics.DroppedCount)
	assert.True(t, metrics.LastTriggered.IsZero())
}

func TestFlowFileQueue_GetBackPressureStatus(t *testing.T) {
	tests := []struct {
		name        string
		currentSize int64
		maxSize     int64
		config      BackPressureConfig
		expected    string
	}{
		{
			name:        "normal",
			currentSize: 5,
			maxSize:     10,
			config:      BackPressureConfig{Enabled: true, ThresholdPct: 80, Strategy: BackPressureBlock},
			expected:    "Normal (50%)",
		},
		{
			name:        "active",
			currentSize: 9,
			maxSize:     10,
			config:      BackPressureConfig{Enabled: true, ThresholdPct: 80, Strategy: BackPressureBlock},
			expected:    "Active (90%, strategy: block)",
		},
		{
			name:        "disabled",
			currentSize: 9,
			maxSize:     10,
			config:      BackPressureConfig{Enabled: false, ThresholdPct: 80, Strategy: BackPressureBlock},
			expected:    "Disabled (90%)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queue := &FlowFileQueue{
				flowFiles:   make([]*types.FlowFile, 0),
				maxSize:     tt.maxSize,
				currentSize: tt.currentSize,
				metrics:     &BackPressureMetrics{},
			}

			status := queue.GetBackPressureStatus(tt.config)
			assert.Equal(t, tt.expected, status)
		})
	}
}

func TestConnection_IsPenalized(t *testing.T) {
	conn := &Connection{
		Name: "test-connection",
	}

	// Not penalized initially
	assert.False(t, conn.IsPenalized())

	// Set penalty
	now := time.Now()
	conn.lastPenaltyTime = &now
	conn.penaltyDuration = 100 * time.Millisecond

	assert.True(t, conn.IsPenalized())

	// Wait for penalty to expire
	time.Sleep(150 * time.Millisecond)
	assert.False(t, conn.IsPenalized())
}

func TestConnection_GetPenaltyRemaining(t *testing.T) {
	conn := &Connection{
		Name: "test-connection",
	}

	// No penalty
	assert.Equal(t, time.Duration(0), conn.GetPenaltyRemaining())

	// Set penalty
	now := time.Now()
	conn.lastPenaltyTime = &now
	conn.penaltyDuration = 200 * time.Millisecond

	remaining := conn.GetPenaltyRemaining()
	assert.True(t, remaining > 0)
	assert.True(t, remaining <= 200*time.Millisecond)

	// Wait for penalty to expire
	time.Sleep(250 * time.Millisecond)
	assert.Equal(t, time.Duration(0), conn.GetPenaltyRemaining())
}

func TestConnection_ClearPenalty(t *testing.T) {
	conn := &Connection{
		Name: "test-connection",
	}

	// Set penalty
	now := time.Now()
	conn.lastPenaltyTime = &now
	conn.penaltyDuration = 100 * time.Millisecond

	assert.True(t, conn.IsPenalized())

	// Clear penalty
	conn.ClearPenalty()

	assert.False(t, conn.IsPenalized())
	assert.Nil(t, conn.lastPenaltyTime)
	assert.Equal(t, time.Duration(0), conn.penaltyDuration)
}

func TestConnection_GetBackPressureInfo(t *testing.T) {
	queue := &FlowFileQueue{
		flowFiles:   make([]*types.FlowFile, 0),
		maxSize:     100,
		currentSize: 75,
		metrics:     &BackPressureMetrics{},
	}

	conn := &Connection{
		Name:  "test-connection",
		Queue: queue,
	}

	config := BackPressureConfig{
		Enabled:      true,
		Strategy:     BackPressureBlock,
		ThresholdPct: 80,
	}

	info := conn.GetBackPressureInfo(config)

	assert.Equal(t, conn.Name, info.ConnectionName)
	assert.Equal(t, int64(75), info.CurrentSize)
	assert.Equal(t, int64(100), info.MaxSize)
	assert.Equal(t, int64(75), info.PercentageFull)
	assert.Equal(t, "Normal (75%)", info.BackPressureStatus)
	assert.False(t, info.IsPenalized)
}
