//go:build e2e

package e2e

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kyma-project/telemetry-manager/internal/conditions"
	kitk8s "github.com/kyma-project/telemetry-manager/test/testkit/k8s"
	"github.com/kyma-project/telemetry-manager/test/testkit/mocks/backend"
	"github.com/kyma-project/telemetry-manager/test/testkit/mocks/loggen"
	"github.com/kyma-project/telemetry-manager/test/testkit/tlsgen"
	"github.com/kyma-project/telemetry-manager/test/testkit/verifiers"
)

var _ = Describe("Logs mTLS with invalid certificate", Label("logs"), func() {
	const (
		mockBackendName = "logs-tls-receiver"
		mockNs          = "logs-mocks-invalid-tls"
		logProducerName = "logs-invalid-mtls-cert"
	)
	var (
		pipelineName string
	)

	makeResources := func() []client.Object {
		var objs []client.Object
		objs = append(objs, kitk8s.NewNamespace(mockNs).K8sObject())

		serverCerts, clientCerts, err := tlsgen.NewCertBuilder(mockBackendName, mockNs).
			WithInvalidClientCert().
			Build()
		Expect(err).ToNot(HaveOccurred())

		mockBackend := backend.New(mockBackendName, mockNs, backend.SignalTypeLogs, backend.WithTLS(*serverCerts))
		objs = append(objs, mockBackend.K8sObjects()...)

		logPipeline := kitk8s.NewLogPipelineV1Alpha1(fmt.Sprintf("%s-%s", mockBackend.Name(), "pipeline")).
			WithSecretKeyRef(mockBackend.HostSecretRefV1Alpha1()).
			WithHTTPOutput().
			WithTLS(*clientCerts)
		pipelineName = logPipeline.Name()

		mockLogProducer := loggen.New(logProducerName, mockNs)
		objs = append(objs, mockLogProducer.K8sObject(kitk8s.WithLabel("app", "logging-test")))
		objs = append(objs,
			logPipeline.K8sObject(),
		)

		return objs
	}

	Context("When a log pipeline with invalid TLS Cert is created", Ordered, func() {
		BeforeAll(func() {
			k8sObjects := makeResources()

			DeferCleanup(func() {
				Expect(kitk8s.DeleteObjects(ctx, k8sClient, k8sObjects...)).Should(Succeed())
			})
			Expect(kitk8s.CreateObjects(ctx, k8sClient, k8sObjects...)).Should(Succeed())
		})

		It("Should not have running pipelines", func() {
			verifiers.LogPipelineShouldNotBeHealthy(ctx, k8sClient, pipelineName)
		})

		It("Should have a tls certificate with invalid Condition set in pipeline conditions", func() {
			verifiers.LogPipelineShouldHaveTLSCondition(ctx, k8sClient, pipelineName, conditions.ReasonTLSCertificateInvalid)
		})

		It("Should have telemetryCR showing tls certificate expired for log component in its status", func() {
			verifiers.TelemetryShouldHaveCondition(ctx, k8sClient, "LogComponentsHealthy", conditions.ReasonTLSCertificateInvalid, false)
		})
	})
})