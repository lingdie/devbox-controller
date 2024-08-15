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
	devboxv1alpha1 "github.com/labring/sealos/controllers/devbox/api/v1alpha1"
	"github.com/labring/sealos/controllers/devbox/internal/controller/utils/tag"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// DevBoxReleaseReconciler reconciles a DevBoxRelease object
type DevBoxReleaseReconciler struct {
	client.Client
	TagClient tag.ReleaseTagClient
	Scheme    *runtime.Scheme
}

const (
	DevboxReleaseTagged    = "Tagged"
	DevboxReleaseNotTagged = "NotTagged"
	DevboxReleaseFailed    = "Failed"
)

// +kubebuilder:rbac:groups=devbox.sealos.io,resources=devboxreleases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=devbox.sealos.io,resources=devboxreleases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=devbox.sealos.io,resources=devboxreleases/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DevBoxRelease object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/reconcile
func (r *DevBoxReleaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	devboxRelease := &devboxv1alpha1.DevBoxRelease{}
	if err := r.Client.Get(ctx, req.NamespacedName, devboxRelease); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if devboxRelease.ObjectMeta.DeletionTimestamp.IsZero() {
		if controllerutil.AddFinalizer(devboxRelease, FinalizerName) {
			if err := r.Update(ctx, devboxRelease); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		if controllerutil.RemoveFinalizer(devboxRelease, FinalizerName) {
			if err := r.Update(ctx, devboxRelease); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if len(devboxRelease.Status.Phase) == 0 {
		devboxRelease.Status.Phase = DevboxReleaseNotTagged
		err := r.Update(ctx, devboxRelease)
		if err != nil {
			return ctrl.Result{}, err
		}
		username := "mlhiter"
		password := "9wv4sWHL!t8GFmD"
		repositoryName := "mlhiter"
		devbox := &devboxv1alpha1.Devbox{}
		devboxInfo := types.NamespacedName{
			Name:      devboxRelease.Spec.DevboxName,
			Namespace: devboxRelease.Namespace,
		}
		if err := r.Get(ctx, devboxInfo, devbox); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}

		//imageName := devbox.Status.Commit.CommitID
		//oldTag := devboxRelease.Spec.OldTag

		newTag := devboxRelease.Spec.NewTag
		err = r.TagClient.TagImage(username, password, repositoryName, imageName, oldTag, newTag)
		if err != nil {
			return ctrl.Result{}, err
		}
		err = r.Update(ctx, devboxRelease)
		if err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DevBoxReleaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&devboxv1alpha1.DevBoxRelease{}).
		Complete(r)
}
