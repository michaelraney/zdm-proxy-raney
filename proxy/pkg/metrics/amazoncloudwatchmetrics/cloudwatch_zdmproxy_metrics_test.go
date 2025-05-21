package amazoncloudwatchmetrics

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/datastax/zdm-proxy/proxy/pkg/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestFactory(t *testing.T) (*CloudWatchMetricFactory, *MockCloudWatchClient) {
	mockClient := &MockCloudWatchClient{}
	factory := &CloudWatchMetricFactory{
		client:                mockClient,
		namespace:             "TestNamespace",
		metricsPrefix:         "TestPrefix",
		reportIntervalMinutes: 1,
		lock:                  &sync.Mutex{},
		metrics:               make([]reporter, 0),
		stopChan:              make(chan struct{}),
	}
	return factory, mockClient
}

func TestNewCloudWatchMetricFactory(t *testing.T) {
	tests := []struct {
		name              string
		region            string
		namespace         string
		metricsPrefix     string
		reportInterval    int16
		expectedError     bool
		expectedNamespace string
		expectedPrefix    string
		expectedInterval  int16
	}{
		{
			name:              "Valid configuration",
			region:            "us-west-2",
			namespace:         "TestNamespace",
			metricsPrefix:     "test",
			reportInterval:    1,
			expectedError:     false,
			expectedNamespace: "TestNamespace",
			expectedPrefix:    "test",
			expectedInterval:  1,
		},
		{
			name:              "Empty region",
			region:            "",
			namespace:         "TestNamespace",
			metricsPrefix:     "test",
			reportInterval:    1,
			expectedError:     false,
			expectedNamespace: "TestNamespace",
			expectedPrefix:    "test",
			expectedInterval:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, err := NewCloudWatchMetricFactory(tt.region, tt.namespace, tt.metricsPrefix, tt.reportInterval)
			if tt.expectedError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, factory)
			assert.Equal(t, tt.expectedNamespace, factory.namespace)
			assert.Equal(t, tt.expectedPrefix, factory.metricsPrefix)
			assert.Equal(t, tt.expectedInterval, factory.reportIntervalMinutes)
			factory.Stop() // Make sure to stop the reporting loop
		})
	}
}

func TestCloudWatchMetricFactory_GetOrCreateCounter(t *testing.T) {
	factory, mockClient := setupTestFactory(t)
	defer factory.Stop()

	tests := []struct {
		name      string
		metric    metrics.Metric
		addValue  int
		expected  float64
		withLabel bool
	}{
		{
			name:      "Simple Counter",
			metric:    metrics.NewMetric("test_counter", "test_counter description"),
			addValue:  5,
			expected:  5,
			withLabel: false,
		},
		{
			name:      "Labeled Counter",
			metric:    metrics.NewMetricWithLabels("test_counter", "test_counter description", map[string]string{"label": "value"}),
			addValue:  3,
			expected:  3,
			withLabel: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fmt.Printf("Running test case: %s\n", tt.name)
			// Clear both the mock client and factory metrics
			mockClient.Clear()
			factory.lock.Lock()
			factory.metrics = make([]reporter, 0)
			factory.lock.Unlock()

			counter, err := factory.GetOrCreateCounter(tt.metric)
			require.NoError(t, err)
			require.NotNil(t, counter)

			fmt.Printf("Adding value %d to counter\n", tt.addValue)
			counter.Add(tt.addValue)

			fmt.Printf("Triggering reporting\n")
			factory.reportMetrics()

			inputs := mockClient.GetPutMetricDataInputs()
			fmt.Printf("Number of PutMetricData calls: %d\n", len(inputs))
			require.Len(t, inputs, 1, "Expected one PutMetricData call")

			metricData := inputs[0].MetricData
			fmt.Printf("Number of metric data points: %d\n", len(metricData))
			require.Len(t, metricData, 1, "Expected one metric datum")

			fmt.Printf("Metric value: %v\n", *metricData[0].Value)
			assert.Equal(t, tt.expected, *metricData[0].Value)
			if tt.withLabel {
				require.Len(t, metricData[0].Dimensions, 1)
				assert.Equal(t, "label", *metricData[0].Dimensions[0].Name)
				assert.Equal(t, "value", *metricData[0].Dimensions[0].Value)
			}
		})
	}
}

