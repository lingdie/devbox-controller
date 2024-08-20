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
	"k8s.io/apimachinery/pkg/util/rand"
	"time"

	devboxv1alpha1 "github.com/labring/sealos/controllers/devbox/api/v1alpha1"
	"github.com/labring/sealos/controllers/devbox/label"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	rate          = 10
	FinalizerName = "devbox.sealos.io/finalizer"
	Devbox        = "devbox"
	DevBoxPartOf  = "devbox"
)

// DevboxReconciler reconciles a Devbox object
type DevboxReconciler struct {
	CommitImageRegistry string

	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=devbox.sealos.io,resources=devboxes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=devbox.sealos.io,resources=devboxes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=devbox.sealos.io,resources=devboxes/finalizers,verbs=update

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

	devbox.Status.Network.Type = devbox.Spec.NetworkSpec.Type
	_ = r.Status().Update(ctx, devbox)

	recLabels := label.RecommendedLabels(&label.Recommended{
		Name:      devbox.Name,
		ManagedBy: label.DefaultManagedBy,
		PartOf:    DevBoxPartOf,
	})

	// create or update secret
	if err := r.syncSecret(ctx, devbox, recLabels); err != nil {
		logger.Error(err, "create or update secret failed")
		r.Recorder.Eventf(devbox, corev1.EventTypeWarning, "Create secret failed", "%v", err)
		return ctrl.Result{}, err
	}

	// if devbox is running, create or update pod
	if devbox.Spec.State == devboxv1alpha1.DevboxStateRunning {
		if err := r.syncPod(ctx, devbox, recLabels); err != nil {
			logger.Error(err, "create or update pod failed")
			r.Recorder.Eventf(devbox, corev1.EventTypeWarning, "Create pod failed", "%v", err)
			return ctrl.Result{}, err
		}
	} else {
		// if devbox is not running, delete pod if exists
		pod := &corev1.Pod{}
		err := r.Get(ctx, client.ObjectKey{Namespace: devbox.Namespace, Name: devbox.Name}, pod)
		if err != nil && client.IgnoreNotFound(err) != nil {
			logger.Error(err, "get devbox pod failed")
			return ctrl.Result{}, err
		} else if err == nil {
			if err = r.Delete(ctx, pod); err != nil {
				logger.Error(err, "delete devbox pod failed")
				return ctrl.Result{}, err
			}
		}
	}

	// create service if network type is NodePort
	if devbox.Spec.NetworkSpec.Type == devboxv1alpha1.NetworkTypeNodePort {
		if err := r.syncService(ctx, devbox, recLabels); err != nil {
			logger.Error(err, "Create service failed")
			r.Recorder.Eventf(devbox, corev1.EventTypeWarning, "Create service failed", "%v", err)
			return ctrl.Result{RequeueAfter: time.Second * 3}, err
		}
	}
	r.Recorder.Eventf(devbox, corev1.EventTypeNormal, "Created", "create devbox success: %v", devbox.ObjectMeta.Name)
	return ctrl.Result{Requeue: false}, nil
}

func (r *DevboxReconciler) syncSecret(ctx context.Context, devbox *devboxv1alpha1.Devbox, recLabels map[string]string) error {
	logger := log.FromContext(ctx, "devbox", devbox.Name, "namespace", devbox.Namespace)
	objectMeta := metav1.ObjectMeta{
		Name:      devbox.Name,
		Namespace: devbox.Namespace,
		Labels:    recLabels,
	}
	devboxSecret := &corev1.Secret{
		ObjectMeta: objectMeta,
	}

	err := r.Get(ctx, client.ObjectKey{Namespace: devbox.Namespace, Name: devbox.Name}, devboxSecret)
	if err != nil && client.IgnoreNotFound(err) != nil {
		logger.Error(err, "get devbox secret failed")
		return err
	}
	// if secret not found, create a new one
	if err != nil && client.IgnoreNotFound(err) == nil {
		// set password to context, if error then no need to update secret
		secret := &corev1.Secret{
			ObjectMeta: objectMeta,
			Data:       map[string][]byte{"SEALOS_DEVBOX_PASSWORD": []byte(rand.String(12))},
		}
		if err := controllerutil.SetControllerReference(devbox, secret, r.Scheme); err != nil {
			return err
		}
		if err := r.Create(ctx, secret); err != nil {
			logger.Error(err, "create devbox secret failed")
			return err
		}
		return nil
	}
	return nil
}

