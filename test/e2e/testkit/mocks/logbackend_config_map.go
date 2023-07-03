//go:build e2e

package mocks

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type LogBackendConfigMap struct {
	name              string
	namespace         string
	exportedFilePath  string
	fluentdConfigName string
}

func NewLogBackendConfigMap(name, namespace, path, fluentdConfigName string) *LogBackendConfigMap {
	return &LogBackendConfigMap{
		name:              name,
		namespace:         namespace,
		exportedFilePath:  path,
		fluentdConfigName: fluentdConfigName,
	}
}

const configTemplateLog = `receivers:
  fluentforward:
    endpoint: 0.0.0.0:8006
  otlp:
    protocols:
      grpc: {}
      http: {}
exporters:
  file:
    path: {{ FILEPATH }}
  logging:
    loglevel: debug
service:
  telemetry:
    logs:
      level: "debug"
  pipelines:
    logs:
      receivers:
        - otlp
        - fluentforward
      exporters:
        - file
        - logging`

const configTemplateFluentd = `<source>
  @type http
  tag input
  port 9880
  bind 0.0.0.0
  body_size_limit 32m
  add_http_headers true
  <parse>
    @type json
  </parse>
</source>
<match input>
  @type forward
  send_timeout 60s
  recover_wait 10s
  hard_timeout 60s
  flush_interval 1s

  <server>
    name otlp
    host 127.0.0.1
    port 8006
    weight 60
  </server>
  
</match>`

func (cm *LogBackendConfigMap) Name() string {
	return cm.name
}

func (cm *LogBackendConfigMap) K8sObject() *corev1.ConfigMap {
	config := strings.Replace(configTemplateLog, "{{ FILEPATH }}", cm.exportedFilePath, 1)

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cm.name,
			Namespace: cm.namespace,
		},
		Data: map[string]string{"config.yaml": config},
	}
}

func (cm *LogBackendConfigMap) K8sObjectFluentDConfig() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cm.fluentdConfigName,
			Namespace: cm.namespace,
		},
		Data: map[string]string{"fluent.conf": configTemplateFluentd},
	}
}