func TestCloudWatchMetricFactory_GetOrCreateGauge(t *testing.T) {
	factory, mockClient := setupTestFactory(t)
	defer factory.Stop()

	tests := []struct {
		name      string
		metric    metrics.Metric
		setValue  int
		expected  float64
		withLabel bool
	}{
		{
			name:      "Simple Gauge",
			metric:    metrics.NewMetric("test_gauge", "test_gauge description"),
			setValue:  10,
			expected:  10,
			withLabel: false,
		},
		{
			name:      "Labeled Gauge",
			metric:    metrics.NewMetricWithLabels("test_gauge", "test_gauge description", map[string]string{"label": "value"}),
			setValue:  7,
			expected:  7,
			withLabel: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fmt.Printf("Running test case: %s\n", tt.name)
			// Clear both the mock client and factory metrics
			mockClient.Clear()
			factory.lock.Lock()
			factory.metrics = make([]reporter, 0)
			factory.lock.Unlock()

			gauge, err := factory.GetOrCreateGauge(tt.metric)
			require.NoError(t, err)
			require.NotNil(t, gauge)

			fmt.Printf("Setting gauge value to %d\n", tt.setValue)
			gauge.Set(tt.setValue)

			fmt.Printf("Triggering reporting\n")
			factory.reportMetrics()

			inputs := mockClient.GetPutMetricDataInputs()
			fmt.Printf("Number of PutMetricData calls: %d\n", len(inputs))
			require.Len(t, inputs, 1, "Expected one PutMetricData call")

			metricData := inputs[0].MetricData
			fmt.Printf("Number of metric data points: %d\n", len(metricData))
			require.Len(t, metricData, 1, "Expected one metric datum")

			fmt.Printf("Metric value: %v\n", *metricData[0].Value)
			assert.Equal(t, tt.expected, *metricData[0].Value)
			if tt.withLabel {
				require.Len(t, metricData[0].Dimensions, 1)
				assert.Equal(t, "label", *metricData[0].Dimensions[0].Name)
				assert.Equal(t, "value", *metricData[0].Dimensions[0].Value)
			}
		})
	}
}

func TestCloudWatchMetricFactory_GetOrCreateHistogram(t *testing.T) {
	factory, mockClient := setupTestFactory(t)
	defer factory.Stop()

	tests := []struct {
		name      string
		metric    metrics.Metric
		values    []time.Duration
		buckets   []float64
		withLabel bool
	}{
		{
			name:      "Simple Histogram",
			metric:    metrics.NewMetric("test_histogram", "test_histogram description"),
			values:    []time.Duration{100 * time.Millisecond, 200 * time.Millisecond},
			buckets:   []float64{0.5, 0.9, 0.99},
			withLabel: false,
		},
		{
			name:      "Labeled Histogram",
			metric:    metrics.NewMetricWithLabels("test_histogram", "test_histogram description", map[string]string{"label": "value"}),
			values:    []time.Duration{150 * time.Millisecond, 250 * time.Millisecond},
			buckets:   []float64{0.5, 0.9, 0.99},
			withLabel: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fmt.Printf("Running test case: %s\n", tt.name)
			// Clear both the mock client and factory metrics
			mockClient.Clear()
			factory.lock.Lock()
			factory.metrics = make([]reporter, 0)
			factory.lock.Unlock()

			histogram, err := factory.GetOrCreateHistogram(tt.metric, tt.buckets)
			require.NoError(t, err)
			require.NotNil(t, histogram)

			fmt.Printf("Tracking %d values\n", len(tt.values))
			for _, value := range tt.values {
				histogram.Track(time.Now().Add(-value))
			}

			fmt.Printf("Triggering reporting\n")
			factory.reportMetrics()

			inputs := mockClient.GetPutMetricDataInputs()
			fmt.Printf("Number of PutMetricData calls: %d\n", len(inputs))
			require.Len(t, inputs, 1, "Expected one PutMetricData call")

			metricData := inputs[0].MetricData
			fmt.Printf("Number of metric data points: %d\n", len(metricData))
			require.Greater(t, len(metricData), 1, "Expected multiple metric data points")

			// Verify summary statistics
			summary := metricData[0]
			fmt.Printf("Sample count: %v\n", *summary.StatisticValues.SampleCount)
			assert.Equal(t, float64(len(tt.values)), *summary.StatisticValues.SampleCount)

			// Verify percentiles
			for i, bucket := range tt.buckets {
				percentileMetric := metricData[i+1]
				fmt.Printf("Percentile %v: %v\n", bucket, *percentileMetric.Value)
				assert.Contains(t, *percentileMetric.MetricName, "_p")
				assert.NotNil(t, percentileMetric.Value)
			}

			if tt.withLabel {
				for _, metric := range metricData {
					require.Len(t, metric.Dimensions, 1)
					assert.Equal(t, "label", *metric.Dimensions[0].Name)
					assert.Equal(t, "value", *metric.Dimensions[0].Value)
				}
			}
		})
	}
}

