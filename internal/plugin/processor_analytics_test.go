package plugin

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/shawntherrien/databridge/pkg/types"
)

func TestNewProcessorAnalytics(t *testing.T) {
	logger := &types.MockLogger{}
	analytics := NewProcessorAnalytics(24*time.Hour, logger)

	assert.NotNil(t, analytics)
	assert.NotNil(t, analytics.processorMetrics)
	assert.NotNil(t, analytics.instanceMetrics)
	assert.NotNil(t, analytics.aggregateMetrics)
	assert.Equal(t, 24*time.Hour, analytics.retentionPeriod)
}

func TestRegisterInstance(t *testing.T) {
	logger := &types.MockLogger{}
	analytics := NewProcessorAnalytics(24*time.Hour, logger)

	instanceID := uuid.New()
	analytics.RegisterInstance(instanceID, "TestProcessor", "test-instance")

	// Verify processor metrics created
	procMetrics, err := analytics.GetProcessorMetrics("TestProcessor")
	assert.NoError(t, err)
	assert.Equal(t, "TestProcessor", procMetrics.ProcessorType)
	assert.Equal(t, 1, procMetrics.ActiveInstances)

	// Verify instance metrics created
	instMetrics, err := analytics.GetInstanceMetrics(instanceID)
	assert.NoError(t, err)
	assert.Equal(t, instanceID, instMetrics.InstanceID)
	assert.Equal(t, "TestProcessor", instMetrics.ProcessorType)
	assert.Equal(t, "test-instance", instMetrics.ProcessorName)
}

func TestUnregisterInstance(t *testing.T) {
	logger := &types.MockLogger{}
	analytics := NewProcessorAnalytics(24*time.Hour, logger)

	instanceID := uuid.New()
	analytics.RegisterInstance(instanceID, "TestProcessor", "test-instance")
	analytics.UnregisterInstance(instanceID)

	// Verify instance removed
	_, err := analytics.GetInstanceMetrics(instanceID)
	assert.Error(t, err)

	// Verify processor active instances decremented
	procMetrics, _ := analytics.GetProcessorMetrics("TestProcessor")
	assert.Equal(t, 0, procMetrics.ActiveInstances)
}

func TestRecordInvocation(t *testing.T) {
	logger := &types.MockLogger{}
	analytics := NewProcessorAnalytics(24*time.Hour, logger)

	instanceID := uuid.New()
	analytics.RegisterInstance(instanceID, "TestProcessor", "test-instance")

	// Record successful invocation
	analytics.RecordInvocation(
		instanceID,
		100*time.Millisecond,
		5,    // flowFilesIn
		5,    // flowFilesOut
		1024, // bytes
		true,
		nil,
	)

	// Verify instance metrics
	instMetrics, _ := analytics.GetInstanceMetrics(instanceID)
	assert.Equal(t, int64(1), instMetrics.TotalInvocations)
	assert.Equal(t, int64(5), instMetrics.TotalFlowFiles)
	assert.Equal(t, int64(1024), instMetrics.TotalBytes)
	assert.Equal(t, int64(1), instMetrics.TotalSuccesses)
	assert.Equal(t, int64(0), instMetrics.TotalFailures)
	assert.Equal(t, 100*time.Millisecond, instMetrics.MinExecTime)
	assert.Equal(t, 100*time.Millisecond, instMetrics.MaxExecTime)

	// Verify processor metrics
	procMetrics, _ := analytics.GetProcessorMetrics("TestProcessor")
	assert.Equal(t, int64(1), procMetrics.TotalInvocations)
	assert.Equal(t, int64(5), procMetrics.TotalFlowFiles)
	assert.Equal(t, int64(1024), procMetrics.TotalBytes)
}

func TestRecordInvocationWithError(t *testing.T) {
	logger := &types.MockLogger{}
	analytics := NewProcessorAnalytics(24*time.Hour, logger)

	instanceID := uuid.New()
	analytics.RegisterInstance(instanceID, "TestProcessor", "test-instance")

	// Record failed invocation
	testError := errors.New("test error")
	analytics.RecordInvocation(
		instanceID,
		50*time.Millisecond,
		3,
		0,
		0,
		false,
		testError,
	)

	instMetrics, _ := analytics.GetInstanceMetrics(instanceID)
	assert.Equal(t, int64(1), instMetrics.TotalInvocations)
	assert.Equal(t, int64(0), instMetrics.TotalSuccesses)
	assert.Equal(t, int64(1), instMetrics.TotalFailures)
	assert.Equal(t, int64(1), instMetrics.TotalErrors)
	assert.Len(t, instMetrics.errorMessages, 1)
	assert.Equal(t, "test error", instMetrics.errorMessages[0])
}

