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
	"os"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
	// apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const helloworldFinalizer = "operator.example.com/finalizer"

// typeAvailableHelloWorld 表示 Deployment 解耦的状态。
const typeAvailableHelloWorld = "Available"

// typeDegradedHelloWorld 表示当自定义资源被删除时必须发生的终结器操作的状态。
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
		meta.SetStatusCondition(&helloworld.Status.Conditions, metav1.Condition{
			Type: typeAvailableHelloWorld, Status: metav1.ConditionUnknown,
			Reason: "Reconciling", Message: "Starting reconciliation",
		})
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
			meta.SetStatusCondition(&helloworld.Status.Conditions, metav1.Condition{
				Type: typeDegradedHelloWorld, Status: metav1.ConditionUnknown,
				Reason: "Finalizing", Message: fmt.Sprintf("Performing finalizer operations for the custom resource: %s", helloworld.Name),
			})

			if err := r.Status().Update(ctx, helloworld); err != nil {
				logger.Error(err, "Failed to update HelloWorld status")
				return ctrl.Result{}, err
			}

			r.doFinalizerOperationsForHelloWorld(helloworld)

			if err := r.Get(ctx, req.NamespacedName, helloworld); err != nil {
				logger.Error(err, "Failed to re-fetch helloworld")
				return ctrl.Result{}, err
			}

			meta.SetStatusCondition(&helloworld.Status.Conditions, metav1.Condition{
				Type: typeDegradedHelloWorld, Status: metav1.ConditionTrue,
				Reason: "Finalizing", Message: fmt.Sprintf("Finalizer operations for custom resource %s name were successfully accomplished",
					helloworld.Name),
			})

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

// deploymentForHelloWorld返回一个HelloWorld Deployment对象
func (r *HelloWorldReconciler) deploymentForHelloWorld(helloworld *opeartorv1.HelloWorld) (*appsv1.Deployment, error) {
	labels := labelsForHelloWorld(helloworld.Name)
	replicas := helloworld.Spec.Size

	image, err := imagesForHelloWorld()
	if err != nil {
		return nil, err
	}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      helloworld.Name,
			Namespace: helloworld.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &[]bool{true}[0],
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{
						{
							Image:           image,
							Name:            "helloworld",
							ImagePullPolicy: corev1.PullIfNotPresent,
							Env: []corev1.EnvVar{
								{
									Name:  "HELLO_TEXT",
									Value: helloworld.Spec.Text,
								},
							},
							SecurityContext: &corev1.SecurityContext{
								RunAsNonRoot:             &[]bool{true}[0],
								AllowPrivilegeEscalation: &[]bool{false}[0],
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{
										"ALL",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if err := ctrl.SetControllerReference(helloworld, dep, r.Scheme); err != nil {
		return nil, err
	}

	return dep, nil
}

func labelsForHelloWorld(name string) map[string]string {
	var imageTag string
	image, err := imagesForHelloWorld()
	if err != nil {
		imageTag = strings.Split(image, ":")[1]
	}
	return map[string]string{
		"app.k8s.io/instance":   name,
		"app.k8s.io/version":    imageTag,
		"app.k8s.io/part-of":    "hello-world-operator",
		"app.k8s.io/created-by": "controller-manager",
	}
}

// imageForHelloWorld函数从HELLWORLD_IMAGE环境变量中获取由该控制器管理的operand(操作对象)镜像,该环境变量定义在config/manager/manager.yaml中
func imagesForHelloWorld() (string, error) {
	imageEnvVar := "HELLOWORLD_IMAGE"
	image, found := os.LookupEnv(imageEnvVar)
	if !found {
		return "", fmt.Errorf("Unable to find %s environment variable with the image", imageEnvVar)
	}
	return image, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *HelloWorldReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&opeartorv1.HelloWorld{}).
		Complete(r)
}
