//go:build e2e

package log

import (
	k8smeta "k8s.io/apimachinery/pkg/apis/meta/v1"

	telemetry "github.com/kyma-project/telemetry-manager/apis/telemetry/v1alpha1"
)

type Pipeline struct {
	name string
}

func NewPipeline(name string) *Pipeline {
	return &Pipeline{
		name: name,
	}
}
func (p *Pipeline) K8sObject() *telemetry.LogPipeline {
	return &telemetry.LogPipeline{
		ObjectMeta: k8smeta.ObjectMeta{
			Name: p.name,
		},
		Spec: telemetry.LogPipelineSpec{
			Output: telemetry.Output{
				HTTP: &telemetry.HTTPOutput{
					Dedot: true,
					Host: telemetry.ValueType{
						Value: "http://endpoint",
					},
					Port:   "9880",
					URI:    "/",
					Format: "json",
					TLSConfig: telemetry.TLSConfig{
						Disabled:                  true,
						SkipCertificateValidation: true,
					},
				},
			},
		},
	}
}
