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
	"fmt"
	"os"
	"strings"
	"time"

	toolsv1alpha1 "github.com/junaidk/dummy-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const dummyFinalizer = "tools.interview.com/finalizer"
const defaultNginxImage = "nginx:latest"

// DummyReconciler reconciles a Dummy object
type DummyReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=tools.interview.com,resources=dummies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=tools.interview.com,resources=dummies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=tools.interview.com,resources=dummies/finalizers,verbs=update
// +kubebuilder:rbac:groups=tools.interview.com,resources=dummies/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Dummy object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *DummyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	dummy := &toolsv1alpha1.Dummy{}
	err := r.Get(ctx, req.NamespacedName, dummy)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the custom resource is not found then it usually means that it was deleted or not created
			// In this way, we will stop the reconciliation
			log.Info("dummy resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get Dummy")
		return ctrl.Result{}, err
	}

	log.Info("reconciled", "name", dummy.Name, "namespace", dummy.Namespace, "message", dummy.Spec.Message)

	if dummy.Status.SpecEcho != dummy.Spec.Message {
		dummy.Status.SpecEcho = dummy.Spec.Message
		if err := r.Status().Update(ctx, dummy); err != nil {
			log.Error(err, "Failed to update status for Dummy")
			return ctrl.Result{Requeue: true}, err
		}
	}

	// Add a finalizer. Then, we can define some operations which should
	// occur before the custom resource is deleted.
	if !controllerutil.ContainsFinalizer(dummy, dummyFinalizer) {
		log.Info("Adding Finalizer for Dummy")
		if ok := controllerutil.AddFinalizer(dummy, dummyFinalizer); !ok {
			log.Error(err, "Failed to add finalizer into the custom resource")
			return ctrl.Result{Requeue: true}, nil
		}

		if err = r.Update(ctx, dummy); err != nil {
			log.Error(err, "Failed to update custom resource to add finalizer")
			return ctrl.Result{}, err
		}
	}

	// Check if the Dummy instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isDummyMarkedToBeDeleted := dummy.GetDeletionTimestamp() != nil
	if isDummyMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(dummy, dummyFinalizer) {
			log.Info("Performing Finalizer Operations for Dummy before delete CR")

			r.doFinalizerOperationsForDummy(dummy)

			// Re-fetch the dummy Custom Resource before updating the status
			// so that we have the latest state of the resource on the cluster
			if err := r.Get(ctx, req.NamespacedName, dummy); err != nil {
				log.Error(err, "Failed to re-fetch dummy")
				return ctrl.Result{}, err
			}

			log.Info("Removing Finalizer for Dummy after successfully perform the operations")
			if ok := controllerutil.RemoveFinalizer(dummy, dummyFinalizer); !ok {
				log.Error(err, "Failed to remove finalizer for Dummy")
				return ctrl.Result{Requeue: true}, nil
			}

			if err := r.Update(ctx, dummy); err != nil {
				log.Error(err, "Failed to remove finalizer for Dummy")
				return ctrl.Result{}, err
			}
		}
	}

	// Check if the nginx pod already exists, if not create a new one
	found := &corev1.Pod{}
	err = r.Get(ctx, types.NamespacedName{Name: dummy.Name, Namespace: dummy.Namespace}, found)
	if err != nil && apierrors.IsNotFound(err) {
		pod, err := r.podForDummy(dummy)
		if err != nil {
			log.Error(err, "Failed to define new Pod resource for Dummy")
			return ctrl.Result{}, err
		}
		log.Info("Creating a new Pod", "Pod.Namespace", pod.Namespace, "Pod.Name", pod.Name)
		if err = r.Create(ctx, pod); err != nil {
			log.Error(err, "Failed to create new Pod", "Pod.Namespace", pod.Namespace, "Pod.Name", pod.Name)
			return ctrl.Result{}, err
		}

		dummy.Status.PodStatus = toolsv1alpha1.PodPhasePending
		if err := r.Status().Update(ctx, dummy); err != nil {
			log.Error(err, "Failed to update status for Dummy")
			return ctrl.Result{}, err
		}

		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Pod")
		// return the error for the reconciliation be re-trigged again
		return ctrl.Result{}, err
	}

	if IsPodReady(found) {
		dummy.Status.PodStatus = toolsv1alpha1.PodPhaseRunning
	} else {
		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: time.Second * 10,
		}, nil
	}

	if err := r.Status().Update(ctx, dummy); err != nil {
		log.Error(err, "Failed to update status for Dummy")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *DummyReconciler) doFinalizerOperationsForDummy(cr *toolsv1alpha1.Dummy) {
	// Can be used to add the cleanup steps that the operator
	// needs to do before the CR can be deleted. Examples
	// of finalizers include performing backups and deleting
	// resources that are not owned by this CR, like a PVC.

	// The following implementation will raise an event
	r.Recorder.Event(cr, "Warning", "Deleting",
		fmt.Sprintf("Custom Resource %s is being deleted from the namespace %s",
			cr.Name,
			cr.Namespace))
}

func (r *DummyReconciler) podForDummy(dummy *toolsv1alpha1.Dummy) (*corev1.Pod, error) {
	ls := labelsForDummy(dummy.Name)

	containerNmae := "nginx"
	image := imageForDummy()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dummy.Name,
			Namespace: dummy.Namespace,
			Labels:    ls,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  containerNmae,
					Image: image,
					Ports: []corev1.ContainerPort{
						{ContainerPort: 80},
					},
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: &[]bool{false}[0],
					},
				},
			},
			SecurityContext: &corev1.PodSecurityContext{
				SeccompProfile: &corev1.SeccompProfile{
					Type: corev1.SeccompProfileTypeRuntimeDefault,
				},
			},
		},
	}

	if err := ctrl.SetControllerReference(dummy, pod, r.Scheme); err != nil {
		return nil, err
	}

	return pod, nil
}

// IsPodReady returns true if a pod is running and all containers are ready
func IsPodReady(pod *corev1.Pod) bool {
	// Check if pod phase is Running
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}

	// Check if all containers are ready
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if !containerStatus.Ready {
			return false
		}
	}

	// All checks passed
	return true
}

// labelsForDummyreturns the labels for selecting the resources
func labelsForDummy(name string) map[string]string {
	var imageTag string
	imageAndTag := strings.Split(imageForDummy(), ":")

	imageTag = "latest"
	if len(imageAndTag) == 2 {
		imageTag = imageAndTag[1]
	}

	return map[string]string{"app.kubernetes.io/name": "dummy-operator",
		"app.kubernetes.io/version":    imageTag,
		"app.kubernetes.io/managed-by": "DummyController",
		"app.kubernetes.io/cr-name":    name,
	}
}

// imageForDummy gets the Operand image which is managed by this controller
// from the NGINX_IMAGE environment variable defined in the config/manager/manager.yaml
func imageForDummy() string {
	var imageEnvVar = "NGINX_IMAGE"
	image, found := os.LookupEnv(imageEnvVar)
	if !found {
		return defaultNginxImage
	}
	return image
}

// SetupWithManager sets up the controller with the Manager.
func (r *DummyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&toolsv1alpha1.Dummy{}).
		Owns(&corev1.Pod{}).
		Complete(r)
}
