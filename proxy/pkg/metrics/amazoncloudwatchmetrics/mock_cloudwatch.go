package amazoncloudwatchmetrics

import (
	"context"
	"sync"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
)

// MockCloudWatchClient implements CloudWatchClient interface for testing
type MockCloudWatchClient struct {
	putMetricDataInputs []*cloudwatch.PutMetricDataInput
	mutex               sync.Mutex
}

// PutMetricData stores the input for later verification
func (m *MockCloudWatchClient) PutMetricData(ctx context.Context, params *cloudwatch.PutMetricDataInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.PutMetricDataOutput, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.putMetricDataInputs = append(m.putMetricDataInputs, params)
	return &cloudwatch.PutMetricDataOutput{}, nil
}

// Options returns empty options for testing
func (m *MockCloudWatchClient) Options() cloudwatch.Options {
	return cloudwatch.Options{}
}

// GetPutMetricDataInputs returns all stored inputs
func (m *MockCloudWatchClient) GetPutMetricDataInputs() []*cloudwatch.PutMetricDataInput {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.putMetricDataInputs
}

// Clear clears all stored inputs
func (m *MockCloudWatchClient) Clear() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.putMetricDataInputs = nil
}
