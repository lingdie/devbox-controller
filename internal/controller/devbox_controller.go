/*
Copyright 2024.

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
	devboxv1alpha1 "github.com/labring/sealos/controllers/devbox/api/v1alpha1"
	"github.com/labring/sealos/controllers/devbox/label"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"time"
)

const (
	FinalizerName = "devbox.sealos.io/finalizer"
)

const (
	DevBoxPartOf = "devbox"
)

const (
	Devbox = "devbox"
)
const (
	rate = 10
)

// DevboxReconciler reconciles a Devbox object
type DevboxReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=devbox.sealos.io,resources=devboxes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=devbox.sealos.io,resources=devboxes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=devbox.sealos.io,resources=devboxes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Devbox object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/reconcile
func (r *DevboxReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx, "devbox", req.NamespacedName)
	devbox := &devboxv1alpha1.Devbox{}
	if err := r.Get(ctx, req.NamespacedName, devbox); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if devbox.ObjectMeta.DeletionTimestamp.IsZero() {
		if controllerutil.AddFinalizer(devbox, FinalizerName) {
			if err := r.Update(ctx, devbox); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		if controllerutil.RemoveFinalizer(devbox, FinalizerName) {
			if err := r.Update(ctx, devbox); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if err := r.fillDefaultValue(ctx, devbox); err != nil {
		return ctrl.Result{}, err
	}

	recLabels := label.RecommendedLabels(&label.Recommended{
		Name:      devbox.Name,
		ManagedBy: label.DefaultManagedBy,
		PartOf:    DevBoxPartOf,
	})

	if err := r.syncPod(ctx, devbox, recLabels); err != nil {
		logger.Error(err, "create pod failed")
		r.recorder.Eventf(devbox, corev1.EventTypeWarning, "Create pod failed", "%v", err)
		return ctrl.Result{}, err
	}

	currentTime := time.Now()
	formattedTime := currentTime.Format(time.RFC3339)

	devbox.Status.Commit.CommitID = formattedTime
	if err := r.Update(ctx, devbox); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.syncService(ctx, devbox, recLabels); err != nil {
		logger.Error(err, "create service failed")
		r.recorder.Eventf(devbox, corev1.EventTypeWarning, "Create service failed", "%v", err)
		return ctrl.Result{}, err
	}

	r.recorder.Eventf(devbox, corev1.EventTypeNormal, "Created", "create terminal success: %v", devbox.ObjectMeta.Name)
	return ctrl.Result{}, nil
}

func (r *DevboxReconciler) fillDefaultValue(ctx context.Context, devbox *devboxv1alpha1.Devbox) error {
	return nil
}

func (r *DevboxReconciler) syncPod(ctx context.Context, devbox *devboxv1alpha1.Devbox, recLabels map[string]string) error {
	objectMeta := metav1.ObjectMeta{
		Name:      devbox.ObjectMeta.Name,
		Namespace: devbox.ObjectMeta.Namespace,
		Labels:    recLabels,
	}
	ports := []corev1.ContainerPort{
		{
			Name:          "http",
			Protocol:      corev1.ProtocolTCP,
			ContainerPort: 2222,
		},
	}

	envs := []corev1.EnvVar{
		{Name: "POD_TYPE", Value: Devbox},
		{Name: "VERSION", Value: devbox.Status.Commit.CommitID},
	}

	cpuRequest := devbox.Spec.Resource["cpu"]
	memoryRequest := devbox.Spec.Resource["memory"]

	cpuLimit := cpuRequest.DeepCopy()
	cpuLimit.Set(cpuRequest.Value() * rate)

	memoryLimit := memoryRequest.DeepCopy()
	memoryLimit.Set(memoryRequest.Value() * rate)

	//get image name
	imageName, err := r.GetImageName(ctx, devbox)

	containers := []corev1.Container{
		{
			Name:  devbox.ObjectMeta.Name,
			Image: imageName,
			Ports: ports,
			Env:   envs,
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					"cpu":    cpuRequest,
					"memory": memoryRequest,
				},
				Limits: corev1.ResourceList{
					"cpu":    cpuLimit,
					"memory": memoryLimit,
				},
			},
		},
	}
	expectPod := &corev1.Pod{
		ObjectMeta: objectMeta,
		Spec: corev1.PodSpec{
			Containers: containers,
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: objectMeta,
	}

	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, pod, func() error {
		// only update some specific fields
		pod.Spec.Containers = expectPod.Spec.Containers
		return controllerutil.SetControllerReference(devbox, pod, r.Scheme)
	})
	if err != nil {
		return err
	}

	return nil
}

func (r *DevboxReconciler) GetImageName(ctx context.Context, devbox *devboxv1alpha1.Devbox) (string, error) {
	var err error
	if len(devbox.Status.Commit.CommitID) == 0 {
		return devbox.Spec.RuntimeRef.Name, err
	} else {
		return devbox.Status.Commit.CommitID, err
	}
}

func (r *DevboxReconciler) syncService(ctx context.Context, devbox *devboxv1alpha1.Devbox, recLabels map[string]string) error {
	expectServiceSpec := corev1.ServiceSpec{
		Selector: recLabels,
		Type:     corev1.ServiceTypeNodePort,
		Ports: []corev1.ServicePort{
			{
				Name:       "tty",
				Port:       2222,
				TargetPort: intstr.FromInt(2222),
				Protocol:   corev1.ProtocolTCP,
				//NodePort:   30000,
			},
		},
	}

	if devbox.Status.Network.ServiceName == "" {
		devbox.Status.Network.ServiceName = devbox.Name + "-svc"
		if err := r.Status().Update(ctx, devbox); err != nil {
			return err
		}
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      devbox.Status.Network.ServiceName,
			Namespace: devbox.Namespace,
			Labels:    recLabels,
		},
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, func() error {
		// only update some specific fields
		service.Spec.Selector = expectServiceSpec.Selector
		service.Spec.Type = expectServiceSpec.Type
		if len(service.Spec.Ports) == 0 {
			service.Spec.Ports = expectServiceSpec.Ports
		} else {
			service.Spec.Ports[0].Name = expectServiceSpec.Ports[0].Name
			service.Spec.Ports[0].Port = expectServiceSpec.Ports[0].Port
			service.Spec.Ports[0].TargetPort = expectServiceSpec.Ports[0].TargetPort
			service.Spec.Ports[0].Protocol = expectServiceSpec.Ports[0].Protocol
		}
		return controllerutil.SetControllerReference(devbox, service, r.Scheme)
	}); err != nil {
		return err
	}

	// Retrieve the updated Service to get the NodePort
	var updatedService corev1.Service
	if err := r.Client.Get(ctx, client.ObjectKey{Namespace: service.Namespace, Name: service.Name}, &updatedService); err != nil {
		return err
	}

	// Extract the NodePort
	nodePort := int32(0)
	for _, port := range updatedService.Spec.Ports {
		if port.NodePort != 0 {
			nodePort = port.NodePort
			break
		}
	}
	if nodePort == 0 {
		return fmt.Errorf("NodePort not found for service %s", service.Name)
	} else {
		devbox.Status.Network.NodePort = nodePort
		if err := r.Status().Update(ctx, devbox); err != nil {
			return err
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DevboxReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&devboxv1alpha1.Devbox{}).
		Owns(&corev1.Pod{}).Owns(&corev1.Service{}).
		Complete(r)
}
