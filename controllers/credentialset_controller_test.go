//go:build integration

package controllers

import (
	"context"
	"testing"

	porterv1 "get.porter.sh/operator/api/v1"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCredentialSetReconiler_Reconcile(t *testing.T) {
	ctx := context.Background()

	namespace := "test"
	name := "mybuns"
	testdata := []client.Object{
		&porterv1.CredentialSet{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, Generation: 1},
		},
	}
	controller := setupCredentialSetController(testdata...)

	var cs porterv1.CredentialSet
	triggerReconcile := func() {
		fullname := types.NamespacedName{Namespace: namespace, Name: name}
		key := client.ObjectKey{Namespace: namespace, Name: name}
		request := controllerruntime.Request{
			NamespacedName: fullname,
		}
		result, err := controller.Reconcile(ctx, request)
		require.NoError(t, err)
		require.True(t, result.IsZero())

		err = controller.Get(ctx, key, &cs)
		if !apierrors.IsNotFound(err) {
			require.NoError(t, err)
		}
	}
	triggerReconcile()
}

func TestCredentialSetReconciler_createAgentAction(t *testing.T) {

}

func setupCredentialSetController(objs ...client.Object) CredentialSetReconciler {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(porterv1.AddToScheme(scheme))

	fakeBuilder := fake.NewClientBuilder()
	fakeBuilder.WithScheme(scheme)
	fakeBuilder.WithObjects(objs...)
	fakeClient := fakeBuilder.Build()

	return CredentialSetReconciler{
		Log:    logr.Discard(),
		Client: fakeClient,
		Scheme: scheme,
	}
}
