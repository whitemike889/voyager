package e2e

import (
	"math"
	"net/http"
	"strings"

	api "github.com/appscode/voyager/apis/voyager/v1beta1"
	"github.com/appscode/voyager/test/framework"
	"github.com/appscode/voyager/test/test-server/client"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ = Describe("With Agent Check Annotation", func() {
	var (
		f    *framework.Invocation
		ing  *api.Ingress
		meta metav1.ObjectMeta
	)

	BeforeEach(func() {
		f = root.Invoke()
		ing = f.Ingress.GetSkeleton()
		f.Ingress.SetSkeletonRule(ing)
	})

	BeforeEach(func() {
		var err error
		meta, err = f.Ingress.CreateResourceWithBackendWeight()
		Expect(err).NotTo(HaveOccurred())
	})

	JustBeforeEach(func() {
		By("Creating ingress with name " + ing.GetName())
		err := f.Ingress.Create(ing)
		Expect(err).NotTo(HaveOccurred())

		f.Ingress.EventuallyStarted(ing).Should(BeTrue())

		By("Checking generated resource")
		Expect(f.Ingress.IsExistsEventually(ing)).Should(BeTrue())
	})

	AfterEach(func() {
		if options.Cleanup {
			f.Ingress.DeleteResourceWithBackendWeight(meta)
			f.Ingress.Delete(ing)
		}
	})

	Describe("Create", func() {
		BeforeEach(func() {
			ing.Spec.Rules = []api.IngressRule{
				{
					IngressRuleValue: api.IngressRuleValue{
						HTTP: &api.HTTPIngressRuleValue{
							Paths: []api.HTTPIngressPath{
								{
									Path: "/testpath",
									Backend: api.HTTPIngressBackend{
										IngressBackend: api.IngressBackend{
											ServiceName: meta.Name,
											ServicePort: intstr.FromInt(80),
										},
									},
								},
							},
						},
					},
				},
			}
		})

		It("Should create Ingress with hostname", func() {
			By("Checking StatusIP for provider" + options.CloudProvider)
			if options.CloudProvider == "minikube" {
				Skip("Minikube do not support this")
			}

			svc, err := f.Ingress.GetServiceWithLoadBalancerStatus(ing.OffshootName(), ing.Namespace)
			Expect(err).NotTo(HaveOccurred())

			baseIngress, err := f.Ingress.GetIngressWithLoadBalancerStatus(ing.Name, ing.Namespace)
			Expect(err).NotTo(HaveOccurred())

			Expect(len(baseIngress.Status.LoadBalancer.Ingress)).Should(Equal(len(svc.Status.LoadBalancer.Ingress)))
			Expect(baseIngress.Status.LoadBalancer.Ingress[0]).Should(Equal(svc.Status.LoadBalancer.Ingress[0]))
		})

		It("Should response HTTP weighted", func() {
			By("Getting HTTP endpoints")
			eps, err := f.Ingress.GetHTTPEndpoints(ing)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(eps)).Should(BeNumerically(">=", 1))

			var dep1Counter, dep2Counter int
			for i := 1; i <= 100; i++ {
				err = f.Ingress.DoHTTP(framework.MaxRetry, "", ing, eps, "GET", "/testpath", func(r *client.Response) bool {
					if strings.HasPrefix(r.PodName, "dep-1-"+meta.Name) {
						dep1Counter++
					} else if strings.HasPrefix(r.PodName, "dep-2-"+meta.Name) {
						dep2Counter++
					}

					return Expect(r.Status).Should(Equal(http.StatusOK)) &&
						Expect(r.Method).Should(Equal("GET")) &&
						Expect(r.Path).Should(Equal("/testpath"))
				})
				Expect(err).NotTo(HaveOccurred())
			}

			ratio := math.Ceil(float64(dep1Counter) / float64(dep2Counter))
			Expect(ratio).Should(BeNumerically(">=", float64(9.0)))
		})
	})
})
