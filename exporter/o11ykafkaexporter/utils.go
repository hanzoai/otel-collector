package o11ykafkaexporter

import (
	"go.opentelemetry.io/collector/client"
)

const (
	DefaultKafkaTopicPrefix = "default"
)

// getKafkaTopicFromClientMetadata returns the kafka topic from client metadata
func getKafkaTopicPrefixFromClientMetadata(md client.Metadata) string {
	// return default topic if no tenant id is found in client metadata
	o11yTenantId := md.Get("o11y_tenant_id")
	if len(o11yTenantId) != 0 {
		return o11yTenantId[0]
	}

	return DefaultKafkaTopicPrefix
}