func TestCloudWatchMetricFactory_Stop(t *testing.T) {
	factory, _ := setupTestFactory(t)

	// Start the reporting loop
	go factory.reportingLoop()

	// Stop the factory
	factory.Stop()

	// Verify the stop channel is closed
	select {
	case <-factory.stopChan:
		// Channel is closed, which is what we want
	default:
		t.Error("Stop channel should be closed")
	}
}

func TestCloudWatchMetricFactory_UnregisterAllMetrics(t *testing.T) {
	factory, _ := setupTestFactory(t)

	// UnregisterAllMetrics should be a no-op for CloudWatch
	err := factory.UnregisterAllMetrics()
	assert.NoError(t, err)
}

func TestCloudWatchMetricFactory_HttpHandler(t *testing.T) {
	factory, _ := setupTestFactory(t)

	handler := factory.HttpHandler()
	assert.NotNil(t, handler)

	// The handler should return a 404 with a message about CloudWatch metrics
	// This is a basic test to ensure the handler exists and returns a response
	// More detailed testing would require setting up an HTTP test server
}

func TestCloudWatchMetricFactory_ReportingLoop(t *testing.T) {
	mockClient := &MockCloudWatchClient{}
	factory := &CloudWatchMetricFactory{
		client:                mockClient,
		namespace:             "TestNamespace",
		metricsPrefix:         "TestPrefix",
		reportIntervalMinutes: 1,
		lock:                  &sync.Mutex{},
		metrics:               make([]reporter, 0),
		stopChan:              make(chan struct{}),
	}
	defer factory.Stop()

	// Create a channel to signal when the first report is done
	firstReportDone := make(chan struct{})

	fmt.Println("Starting reporting loop...")
	// Start the reporting loop with a shorter interval for testing
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		firstReport := true
		for {
			select {
			case <-ticker.C:
				fmt.Println("Reporting metrics...")
				factory.reportMetrics()
				if firstReport {
					close(firstReportDone)
					firstReport = false
				}
			case <-factory.stopChan:
				fmt.Println("Reporting loop stopped")
				return
			}
		}
	}()

	// Create and update a counter
	fmt.Println("Creating counter...")
	counter, err := factory.GetOrCreateCounter(metrics.NewMetric("test_counter", "test_counter description"))
	require.NoError(t, err)
	require.NotNil(t, counter)

	fmt.Println("Adding value to counter...")
	counter.Add(5)

	// Wait for the first report to complete
	fmt.Println("Waiting for first report...")
	<-firstReportDone

	// Verify metrics were reported
	inputs := mockClient.GetPutMetricDataInputs()
	fmt.Printf("Number of PutMetricData calls: %d\n", len(inputs))
	require.Len(t, inputs, 1, "Expected one PutMetricData call")
	metricData := inputs[0].MetricData
	require.Len(t, metricData, 1, "Expected one metric datum")
	assert.Equal(t, float64(5), *metricData[0].Value)

	// Verify the reporting loop is still running by checking for more reports
	time.Sleep(200 * time.Millisecond)
	inputs = mockClient.GetPutMetricDataInputs()
	fmt.Printf("Total number of PutMetricData calls: %d\n", len(inputs))
	assert.Greater(t, len(inputs), 1, "Expected multiple reports")
}
