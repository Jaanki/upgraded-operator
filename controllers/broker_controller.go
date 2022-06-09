/*
Copyright 2022.

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
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/submariner-io/submariner-operator/pkg/crd"
	"github.com/submariner-io/submariner-operator/pkg/discovery/globalnet"
	"github.com/submariner-io/submariner-operator/pkg/gateway"
	"github.com/submariner-io/submariner-operator/pkg/lighthouse"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/submariner-io/submariner-operator/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// BrokerReconciler reconciles a Broker object
type BrokerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config *rest.Config
	Log    logr.Logger
}

//+kubebuilder:rbac:groups=submariner.io,resources=brokers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=submariner.io,resources=brokers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=submariner.io,resources=brokers/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.2/pkg/reconcile
func (r *BrokerReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	_ = r.Log.WithValues("broker", request.NamespacedName)

	// Fetch the Broker instance
	instance := &v1alpha1.Broker{}

	err := r.Client.Get(ctx, request.NamespacedName, instance)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, errors.Wrap(err, "error retrieving Broker resource")
	}

	if instance.ObjectMeta.DeletionTimestamp != nil {
		// Graceful deletion has been requested, ignore the object
		return reconcile.Result{}, nil
	}

	kubeClient, err := kubernetes.NewForConfig(r.Config)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "error creating kube client")
	}

	// Broker CRDs
	crdUpdater := crd.UpdaterFromControllerClient(r.Client)

	err = gateway.Ensure(crdUpdater)
	if err != nil {
		return ctrl.Result{}, err // nolint:wrapcheck // Errors are already wrapped
	}

	// Lighthouse CRDs
	_, err = lighthouse.Ensure(crdUpdater, lighthouse.BrokerCluster)
	if err != nil {
		return ctrl.Result{}, err // nolint:wrapcheck // Errors are already wrapped
	}

	// Globalnet
	err = globalnet.ValidateExistingGlobalNetworks(kubeClient, request.Namespace)
	if err != nil {
		return ctrl.Result{}, err // nolint:wrapcheck // Errors are already wrapped
	}

	err = globalnet.CreateConfigMap(kubeClient, instance.Spec.GlobalnetEnabled, instance.Spec.GlobalnetCIDRRange,
		instance.Spec.DefaultGlobalnetClusterSize, request.Namespace)
	if err != nil {
		return ctrl.Result{}, err // nolint:wrapcheck // Errors are already wrapped
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BrokerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Broker{}).
		Complete(r)
}
