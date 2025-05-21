// Package cloudwatchmetrics provides an implementation of metrics interfaces for Amazon CloudWatch.
// It allows sending metrics data directly to Amazon CloudWatch for monitoring and visualization.
package amazoncloudwatchmetrics

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/datastax/zdm-proxy/proxy/pkg/metrics"
	log "github.com/sirupsen/logrus"
)

// CloudWatchClient is an interface that defines the methods we need from the Amazon CloudWatch client.
// This interface allows for easier testing by enabling mock implementations.
type CloudWatchClient interface {
	PutMetricData(ctx context.Context, params *cloudwatch.PutMetricDataInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.PutMetricDataOutput, error)
	Options() cloudwatch.Options
}

// CloudWatchMetricFactory implements the metrics.MetricFactory interface for Amazon CloudWatch.
// It creates and manages different types of metrics (counters, gauges, histograms) that send data to Amazon CloudWatch.
type CloudWatchMetricFactory struct {
	client                CloudWatchClient
	namespace             string
	metricsPrefix         string
	reportIntervalMinutes int16
	lock                  *sync.Mutex
	metrics               []reporter
	stopChan              chan struct{}
}

// NewCloudWatchMetricFactory creates a new Amazon CloudWatch metric factory with the specified configuration.
// region: AWS region where Amazon CloudWatch is located
// namespace: Amazon CloudWatch namespace for the metrics
// metricsPrefix: Prefix to be added to all metric names
func NewCloudWatchMetricFactory(region string, namespace string, metricsPrefix string, reportIntervalMinutes int16) (*CloudWatchMetricFactory, error) {
	//if the region is set pass it to the config, if not use the default region
	cfgOptions := []func(*config.LoadOptions) error{}
	if region != "" {
		cfgOptions = append(cfgOptions, config.WithRegion(region))
	}

	cfg, err := config.LoadDefaultConfig(context.TODO(), cfgOptions...)
	if err != nil {
		return nil, err
	}

	factory := &CloudWatchMetricFactory{
		client:                cloudwatch.NewFromConfig(cfg),
		namespace:             namespace,
		reportIntervalMinutes: reportIntervalMinutes,
		lock:                  &sync.Mutex{},
		metricsPrefix:         metricsPrefix,
		metrics:               make([]reporter, 0),
		stopChan:              make(chan struct{}),
	}

	// Start the reporting goroutine
	go factory.reportingLoop()

	return factory, nil
}

// reportingLoop periodically reports all metrics to CloudWatch
func (cw *CloudWatchMetricFactory) reportingLoop() {
	ticker := time.NewTicker(time.Duration(cw.reportIntervalMinutes) * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cw.reportMetrics()
		case <-cw.stopChan:
			return
		}
	}
}

// reportMetrics reports all metrics to CloudWatch
func (cw *CloudWatchMetricFactory) reportMetrics() {
	cw.lock.Lock()
	var allMetricData []types.MetricDatum
	for _, metric := range cw.metrics {
		if metricData := metric.reportMetric(); metricData != nil {
			allMetricData = append(allMetricData, metricData...)
		}
	}
	cw.lock.Unlock()

	// Send metrics in batches of 20
	const batchSize = 20
	for len(allMetricData) > 0 {
		batch := allMetricData
		if len(batch) > batchSize {
			batch = allMetricData[:batchSize]
			allMetricData = allMetricData[batchSize:]
		} else {
			allMetricData = nil
		}

		_, err := cw.client.PutMetricData(context.TODO(), &cloudwatch.PutMetricDataInput{
			Namespace:  &cw.namespace,
			MetricData: batch,
		})
		if err != nil {
			log.Errorf("Failed to put metric data to Amazon CloudWatch: %v", err)
		}
	}
}

// Stop stops the metric reporting
func (cw *CloudWatchMetricFactory) Stop() {
	close(cw.stopChan)
}

// GetOrCreateCounter creates or retrieves a counter metric that sends data to Amazon CloudWatch.
// The counter will track cumulative values over time.
func (cw *CloudWatchMetricFactory) GetOrCreateCounter(mn metrics.Metric) (metrics.Counter, error) {
	counter := &CloudWatchCounter{
		client:    cw.client,
		namespace: cw.namespace,
		name:      cw.metricsPrefix + "_" + mn.GetName(),
		labels:    mn.GetLabels(),
		mutex:     sync.RWMutex{},
	}

	cw.lock.Lock()
	cw.metrics = append(cw.metrics, counter)
	cw.lock.Unlock()

	return counter, nil
}

// GetOrCreateGauge creates or retrieves a gauge metric that sends data to Amazon CloudWatch.
// The gauge will track current values that can go up and down.
func (cw *CloudWatchMetricFactory) GetOrCreateGauge(mn metrics.Metric) (metrics.Gauge, error) {
	gauge := &CloudWatchGauge{
		client:    cw.client,
		namespace: cw.namespace,
		name:      cw.metricsPrefix + "_" + mn.GetName(),
		labels:    mn.GetLabels(),
		mutex:     sync.RWMutex{},
	}

	cw.lock.Lock()
	cw.metrics = append(cw.metrics, gauge)
	cw.lock.Unlock()

	return gauge, nil
}

// GetOrCreateGaugeFunc creates or retrieves a gauge function metric that sends data to Amazon CloudWatch.
// The gauge function will evaluate the provided function to get the current value.
func (cw *CloudWatchMetricFactory) GetOrCreateGaugeFunc(mn metrics.Metric, mf func() float64) (metrics.GaugeFunc, error) {
	return &CloudWatchGaugeFunc{
		client:    cw.client,
		namespace: cw.namespace,
		name:      cw.metricsPrefix + "_" + mn.GetName(),
		labels:    mn.GetLabels(),
		valueFunc: mf,
	}, nil
}

// GetOrCreateHistogram creates or retrieves a histogram metric that sends data to Amazon CloudWatch.
// The histogram will track the distribution of values over time.
func (cw *CloudWatchMetricFactory) GetOrCreateHistogram(mn metrics.Metric, buckets []float64) (metrics.Histogram, error) {
	histogram := &CloudWatchHistogram{
		client:    cw.client,
		namespace: cw.namespace,
		name:      cw.metricsPrefix + "_" + mn.GetName(),
		labels:    mn.GetLabels(),
		mutex:     sync.RWMutex{},
		buckets:   buckets,
	}

	cw.lock.Lock()
	cw.metrics = append(cw.metrics, histogram)
	cw.lock.Unlock()

	return histogram, nil
}

// UnregisterAllMetrics is a no-op for Amazon CloudWatch as it doesn't require metric unregistration.
func (cw *CloudWatchMetricFactory) UnregisterAllMetrics() error {
	// Amazon CloudWatch doesn't require unregistration
	return nil
}

// HttpHandler returns an HTTP handler that explains Amazon CloudWatch metrics are sent directly to AWS.
func (cw *CloudWatchMetricFactory) HttpHandler() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, "Amazon CloudWatch metrics are sent directly to Amazon CloudWatch.", http.StatusNotFound)
	})
}
