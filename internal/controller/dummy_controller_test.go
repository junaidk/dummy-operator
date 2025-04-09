/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	toolsv1alpha1 "github.com/junaidk/dummy-operator/api/v1alpha1"
)

var _ = Describe("Dummy Controller", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
		message  = "hello"
	)
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: resourceName,
			},
		}

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: resourceName,
		}
		dummy := &toolsv1alpha1.Dummy{}

		BeforeEach(func() {
			By("Creating the Namespace to perform the tests")
			err := k8sClient.Create(ctx, namespace)
			Expect(err).To(Not(HaveOccurred()))

			By("Setting the Image ENV VAR which stores the Operand image")
			err = os.Setenv("NGINX_IMAGE", "example.com/image:test")
			Expect(err).To(Not(HaveOccurred()))

			By("creating the custom resource for the Kind Dummy")
			err = k8sClient.Get(ctx, typeNamespacedName, dummy)
			if err != nil && errors.IsNotFound(err) {
				resource := &toolsv1alpha1.Dummy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: resourceName,
					},
					Spec: toolsv1alpha1.DummySpec{
						Message: message,
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &toolsv1alpha1.Dummy{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Dummy")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			By("Deleting the Namespace to perform the tests")
			_ = k8sClient.Delete(ctx, namespace)

			By("Removing the Image ENV VAR which stores the Operand image")
			_ = os.Unsetenv("NGINX_IMAGE")
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &DummyReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking if Pod was successfully created in the reconciliation")
			createdPod := &corev1.Pod{}
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, createdPod)
			}, time.Minute, time.Second).Should(Succeed())

			By("Checking if SpecEcho filed was successfully updated")
			createdDummy := &toolsv1alpha1.Dummy{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, createdDummy)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			Expect(createdDummy.Status.SpecEcho).Should(Equal(message))

			By("Checking if PodStatus filed was successfully updated")
			Expect(createdDummy.Status.PodStatus).Should(Equal(toolsv1alpha1.PodPhasePending))

			By("Checking if Pod image filed was successfully set")
			Expect(createdPod.Spec.Containers[0].Image).Should(Equal(os.Getenv("NGINX_IMAGE")))

		})
	})
})
