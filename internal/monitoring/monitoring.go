// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package monitoring

import (
	"fmt"
	"io/ioutil"

	"code.google.com/p/goauth2/oauth/jwt"
	"google.golang.org/api/cloudmonitoring/v2beta2"
)

const (
	customMetricPrefix = "custom.cloudmonitoring.googleapis.com"
)

// CustomMetricDescriptors is a map from metric's short names to their
// MetricDescriptor definitions.
var CustomMetricDescriptors = map[string]*cloudmonitoring.MetricDescriptor{
	// Custom metric for recording check latency of vanadium production services.
	"service-latency": &cloudmonitoring.MetricDescriptor{
		Name:        fmt.Sprintf("%s/v/service/latency", customMetricPrefix),
		Description: "The check latency (ms) of vanadium production services.",
		TypeDescriptor: &cloudmonitoring.MetricDescriptorTypeDescriptor{
			MetricType: "gauge",
			ValueType:  "double",
		},
		Labels: []*cloudmonitoring.MetricDescriptorLabelDescriptor{
			&cloudmonitoring.MetricDescriptorLabelDescriptor{
				Key:         fmt.Sprintf("%s/name", customMetricPrefix),
				Description: "The name of the vanadium service.",
			},
		},
	},
	// Custom metric for recording GCE instances stats.
	"gce-instance-cpu":     createGCEInstanceMetric("cpu/usage", "The cpu usage (0-1) of this GCE instance.", "double"),
	"gce-instance-memory":  createGCEInstanceMetric("memory/usage", "The memory usage (0-1) of this GCE instance.", "double"),
	"gce-instance-disk":    createGCEInstanceMetric("disk/usage", "The disk usage (0-1) of this GCE instance.", "double"),
	"gce-instance-ping":    createGCEInstanceMetric("ping", "The ping latency (ms) of this GCE instance.", "double"),
	"gce-instance-tcpconn": createGCEInstanceMetric("tcpconn/count", "The number of open tcp connections of this GCE instance.", "int64"),
}

func createGCEInstanceMetric(metricName, description, valueType string) *cloudmonitoring.MetricDescriptor {
	return &cloudmonitoring.MetricDescriptor{
		Name:        fmt.Sprintf("%s/v/gceinstance/%s", customMetricPrefix, metricName),
		Description: description,
		TypeDescriptor: &cloudmonitoring.MetricDescriptorTypeDescriptor{
			MetricType: "gauge",
			ValueType:  valueType,
		},
		Labels: []*cloudmonitoring.MetricDescriptorLabelDescriptor{
			&cloudmonitoring.MetricDescriptorLabelDescriptor{
				Key:         fmt.Sprintf("%s/name", customMetricPrefix),
				Description: "The name of the GCE instance.",
			},
			&cloudmonitoring.MetricDescriptorLabelDescriptor{
				Key:         fmt.Sprintf("%s/zone", customMetricPrefix),
				Description: "The zone of the GCE instance.",
			},
		},
	}

}

// Authenticate authenticates the given service account's email with the given
// key. If successful, it returns a service object that can be used in GCM API
// calls.
func Authenticate(serviceAccountEmail, keyFilePath string) (*cloudmonitoring.Service, error) {
	bytes, err := ioutil.ReadFile(keyFilePath)
	if err != nil {
		return nil, fmt.Errorf("ReadFile(%s) failed: %v", keyFilePath, err)
	}

	token := jwt.NewToken(serviceAccountEmail, cloudmonitoring.MonitoringScope, bytes)
	transport, err := jwt.NewTransport(token)
	if err != nil {
		return nil, fmt.Errorf("NewTransport() failed: %v", err)
	}
	c := transport.Client()
	s, err := cloudmonitoring.New(c)
	if err != nil {
		return nil, fmt.Errorf("New() failed: %v", err)
	}
	return s, nil
}
