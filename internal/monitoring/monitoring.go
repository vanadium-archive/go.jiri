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