func TestRecordMultipleInvocations(t *testing.T) {
	logger := &types.MockLogger{}
	analytics := NewProcessorAnalytics(24*time.Hour, logger)

	instanceID := uuid.New()
	analytics.RegisterInstance(instanceID, "TestProcessor", "test-instance")

	// Record multiple invocations
	durations := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		150 * time.Millisecond,
		180 * time.Millisecond,
	}

	for _, d := range durations {
		analytics.RecordInvocation(instanceID, d, 1, 1, 100, true, nil)
	}

	instMetrics, _ := analytics.GetInstanceMetrics(instanceID)
	assert.Equal(t, int64(4), instMetrics.TotalInvocations)
	assert.Equal(t, 100*time.Millisecond, instMetrics.MinExecTime)
	assert.Equal(t, 200*time.Millisecond, instMetrics.MaxExecTime)

	// Average should be (100+200+150+180)/4 = 157.5ms
	expectedAvg := (100 + 200 + 150 + 180) * time.Millisecond / 4
	assert.Equal(t, expectedAvg, instMetrics.AverageExecTime)
}

func TestGetAggregateMetrics(t *testing.T) {
	logger := &types.MockLogger{}
	analytics := NewProcessorAnalytics(24*time.Hour, logger)

	// Register multiple instances
	id1 := uuid.New()
	id2 := uuid.New()
	analytics.RegisterInstance(id1, "Processor1", "instance1")
	analytics.RegisterInstance(id2, "Processor2", "instance2")

	// Record invocations
	analytics.RecordInvocation(id1, 100*time.Millisecond, 5, 5, 1024, true, nil)
	analytics.RecordInvocation(id2, 200*time.Millisecond, 3, 3, 512, true, nil)

	aggMetrics := analytics.GetAggregateMetrics()
	assert.Equal(t, 2, aggMetrics.TotalProcessors)
	assert.Equal(t, 2, aggMetrics.TotalInstances)
	assert.Equal(t, int64(2), aggMetrics.TotalInvocations)
	assert.Equal(t, int64(8), aggMetrics.TotalFlowFiles)
	assert.Equal(t, int64(1536), aggMetrics.TotalBytes)
}

func TestGetPerformanceStats(t *testing.T) {
	logger := &types.MockLogger{}
	analytics := NewProcessorAnalytics(24*time.Hour, logger)

	instanceID := uuid.New()
	analytics.RegisterInstance(instanceID, "TestProcessor", "test-instance")

	// Record multiple invocations to get meaningful stats
	durations := []time.Duration{
		100 * time.Millisecond,
		120 * time.Millisecond,
		150 * time.Millisecond,
		180 * time.Millisecond,
		200 * time.Millisecond,
	}

	for i, d := range durations {
		success := i < 4 // Last one fails
		var err error
		if !success {
			err = errors.New("test error")
		}
		analytics.RecordInvocation(instanceID, d, 1, 1, 100, success, err)
	}

	stats, err := analytics.GetPerformanceStats("TestProcessor")
	assert.NoError(t, err)
	assert.Equal(t, "TestProcessor", stats.ProcessorType)
	assert.NotZero(t, stats.Mean)
	assert.NotZero(t, stats.Median)
	assert.Equal(t, 80.0, stats.SuccessRate)
	assert.Equal(t, 20.0, stats.ErrorRate)
}

func TestGetTopProcessors(t *testing.T) {
	logger := &types.MockLogger{}
	analytics := NewProcessorAnalytics(24*time.Hour, logger)

	// Register and invoke multiple processors
	instances := []struct {
		id    uuid.UUID
		pType string
		count int
	}{
		{uuid.New(), "Processor1", 10},
		{uuid.New(), "Processor2", 5},
		{uuid.New(), "Processor3", 15},
	}

	for _, inst := range instances {
		analytics.RegisterInstance(inst.id, inst.pType, "instance")
		for i := 0; i < inst.count; i++ {
			analytics.RecordInvocation(inst.id, 100*time.Millisecond, 1, 1, 100, true, nil)
		}
	}

	// Get top 2 processors
	top := analytics.GetTopProcessors(2)
	assert.Len(t, top, 2)
	assert.Equal(t, "Processor3", top[0].ProcessorType)
	assert.Equal(t, int64(15), top[0].TotalInvocations)
	assert.Equal(t, "Processor1", top[1].ProcessorType)
	assert.Equal(t, int64(10), top[1].TotalInvocations)
}

