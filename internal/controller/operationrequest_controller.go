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
	"github.com/go-logr/logr"
	"github.com/labring/sealos/controllers/devbox/label"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	devboxv1alpha1 "github.com/labring/sealos/controllers/devbox/api/v1alpha1"
)

// OperationRequestReconciler reconciles a OperationRequest object
type OperationRequestReconciler struct {
	client.Client
	Logger   logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	// expirationTime is the time duration of the request is expired
	expirationTime time.Duration
	// retentionTime is the time duration of the request is retained after it is isCompleted
	retentionTime       time.Duration
	CommitImageRegistry string
}

// OperationReqRequeueDuration is the time interval to reconcile a OperationRequest if no error occurs
const OperationReqRequeueDuration time.Duration = 30 * time.Second

// +kubebuilder:rbac:groups=devbox.sealos.io,resources=operationrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=devbox.sealos.io,resources=operationrequests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=devbox.sealos.io,resources=operationrequests/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the OperationRequest object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/reconcile
func (r *OperationRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx, "devbox", req.NamespacedName)
	request := &devboxv1alpha1.OperationRequest{}
	if err := r.Get(ctx, req.NamespacedName, request); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// delete OperationRequest first if its status is isCompleted and exist for retention time
	if r.isRetained(request) {
		logger.Info("delete request", getLog(request)...)
		if err := r.deleteRequest(ctx, request); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	// return early if its status is isCompleted and didn't exist for retention time
	if r.isCompleted(request) {
		r.Logger.V(1).Info("request is completed and requeue", getLog(request)...)
		return ctrl.Result{RequeueAfter: OperationReqRequeueDuration}, nil
	}
	// change OperationRequest status to failed if it is expired
	if r.isExpired(request) {
		r.Logger.V(1).Info("request is expired, update status to failed", getLog(request)...)
		if err := r.updateRequestStatus(ctx, request, devboxv1alpha1.RequestFailed); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	// update OperationRequest status to processing
	err := r.updateRequestStatus(ctx, request, devboxv1alpha1.RequestProcessing)
	if err != nil {
		return ctrl.Result{}, err
	}
	devbox := &devboxv1alpha1.Devbox{}
	devboxInfo := types.NamespacedName{
		Name:      request.Spec.DevBoxName,
		Namespace: request.Namespace,
	}
	if err := r.Get(ctx, devboxInfo, devbox); err != nil {
		return ctrl.Result{}, err
	}
	devboxPod := &corev1.Pod{}
	err = r.Get(ctx, client.ObjectKey{Namespace: devbox.Namespace, Name: devbox.Name}, devboxPod)
	switch request.Spec.Action {
	case devboxv1alpha1.Open:
		if err != nil && client.IgnoreNotFound(err) != nil {
			// error other than not found
			logger.Error(err, "get devbox pod failed")
			return ctrl.Result{}, err
		} else if err != nil && client.IgnoreNotFound(err) == nil {
			// no devbox pod found, create a new one
			err = r.CreatePod(ctx, devbox, req)
		} else if err == nil {
			// no need to create pod if it already exists and is running or pending
			if devboxPod.Status.Phase == corev1.PodRunning || devboxPod.Status.Phase == corev1.PodPending {
				return ctrl.Result{}, err
			}
		}
	case devboxv1alpha1.Close:
		if err != nil && client.IgnoreNotFound(err) != nil {
			// error other than not found
			logger.Error(err, "get devbox pod failed")
			return ctrl.Result{}, err
		} else if err != nil && client.IgnoreNotFound(err) == nil {
			// no devbox pod found, do nothing
		} else if err == nil {
			// no need to create pod if it already exists and is running or pending
			if devboxPod.Status.Phase != corev1.PodSucceeded {
				err = r.Delete(ctx, devboxPod)
				if err != nil {
					logger.Error(err, "delete devbox pod failed")
					return ctrl.Result{}, err
				}
			}
		}
	}
	// TODO(user): your logic here

	return ctrl.Result{}, nil
}
func (r *OperationRequestReconciler) generateImageName(devbox *devboxv1alpha1.Devbox) string {
	now := time.Now()
	return fmt.Sprintf("%s/%s/%s:%s", r.CommitImageRegistry, devbox.Namespace, devbox.Name, now.Format("2006-01-02-150405"))
}

func (r *OperationRequestReconciler) CreatePod(ctx context.Context, devbox *devboxv1alpha1.Devbox, req ctrl.Request) error {
	logger := log.FromContext(ctx, "devbox", req.NamespacedName)
	nextCommitHistory := devboxv1alpha1.CommitHistory{
		Image:  r.generateImageName(devbox),
		Time:   metav1.Now(),
		Status: devboxv1alpha1.CommitStatusPending,
	}

	recLabels := label.RecommendedLabels(&label.Recommended{
		Name:      devbox.Name,
		ManagedBy: label.DefaultManagedBy,
		PartOf:    DevBoxPartOf,
	})

	objectMeta := metav1.ObjectMeta{
		Name:      devbox.Name,
		Namespace: devbox.Namespace,
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
		{
			Name:  "SEALOS_COMMIT_ON_STOP",
			Value: "true",
		},
		{
			Name:  "SEALOS_COMMIT_IMAGE_NAME",
			Value: nextCommitHistory.Image,
		},
	}

	//get image name
	imageName, err := r.getImageName(ctx, devbox)
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
			RestartPolicy: corev1.RestartPolicyNever,
			Containers:    containers,
		},
	}
	if err = controllerutil.SetControllerReference(devbox, expectPod, r.Scheme); err != nil {
		return err
	}
	if err = r.Create(ctx, expectPod); err != nil {
		logger.Error(err, "create pod failed")
		return err
	}
	return nil
}

