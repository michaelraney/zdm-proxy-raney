package amazoncloudwatchmetrics

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

// reporter interface defines the contract for metrics that need periodic reporting
type reporter interface {
	reportMetric() []types.MetricDatum
}

// CloudWatchGauge implements the metrics.Gauge interface for Amazon CloudWatch.
// It tracks values that can go up and down over time.
type CloudWatchGauge struct {
	client    CloudWatchClient
	namespace string
	name      string
	labels    map[string]string
	value     int
	mutex     sync.RWMutex
}

// Add increases the gauge value by the specified amount.
func (g *CloudWatchGauge) Add(valueToAdd int) {
	g.mutex.Lock()
	g.value += valueToAdd
	g.mutex.Unlock()
}

// Subtract decreases the gauge value by the specified amount.
func (g *CloudWatchGauge) Subtract(valueToSubtract int) {
	g.mutex.Lock()
	g.value -= valueToSubtract
	g.mutex.Unlock()
}

// Set updates the gauge to the specified value
func (g *CloudWatchGauge) Set(valueToSet int) {
	g.mutex.Lock()
	g.value = valueToSet
	g.mutex.Unlock()
}

// reportMetric returns the current value as a MetricDatum
func (g *CloudWatchGauge) reportMetric() []types.MetricDatum {
	g.mutex.RLock()
	currentValue := g.value
	g.mutex.RUnlock()

	dimensions := make([]types.Dimension, 0, len(g.labels))
	for k, v := range g.labels {
		dimensions = append(dimensions, types.Dimension{
			Name:  aws.String(k),
			Value: aws.String(v),
		})
	}

	return []types.MetricDatum{
		{
			MetricName: aws.String(g.name),
			Value:      aws.Float64(float64(currentValue)),
			Unit:       types.StandardUnitCount,
			Dimensions: dimensions,
		},
	}
}

// CloudWatchGaugeFunc implements the metrics.GaugeFunc interface for Amazon CloudWatch.
// It evaluates a function to get the current value and sends it to Amazon CloudWatch.
type CloudWatchGaugeFunc struct {
	client    CloudWatchClient
	namespace string
	name      string
	labels    map[string]string
	valueFunc func() float64
}

// CloudWatchHistogram implements the metrics.Histogram interface for Amazon CloudWatch.
// It tracks the distribution of values over time, particularly useful for measuring latency.
type CloudWatchHistogram struct {
	client    CloudWatchClient
	namespace string
	name      string
	labels    map[string]string
	mutex     sync.RWMutex
	buckets   []float64

	// Summary statistics
	count  int
	sum    float64
	min    float64
	max    float64
	values []float64 // Only used for percentile calculation
}

// Track measures the elapsed time since the provided begin time and updates summary statistics.
func (h *CloudWatchHistogram) Track(begin time.Time) {
	elapsedTimeInSeconds := float64(time.Since(begin)) / float64(time.Second)

	h.mutex.Lock()
	defer h.mutex.Unlock()

	// Update summary statistics
	h.count++
	h.sum += elapsedTimeInSeconds
	if h.count == 1 {
		h.min = elapsedTimeInSeconds
		h.max = elapsedTimeInSeconds
	} else {
		if elapsedTimeInSeconds < h.min {
			h.min = elapsedTimeInSeconds
		}
		if elapsedTimeInSeconds > h.max {
			h.max = elapsedTimeInSeconds
		}
	}

	// Store value for percentile calculation if buckets are defined
	if len(h.buckets) > 0 {
		h.values = append(h.values, elapsedTimeInSeconds)
	}
}

// reportMetric returns the summary statistics as MetricDatum
func (h *CloudWatchHistogram) reportMetric() []types.MetricDatum {
	h.mutex.Lock()
	// Capture current statistics
	count := h.count
	sum := h.sum
	min := h.min
	max := h.max
	values := h.values

	// Reset statistics
	h.count = 0
	h.sum = 0
	h.min = 0
	h.max = 0
	h.values = nil
	h.mutex.Unlock()

	if count == 0 {
		return nil
	}

	dimensions := make([]types.Dimension, 0, len(h.labels))
	for k, v := range h.labels {
		dimensions = append(dimensions, types.Dimension{
			Name:  aws.String(k),
			Value: aws.String(v),
		})
	}

	metricData := []types.MetricDatum{
		{
			MetricName: aws.String(h.name),
			StatisticValues: &types.StatisticSet{
				SampleCount: aws.Float64(float64(count)),
				Sum:         aws.Float64(sum),
				Minimum:     aws.Float64(min),
				Maximum:     aws.Float64(max),
			},
			Dimensions: dimensions,
		},
	}

	// If buckets are defined, calculate percentiles
	if len(values) > 0 {
		sort.Float64s(values)
		for _, bucket := range h.buckets {
			percentile := calculatePercentile(values, bucket)
			metricData = append(metricData, types.MetricDatum{
				MetricName: aws.String(fmt.Sprintf("%s_p%g", h.name, bucket*100)),
				Value:      aws.Float64(percentile),
				Dimensions: dimensions,
			})
		}
	}

	return metricData
}

// calculatePercentile calculates the percentile value from a sorted slice of values
func calculatePercentile(sortedValues []float64, percentile float64) float64 {
	if len(sortedValues) == 0 {
		return 0
	}

	// Ensure percentile is between 0 and 1
	if percentile < 0 {
		percentile = 0
	} else if percentile > 1 {
		percentile = 1
	}

	// Calculate index and ensure it's within bounds
	index := int(float64(len(sortedValues)-1) * percentile)
	if index < 0 {
		index = 0
	} else if index >= len(sortedValues) {
		index = len(sortedValues) - 1
	}

	return sortedValues[index]
}

// CloudWatchCounter implements the metrics.Counter interface for Amazon CloudWatch.
// It tracks cumulative values that only increase over time.
type CloudWatchCounter struct {
	client    CloudWatchClient
	namespace string
	name      string
	labels    map[string]string
	value     int
	mutex     sync.RWMutex
}

// Add increments the counter by the specified value.
func (c *CloudWatchCounter) Add(valueToAdd int) {
	c.mutex.Lock()
	c.value += valueToAdd
	c.mutex.Unlock()
}

// reportMetric returns the current value as a MetricDatum
func (c *CloudWatchCounter) reportMetric() []types.MetricDatum {
	c.mutex.RLock()
	currentValue := c.value
	c.mutex.RUnlock()

	dimensions := make([]types.Dimension, 0, len(c.labels))
	for k, v := range c.labels {
		dimensions = append(dimensions, types.Dimension{
			Name:  aws.String(k),
			Value: aws.String(v),
		})
	}

	return []types.MetricDatum{
		{
			MetricName: aws.String(c.name),
			Value:      aws.Float64(float64(currentValue)),
			Unit:       types.StandardUnitCount,
			Dimensions: dimensions,
		},
	}
}