func (r *DevboxReconciler) syncPod(ctx context.Context, devbox *devboxv1alpha1.Devbox, recLabels map[string]string) error {
	logger := log.FromContext(ctx, "devbox", devbox.Name, "namespace", devbox.Namespace)

	objectMeta := metav1.ObjectMeta{
		Name:      devbox.Name,
		Namespace: devbox.Namespace,
		Labels:    recLabels,
	}
	devboxPod := &corev1.Pod{
		ObjectMeta: objectMeta,
	}
	err := r.Get(ctx, client.ObjectKey{Namespace: devbox.Namespace, Name: devbox.Name}, devboxPod)
	if err != nil && client.IgnoreNotFound(err) != nil {
		// error other than not found
		logger.Error(err, "get devbox pod failed")
		return err
	} else if err != nil && client.IgnoreNotFound(err) == nil {
		// no devbox pod found, create a new one
		// create a new commit history if we need recreate pod for next commit

		//  check commit status, if devbox pod is pending, we assume the commit status is failed because the pod is not created
		if devbox.Status.DevboxPodPhase == corev1.PodPending {
			if len(devbox.Status.CommitHistory) != 0 {
				devbox.Status.CommitHistory[len(devbox.Status.CommitHistory)-1].Status = devboxv1alpha1.CommitStatusFailed
			}
		} else {
			// if devbox pod is not pending, we assume the commit status is success
			if len(devbox.Status.CommitHistory) != 0 {
				devbox.Status.CommitHistory[len(devbox.Status.CommitHistory)-1].Status = devboxv1alpha1.CommitStatusSuccess
			}
		}
		if err := r.Status().Update(ctx, devbox); err != nil {
			logger.Error(err, "update devbox status failed")
			return err
		}

		nextCommitHistory := devboxv1alpha1.CommitHistory{
			Image:  r.generateImageName(devbox),
			Time:   metav1.Now(),
			Status: devboxv1alpha1.CommitStatusPending,
		}
		// recreate pod
		ports := []corev1.ContainerPort{
			{
				Name:          "ssh",
				Protocol:      corev1.ProtocolTCP,
				ContainerPort: 2222,
			},
		}
		ports = append(ports, devbox.Spec.NetworkSpec.ExtraPorts...)
		envs := []corev1.EnvVar{
			{
				Name:  "SEALOS_COMMIT_ON_STOP",
				Value: "true",
			},
			{
				Name:  "SEALOS_COMMIT_IMAGE_NAME",
				Value: nextCommitHistory.Image,
			},
			{
				Name: "SEALOS_DEVBOX_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						Key: "SEALOS_DEVBOX_PASSWORD",
						LocalObjectReference: corev1.LocalObjectReference{
							Name: devbox.Name,
						},
					},
				},
			},
		}

		//get image name
		imageName, err := r.getLastSuccessCommitImageName(ctx, devbox)
		if err != nil {
			logger.Error(err, "get image name failed")
			return err
		}
		containers := []corev1.Container{
			{
				Name:  devbox.ObjectMeta.Name,
				Image: imageName,
				Ports: ports,
				Env:   envs,
				Resources: corev1.ResourceRequirements{
					Requests: calculateResourceRequest(
						corev1.ResourceList{
							corev1.ResourceCPU:    devbox.Spec.Resource["cpu"],
							corev1.ResourceMemory: devbox.Spec.Resource["memory"],
						},
					),
					Limits: corev1.ResourceList{
						"cpu":    devbox.Spec.Resource["cpu"],
						"memory": devbox.Spec.Resource["memory"],
					},
				},
			},
		}
		expectPod := &corev1.Pod{
			ObjectMeta: objectMeta,
			Spec: corev1.PodSpec{
				RestartPolicy:                 corev1.RestartPolicyNever,
				Containers:                    containers,
				TerminationGracePeriodSeconds: pointer.Int64(300),
			},
		}
		if err = controllerutil.SetControllerReference(devbox, expectPod, r.Scheme); err != nil {
			return err
		}
		if err = r.Create(ctx, expectPod); err != nil {
			logger.Error(err, "create pod failed")
			return err
		}
		// add next commit history to status
		devbox.Status.CommitHistory = append(devbox.Status.CommitHistory, nextCommitHistory)
		return r.Status().Update(ctx, devbox)
	}
	// if pod exists, update devbox status using pod status
	devbox.Status.DevboxPodPhase = devboxPod.Status.Phase
	return r.Status().Update(ctx, devbox)
}