func (r *OperationRequestReconciler) getImageName(ctx context.Context, devbox *devboxv1alpha1.Devbox) (string, error) {
	// get image name from runtime if commit history is empty
	if devbox.Status.CommitHistory == nil || len(devbox.Status.CommitHistory) == 0 {
		rt := &devboxv1alpha1.Runtime{}
		err := r.Get(ctx, client.ObjectKey{Namespace: devbox.Namespace, Name: devbox.Spec.RuntimeRef.Name}, rt)
		if err != nil {
			return "", err
		}
		return rt.Spec.Image, nil
	}
	// get image name from commit history, ues the latest commit history
	commitHistory := devbox.Status.CommitHistory[len(devbox.Status.CommitHistory)-1]
	return commitHistory.Image, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OperationRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&devboxv1alpha1.OperationRequest{}).
		Complete(r)
}

// isRetained returns true if the request is isCompleted and exist for retention time
func (r *OperationRequestReconciler) isRetained(request *devboxv1alpha1.OperationRequest) bool {
	if request.Status.Phase == devboxv1alpha1.RequestCompleted && request.CreationTimestamp.Add(r.retentionTime).Before(time.Now()) {
		return true
	}
	return false
}

// isCompleted returns true if the request is isCompleted
func (r *OperationRequestReconciler) isCompleted(request *devboxv1alpha1.OperationRequest) bool {
	return request.Status.Phase == devboxv1alpha1.RequestCompleted
}

// isExpired returns true if the request is expired
func (r *OperationRequestReconciler) isExpired(request *devboxv1alpha1.OperationRequest) bool {
	if request.Status.Phase != devboxv1alpha1.RequestCompleted && request.CreationTimestamp.Add(r.expirationTime).Before(time.Now()) {
		return true
	}
	return false
}

func (r *OperationRequestReconciler) updateRequestStatus(ctx context.Context, request *devboxv1alpha1.OperationRequest, phase devboxv1alpha1.RequestPhase) error {
	request.Status.Phase = phase
	if err := r.Status().Update(ctx, request); err != nil {
		r.Recorder.Eventf(request, v1.EventTypeWarning, "Failed to update OperationRequest status", "Failed to update OperationRequest status %s/%s", request.Namespace, request.Name)
		r.Logger.V(1).Info("update OperationRequest status failed", getLog(request)...)
		return err
	}
	r.Logger.V(1).Info("update OperationRequest status success", getLog(request)...)
	return nil
}

func (r *OperationRequestReconciler) deleteRequest(ctx context.Context, request *devboxv1alpha1.OperationRequest) error {
	r.Logger.V(1).Info("deleting OperationRequest", "request", request)
	if err := r.Delete(ctx, request); client.IgnoreNotFound(err) != nil {
		r.Recorder.Eventf(request, v1.EventTypeWarning, "Failed to delete OperationRequest", "Failed to delete OperationRequest %s/%s", request.Namespace, request.Name)
		r.Logger.Error(err, "Failed to delete OperationRequest", getLog(request)...)
		return fmt.Errorf("failed to delete OperationRequest %s/%s: %w", request.Namespace, request.Name, err)
	}
	r.Logger.V(1).Info("delete OperationRequest success", getLog(request)...)
	return nil
}

func getLog(request *devboxv1alpha1.OperationRequest, kv ...interface{}) []interface{} {
	return append([]interface{}{
		"request.name", request.Name,
		"request.namespace", request.Namespace,
		"request.action", request.Spec.Action,
		"request.phase", request.Status.Phase,
	}, kv...)
}
