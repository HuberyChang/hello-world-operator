/*
Copyright 2023.

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

package controllers

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	opeartorv1 "github.com/HuberyChang/hello-world-operator/api/v1"
	//apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const helloworldFinalizer = "operator.example.com/finalizer"

//typeAvailableHelloWorld 表示 Deployment 解耦的状态。
const typeAvailableHelloWorld = "Available"

//typeDegradedHelloWorld 表示当自定义资源被删除时必须发生的终结器操作的状态。
const typeDegradedHelloWorld = "Degraded"

// HelloWorldReconciler reconciles a HelloWorld object
type HelloWorldReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=opeartor.example.com,resources=helloworlds,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=opeartor.example.com,resources=helloworlds/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=opeartor.example.com,resources=helloworlds/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the HelloWorld object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.1/pkg/reconcile
func (r *HelloWorldReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// TODO(user): your logic here
	helloworld := &opeartorv1.HelloWorld{}
	err := r.Get(ctx, req.NamespacedName, helloworld)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("helloworld resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get helloworld")
		return ctrl.Result{}, err
	}

	if helloworld.Status.Conditions == nil || len(helloworld.Status.Conditions) == 0 {
		meta.SetStatusCondition(&helloworld.Status.Conditions, metav1.Condition{Type: typeAvailableHelloWorld, Status: metav1.ConditionUnknown,
			Reason: "Reconciling", Message: "Starting reconciliation"})
		if err = r.Status().Update(ctx, helloworld); err != nil {
			logger.Error(err, "Failed to update HelloWorld status")
			return ctrl.Result{}, err
		}

		if err := r.Get(ctx, req.NamespacedName, helloworld); err != nil {
			logger.Error(err, "Failed to re-fetch helloworld")
			return ctrl.Result{}, err
		}
	}

	if !controllerutil.ContainsFinalizer(helloworld, helloworldFinalizer) {
		logger.Info("Adding Finalizer for HelloWorld")
		if ok := controllerutil.AddFinalizer(helloworld, helloworldFinalizer); !ok {
			logger.Error(err, "Failed to add Finalizer into the custom resource")
			return ctrl.Result{Requeue: true}, nil
		}

		if err = r.Update(ctx, helloworld); err != nil {
			logger.Error(err, "Failed to update custom resource to add finalizer")
			return ctrl.Result{}, err
		}
	}

	isHelloWorldMarkedToBeDeleted := helloworld.GetDeletionTimestamp() != nil
	if isHelloWorldMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(helloworld, helloworldFinalizer) {
			logger.Info("Performing finalizer operations for HelloWorld before delete CR")
			meta.SetStatusCondition(&helloworld.Status.Conditions, metav1.Condition{Type: typeDegradedHelloWorld, Status: metav1.ConditionUnknown,
				Reason: "Finalizing", Message: fmt.Sprintf("Performing finalizer operations for the custom resource: %s", helloworld.Name)})

			if err := r.Status().Update(ctx, helloworld); err != nil {
				logger.Error(err, "Failed to update HelloWorld status")
				return ctrl.Result{}, err
			}

			r.doFinalizerOperationsForHelloWorld(helloworld)

			if err := r.Get(ctx, req.NamespacedName, helloworld); err != nil {
				logger.Error(err, "Failed to re-fetch helloworld")
				return ctrl.Result{}, err
			}

			meta.SetStatusCondition(&helloworld.Status.Conditions, metav1.Condition{Type: typeDegradedHelloWorld, Status: metav1.ConditionTrue,
				Reason: "Finalizing", Message: fmt.Sprintf("Finalizer operations for custom resource %s name were successfully accomplished",
					helloworld.Name)})

			if err := r.Status().Update(ctx, helloworld); err != nil {
				logger.Error(err, "Failed to update HelloWorld status")
				return ctrl.Result{}, err
			}

			logger.Info("Removing Finalizer for HelloWorld after successfully perform the operations")
			if ok := controllerutil.RemoveFinalizer(helloworld, helloworldFinalizer); !ok {
				logger.Error(err, "Failed to remove Finalizer for helloworld ")
				return ctrl.Result{Requeue: true}, nil
			}

			if err := r.Update(ctx, helloworld); err != nil {
				logger.Error(err, "Failed to remove finalizer for HelloWorld")
				return ctrl.Result{}, err
			}

		}
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

func (r *HelloWorldReconciler) doFinalizerOperationsForHelloWorld(hd *opeartorv1.HelloWorld) {
	r.Recorder.Event(hd, "Warning", "Deleting", fmt.Sprintf("Custom Resource %s is being deleted from the namespace %s", hd.Name, hd.Namespace))
}

// SetupWithManager sets up the controller with the Manager.
func (r *HelloWorldReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&opeartorv1.HelloWorld{}).
		Complete(r)
}
