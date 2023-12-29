//go:build e2e

package e2e

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kitk8s "github.com/kyma-project/telemetry-manager/test/testkit/k8s"
	kitkyma "github.com/kyma-project/telemetry-manager/test/testkit/kyma"
	kitmetricpipeline "github.com/kyma-project/telemetry-manager/test/testkit/kyma/telemetry/metric"
	. "github.com/kyma-project/telemetry-manager/test/testkit/matchers/metric"
	"github.com/kyma-project/telemetry-manager/test/testkit/mocks/backend"
	"github.com/kyma-project/telemetry-manager/test/testkit/mocks/metricproducer"
	"github.com/kyma-project/telemetry-manager/test/testkit/otlp/kubeletstats"
	"github.com/kyma-project/telemetry-manager/test/testkit/periodic"
	"github.com/kyma-project/telemetry-manager/test/testkit/verifiers"
)

var _ = Describe("Metrics Prometheus Input", Label("metrics"), func() {
	const (
		mockNs          = "metric-prometheus-input"
		mockBackendName = "metric-agent-receiver"
	)

	var (
		pipelineName       string
		telemetryExportURL string
	)

	makeResources := func() []client.Object {
		var objs []client.Object
		objs = append(objs, kitk8s.NewNamespace(mockNs).K8sObject())

		// Mocks namespace objects.
		mockBackend := backend.New(mockBackendName, mockNs, backend.SignalTypeMetrics)
		mockMetricProducer := metricproducer.New(mockNs)
		objs = append(objs, mockBackend.K8sObjects()...)
		objs = append(objs, []client.Object{
			mockMetricProducer.Pod().WithPrometheusAnnotations(metricproducer.SchemeHTTP).K8sObject(),
			mockMetricProducer.Service().WithPrometheusAnnotations(metricproducer.SchemeHTTP).K8sObject(),
		}...)
		telemetryExportURL = mockBackend.TelemetryExportURL(proxyClient)

		// Default namespace objects.
		metricPipeline := kitmetricpipeline.NewPipeline("pipeline-with-prometheus-input-enabled").
			WithOutputEndpointFromSecret(mockBackend.HostSecretRef()).
			PrometheusInput(true, kitmetricpipeline.IncludeNamespaces(mockNs))
		pipelineName = metricPipeline.Name()
		objs = append(objs, metricPipeline.K8sObject())

		return objs
	}

	Context("When a metricpipeline with Prometheus input exists", Ordered, func() {
		BeforeAll(func() {
			k8sObjects := makeResources()

			DeferCleanup(func() {
				//Expect(kitk8s.DeleteObjects(ctx, k8sClient, k8sObjects...)).Should(Succeed())
			})

			Expect(kitk8s.CreateObjects(ctx, k8sClient, k8sObjects...)).Should(Succeed())
		})

		It("Ensures the metric gateway deployment is ready", func() {
			verifiers.DeploymentShouldBeReady(ctx, k8sClient, kitkyma.MetricGatewayName)
		})

		It("Ensures the metrics backend is ready", func() {
			verifiers.DeploymentShouldBeReady(ctx, k8sClient, types.NamespacedName{Name: mockBackendName, Namespace: mockNs})
		})

		It("Ensures the metric agent daemonset is ready", func() {
			verifiers.DaemonSetShouldBeReady(ctx, k8sClient, kitkyma.MetricAgentName)
		})

		It("Ensures the metricpipeline is running", func() {
			verifiers.MetricPipelineShouldBeHealthy(ctx, k8sClient, pipelineName)
		})

		It("Ensures custom metric scraped via annotated pods are sent to backend", func() {
			Eventually(func(g Gomega) {
				resp, err := proxyClient.Get(telemetryExportURL)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(resp).To(HaveHTTPStatus(http.StatusOK))
				// here we are discovering the same metric-producer workload twice: once via the annotated service and once via the annotated pod
				// targets discovered via annotated pods must have no service label
				g.Expect(resp).To(HaveHTTPBody(SatisfyAll(
					ContainMd(ContainMetric(SatisfyAll(
						WithName(Equal(metricproducer.MetricCPUTemperature.Name)),
						WithType(Equal(metricproducer.MetricCPUTemperature.Type)),
					))),
					ContainMd(ContainMetric(SatisfyAll(
						WithName(Equal(metricproducer.MetricCPUEnergyHistogram.Name)),
						WithType(Equal(metricproducer.MetricCPUEnergyHistogram.Type)),
					))),
					ContainMd(ContainMetric(SatisfyAll(
						WithName(Equal(metricproducer.MetricHardwareHumidity.Name)),
						WithType(Equal(metricproducer.MetricHardwareHumidity.Type)),
					))),
					ContainMd(ContainMetric(SatisfyAll(
						WithName(Equal(metricproducer.MetricHardDiskErrorsTotal.Name)),
						WithType(Equal(metricproducer.MetricHardDiskErrorsTotal.Type)),
					))),
				),
				))
			}, periodic.TelemetryEventuallyTimeout, periodic.TelemetryInterval).Should(Succeed())
		})

		It("Ensures custom metric scraped via annotated services are sent to backend", func() {
			Eventually(func(g Gomega) {
				resp, err := proxyClient.Get(telemetryExportURL)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(resp).To(HaveHTTPStatus(http.StatusOK))
				g.Expect(resp).To(HaveHTTPBody(SatisfyAll(
					ContainMd(ContainMetric(SatisfyAll(
						WithName(Equal(metricproducer.MetricCPUTemperature.Name)),
						WithType(Equal(metricproducer.MetricCPUTemperature.Type)),
						ContainDataPointAttrs(HaveKey("service")),
					))),
					ContainMd(ContainMetric(SatisfyAll(
						WithName(Equal(metricproducer.MetricCPUEnergyHistogram.Name)),
						WithType(Equal(metricproducer.MetricCPUEnergyHistogram.Type)),
						ContainDataPointAttrs(HaveKey("service")),
					))),
					ContainMd(ContainMetric(SatisfyAll(
						WithName(Equal(metricproducer.MetricHardwareHumidity.Name)),
						WithType(Equal(metricproducer.MetricHardwareHumidity.Type)),
						ContainDataPointAttrs(HaveKey("service")),
					))),
					ContainMd(ContainMetric(SatisfyAll(
						WithName(Equal(metricproducer.MetricHardDiskErrorsTotal.Name)),
						WithType(Equal(metricproducer.MetricHardDiskErrorsTotal.Type)),
						ContainDataPointAttrs(HaveKey("service")),
					))),
				),
				))
			}, periodic.TelemetryEventuallyTimeout, periodic.TelemetryInterval).Should(Succeed())
		})

		It("Ensures no kubeletstats metrics are sent to backend", func() {
			Eventually(func(g Gomega) {
				resp, err := proxyClient.Get(telemetryExportURL)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(resp).To(HaveHTTPStatus(http.StatusOK))
				g.Expect(resp).To(HaveHTTPBody(
					Not(ContainMd(ContainMetric(WithName(BeElementOf(kubeletstats.MetricNames))))),
				))
			}, periodic.TelemetryEventuallyTimeout, periodic.TelemetryInterval).Should(Succeed())
		})

		It("Ensures kubeletstats metrics from system namespaces are not sent to backend", func() {
			verifiers.MetricsFromNamespaceShouldNotBeDelivered(proxyClient, telemetryExportURL, kitkyma.SystemNamespaceName)
		})

		It("Ensures no diagnostic metrics are sent to backend", func() {
			Eventually(func(g Gomega) {
				resp, err := proxyClient.Get(telemetryExportURL)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(resp).To(HaveHTTPStatus(http.StatusOK))
				g.Expect(resp).To(HaveHTTPBody(
					Not(ContainMd(ContainMetric(WithName(BeElementOf("up", "scrape_duration_seconds", "scrape_samples_scraped", "scrape_samples_post_metric_relabeling", "scrape_series_added"))))),
				))
			}, periodic.TelemetryEventuallyTimeout, periodic.TelemetryInterval).Should(Succeed())
		})
	})
})