func TestGetRecentErrors(t *testing.T) {
	logger := &types.MockLogger{}
	analytics := NewProcessorAnalytics(24*time.Hour, logger)

	instanceID := uuid.New()
	analytics.RegisterInstance(instanceID, "TestProcessor", "test-instance")

	// Record invocations with errors
	errorMessages := []string{"error 1", "error 2", "error 3"}
	for _, errMsg := range errorMessages {
		analytics.RecordInvocation(
			instanceID,
			100*time.Millisecond,
			1, 0, 0,
			false,
			errors.New(errMsg),
		)
	}

	recentErrors, err := analytics.GetRecentErrors(instanceID)
	assert.NoError(t, err)
	assert.Len(t, recentErrors, 3)
	assert.Contains(t, recentErrors, "error 1")
	assert.Contains(t, recentErrors, "error 2")
	assert.Contains(t, recentErrors, "error 3")
}

func TestGetRecentInvocations(t *testing.T) {
	logger := &types.MockLogger{}
	analytics := NewProcessorAnalytics(24*time.Hour, logger)

	instanceID := uuid.New()
	analytics.RegisterInstance(instanceID, "TestProcessor", "test-instance")

	// Record several invocations
	for i := 0; i < 5; i++ {
		analytics.RecordInvocation(
			instanceID,
			time.Duration(i*100)*time.Millisecond,
			i, i, int64(i*100),
			true,
			nil,
		)
	}

	// Get recent 3 invocations
	invocations, err := analytics.GetRecentInvocations(instanceID, 3)
	assert.NoError(t, err)
	assert.Len(t, invocations, 3)
	assert.True(t, invocations[0].Success)
}

func TestReset(t *testing.T) {
	logger := &types.MockLogger{}
	analytics := NewProcessorAnalytics(24*time.Hour, logger)

	instanceID := uuid.New()
	analytics.RegisterInstance(instanceID, "TestProcessor", "test-instance")
	analytics.RecordInvocation(instanceID, 100*time.Millisecond, 1, 1, 100, true, nil)

	// Reset analytics
	analytics.Reset()

	// Verify everything is cleared
	_, err := analytics.GetProcessorMetrics("TestProcessor")
	assert.Error(t, err)

	_, err = analytics.GetInstanceMetrics(instanceID)
	assert.Error(t, err)

	aggMetrics := analytics.GetAggregateMetrics()
	assert.Equal(t, 0, aggMetrics.TotalProcessors)
	assert.Equal(t, 0, aggMetrics.TotalInstances)
	assert.Equal(t, int64(0), aggMetrics.TotalInvocations)
}

func TestPurgeOldData(t *testing.T) {
	logger := &types.MockLogger{}
	analytics := NewProcessorAnalytics(1*time.Hour, logger)

	instanceID := uuid.New()
	analytics.RegisterInstance(instanceID, "TestProcessor", "test-instance")

	// Record invocations
	for i := 0; i < 5; i++ {
		analytics.RecordInvocation(instanceID, 100*time.Millisecond, 1, 1, 100, true, nil)
	}

	// Manually set old timestamps for testing
	analytics.mu.Lock()
	instance := analytics.instanceMetrics[instanceID]
	for i := 0; i < 3; i++ {
		instance.recentInvocations[i].Timestamp = time.Now().Add(-2 * time.Hour)
	}
	analytics.mu.Unlock()

	// Purge old data
	purged := analytics.PurgeOldData()
	assert.Equal(t, 3, purged)

	// Verify only recent invocations remain
	invocations, _ := analytics.GetRecentInvocations(instanceID, 10)
	assert.Len(t, invocations, 2)
}

func TestCalculateAverage(t *testing.T) {
	durations := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		300 * time.Millisecond,
	}

	avg := calculateAverage(durations)
	assert.Equal(t, 200*time.Millisecond, avg)

	// Empty slice
	avg = calculateAverage([]time.Duration{})
	assert.Equal(t, time.Duration(0), avg)
}

func TestSortDurations(t *testing.T) {
	durations := []time.Duration{
		300 * time.Millisecond,
		100 * time.Millisecond,
		200 * time.Millisecond,
	}

	sortDurations(durations)

	assert.Equal(t, 100*time.Millisecond, durations[0])
	assert.Equal(t, 200*time.Millisecond, durations[1])
	assert.Equal(t, 300*time.Millisecond, durations[2])
}