func (r *DevboxReconciler) getLastSuccessCommitImageName(ctx context.Context, devbox *devboxv1alpha1.Devbox) (string, error) {
	// get image name from runtime if commit history is empty
	rt := &devboxv1alpha1.Runtime{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: devbox.Namespace, Name: devbox.Spec.RuntimeRef.Name}, rt); err != nil {
		return "", err
	}
	if devbox.Status.CommitHistory == nil || len(devbox.Status.CommitHistory) == 0 {
		return rt.Spec.Image, nil
	}
	// get image name from commit history, ues the latest commit history
	for i := len(devbox.Status.CommitHistory) - 1; i >= 0; i-- {
		if devbox.Status.CommitHistory[i].Status == devboxv1alpha1.CommitStatusSuccess {
			return devbox.Status.CommitHistory[i].Image, nil
		}
	}
	// if all commit history is failed, get image name from runtime
	return rt.Spec.Image, nil
}

func (r *DevboxReconciler) syncService(ctx context.Context, devbox *devboxv1alpha1.Devbox, recLabels map[string]string) error {
	expectServiceSpec := corev1.ServiceSpec{
		Selector: recLabels,
		Type:     corev1.ServiceTypeNodePort,
		Ports: []corev1.ServicePort{
			{
				Name:       "tty",
				Port:       2222,
				TargetPort: intstr.FromInt32(2222),
				Protocol:   corev1.ProtocolTCP,
			},
		},
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      devbox.Name + "-svc",
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
	}
	devbox.Status.Network.Type = devboxv1alpha1.NetworkTypeNodePort
	devbox.Status.Network.NodePort = nodePort
	if err := r.Status().Update(ctx, devbox); err != nil {
		return err
	}
	return nil
}

func (r *DevboxReconciler) generateImageName(devbox *devboxv1alpha1.Devbox) string {
	now := time.Now()
	return fmt.Sprintf("%s/%s/%s:%s", r.CommitImageRegistry, devbox.Namespace, devbox.Name, now.Format("2006-01-02-150405"))
}

func calculateResourceRequest(limit corev1.ResourceList) corev1.ResourceList {
	if limit == nil {
		return nil
	}
	request := make(corev1.ResourceList)
	// Calculate CPU request
	if cpu, ok := limit[corev1.ResourceCPU]; ok {
		cpuValue := cpu.AsApproximateFloat64()
		cpuRequest := cpuValue / rate
		request[corev1.ResourceCPU] = *resource.NewMilliQuantity(int64(cpuRequest*1000), resource.DecimalSI)
	}
	// Calculate memory request
	if memory, ok := limit[corev1.ResourceMemory]; ok {
		memoryValue := memory.AsApproximateFloat64()
		memoryRequest := memoryValue / rate
		request[corev1.ResourceMemory] = *resource.NewQuantity(int64(memoryRequest), resource.BinarySI)
	}
	return request
}

// SetupWithManager sets up the controller with the Manager.
func (r *DevboxReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&devboxv1alpha1.Devbox{}).
		Owns(&corev1.Pod{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
