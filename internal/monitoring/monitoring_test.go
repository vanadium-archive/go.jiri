// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package monitoring

import (
	"fmt"
	"reflect"
	"testing"

	"google.golang.org/api/cloudmonitoring/v2beta2"
)

func TestCreateMetric(t *testing.T) {
	type testCase struct {
		metricType       string
		description      string
		valueType        string
		includeGCELabels bool
		extraLabels      []labelData
		expectedMetric   *cloudmonitoring.MetricDescriptor
	}
	testCases := []testCase{
		testCase{
			metricType:       "test",
			description:      "this is a test",
			valueType:        "double",
			includeGCELabels: false,
			extraLabels:      nil,
			expectedMetric: &cloudmonitoring.MetricDescriptor{
				Name:        fmt.Sprintf("%s/vanadium/test", customMetricPrefix),
				Description: "this is a test",
				TypeDescriptor: &cloudmonitoring.MetricDescriptorTypeDescriptor{
					MetricType: "gauge",
					ValueType:  "double",
				},
				Labels: []*cloudmonitoring.MetricDescriptorLabelDescriptor{
					&cloudmonitoring.MetricDescriptorLabelDescriptor{
						Key:         fmt.Sprintf("%s/metric-name", customMetricPrefix),
						Description: "The name of the metric.",
					},
				},
			},
		},
		testCase{
			metricType:       "test2",
			description:      "this is a test2",
			valueType:        "string",
			includeGCELabels: true,
			extraLabels:      nil,
			expectedMetric: &cloudmonitoring.MetricDescriptor{
				Name:        fmt.Sprintf("%s/vanadium/test2", customMetricPrefix),
				Description: "this is a test2",
				TypeDescriptor: &cloudmonitoring.MetricDescriptorTypeDescriptor{
					MetricType: "gauge",
					ValueType:  "string",
				},
				Labels: []*cloudmonitoring.MetricDescriptorLabelDescriptor{
					&cloudmonitoring.MetricDescriptorLabelDescriptor{
						Key:         fmt.Sprintf("%s/gce-instance", customMetricPrefix),
						Description: "The name of the GCE instance associated with this metric.",
					},
					&cloudmonitoring.MetricDescriptorLabelDescriptor{
						Key:         fmt.Sprintf("%s/gce-zone", customMetricPrefix),
						Description: "The zone of the GCE instance associated with this metric.",
					},
					&cloudmonitoring.MetricDescriptorLabelDescriptor{
						Key:         fmt.Sprintf("%s/metric-name", customMetricPrefix),
						Description: "The name of the metric.",
					},
				},
			},
		},
		testCase{
			metricType:       "test3",
			description:      "this is a test3",
			valueType:        "double",
			includeGCELabels: true,
			extraLabels: []labelData{
				labelData{
					key:         "extraLabel",
					description: "this is an extra label",
				},
			},
			expectedMetric: &cloudmonitoring.MetricDescriptor{
				Name:        fmt.Sprintf("%s/vanadium/test3", customMetricPrefix),
				Description: "this is a test3",
				TypeDescriptor: &cloudmonitoring.MetricDescriptorTypeDescriptor{
					MetricType: "gauge",
					ValueType:  "double",
				},
				Labels: []*cloudmonitoring.MetricDescriptorLabelDescriptor{
					&cloudmonitoring.MetricDescriptorLabelDescriptor{
						Key:         fmt.Sprintf("%s/gce-instance", customMetricPrefix),
						Description: "The name of the GCE instance associated with this metric.",
					},
					&cloudmonitoring.MetricDescriptorLabelDescriptor{
						Key:         fmt.Sprintf("%s/gce-zone", customMetricPrefix),
						Description: "The zone of the GCE instance associated with this metric.",
					},
					&cloudmonitoring.MetricDescriptorLabelDescriptor{
						Key:         fmt.Sprintf("%s/metric-name", customMetricPrefix),
						Description: "The name of the metric.",
					},
					&cloudmonitoring.MetricDescriptorLabelDescriptor{
						Key:         fmt.Sprintf("%s/extraLabel", customMetricPrefix),
						Description: "this is an extra label",
					},
				},
			},
		},
	}
	for _, test := range testCases {
		got := createMetric(test.metricType, test.description, test.valueType, test.includeGCELabels, test.extraLabels)
		if !reflect.DeepEqual(got, test.expectedMetric) {
			t.Fatalf("want %#v, got %#v", test.expectedMetric, got)
		}
	}
}