func TestSortProcessorMetrics(t *testing.T) {
	metrics := []*ProcessorMetrics{
		{ProcessorType: "Proc1", TotalInvocations: 5},
		{ProcessorType: "Proc2", TotalInvocations: 10},
		{ProcessorType: "Proc3", TotalInvocations: 3},
	}

	sortProcessorMetrics(metrics)

	assert.Equal(t, int64(10), metrics[0].TotalInvocations)
	assert.Equal(t, int64(5), metrics[1].TotalInvocations)
	assert.Equal(t, int64(3), metrics[2].TotalInvocations)
}

func TestInvocationRecord(t *testing.T) {
	record := InvocationRecord{
		Timestamp:      time.Now(),
		Duration:       100 * time.Millisecond,
		FlowFilesIn:    5,
		FlowFilesOut:   5,
		BytesProcessed: 1024,
		Success:        true,
		Error:          "",
	}

	assert.True(t, record.Success)
	assert.Equal(t, 5, record.FlowFilesIn)
	assert.Equal(t, int64(1024), record.BytesProcessed)
	assert.Empty(t, record.Error)
}

func TestProcessorMetricsInitialization(t *testing.T) {
	logger := &types.MockLogger{}
	analytics := NewProcessorAnalytics(24*time.Hour, logger)

	instanceID := uuid.New()
	analytics.RegisterInstance(instanceID, "TestProcessor", "test-instance")

	procMetrics, _ := analytics.GetProcessorMetrics("TestProcessor")

	assert.Equal(t, "TestProcessor", procMetrics.ProcessorType)
	assert.Equal(t, int64(0), procMetrics.TotalInvocations)
	assert.Equal(t, 1, procMetrics.ActiveInstances)
	assert.NotZero(t, procMetrics.FirstInvoked)
	assert.Equal(t, time.Duration(1<<63-1), procMetrics.MinExecTime)
}

func TestInstanceMetricsInitialization(t *testing.T) {
	logger := &types.MockLogger{}
	analytics := NewProcessorAnalytics(24*time.Hour, logger)

	instanceID := uuid.New()
	analytics.RegisterInstance(instanceID, "TestProcessor", "test-instance")

	instMetrics, _ := analytics.GetInstanceMetrics(instanceID)

	assert.Equal(t, instanceID, instMetrics.InstanceID)
	assert.Equal(t, "TestProcessor", instMetrics.ProcessorType)
	assert.Equal(t, "test-instance", instMetrics.ProcessorName)
	assert.NotZero(t, instMetrics.CreatedAt)
	assert.Equal(t, time.Duration(1<<63-1), instMetrics.MinExecTime)
}

func TestErrorMessageLimit(t *testing.T) {
	logger := &types.MockLogger{}
	analytics := NewProcessorAnalytics(24*time.Hour, logger)

	instanceID := uuid.New()
	analytics.RegisterInstance(instanceID, "TestProcessor", "test-instance")

	// Record more than 10 errors (the limit)
	for i := 0; i < 15; i++ {
		analytics.RecordInvocation(
			instanceID,
			100*time.Millisecond,
			1, 0, 0,
			false,
			errors.New("test error"),
		)
	}

	errors, _ := analytics.GetRecentErrors(instanceID)
	assert.LessOrEqual(t, len(errors), 10)
}

func TestRecentInvocationsLimit(t *testing.T) {
	logger := &types.MockLogger{}
	analytics := NewProcessorAnalytics(24*time.Hour, logger)

	instanceID := uuid.New()
	analytics.RegisterInstance(instanceID, "TestProcessor", "test-instance")

	// Record more than 50 invocations (the limit)
	for i := 0; i < 60; i++ {
		analytics.RecordInvocation(instanceID, 100*time.Millisecond, 1, 1, 100, true, nil)
	}

	invocations, _ := analytics.GetRecentInvocations(instanceID, 100)
	assert.LessOrEqual(t, len(invocations), 50)
}

func TestExecTimesLimit(t *testing.T) {
	logger := &types.MockLogger{}
	analytics := NewProcessorAnalytics(24*time.Hour, logger)

	instanceID := uuid.New()
	analytics.RegisterInstance(instanceID, "TestProcessor", "test-instance")

	// Record many invocations
	for i := 0; i < 150; i++ {
		analytics.RecordInvocation(instanceID, 100*time.Millisecond, 1, 1, 100, true, nil)
	}

	// Verify exec times array is limited
	analytics.mu.RLock()
	instance := analytics.instanceMetrics[instanceID]
	assert.LessOrEqual(t, len(instance.execTimes), 100)
	analytics.mu.RUnlock()
}
