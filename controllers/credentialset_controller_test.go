//go:build integration

package controllers

import (
	"context"
	"testing"
	"time"

	porterv1 "get.porter.sh/operator/api/v1"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
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

	// Verify the credential set was picked up and the status initialized
	assert.Equal(t, porterv1.PhaseUnknown, cs.Status.Phase, "New resources should be initialized to Phase: Unknown")

	triggerReconcile()

	// Verify an AgentAction was created and set on the status
	require.NotNil(t, cs.Status.Action, "expected Action to be set")
	var action porterv1.AgentAction
	require.NoError(t, controller.Get(ctx, client.ObjectKey{Namespace: cs.Namespace, Name: cs.Status.Action.Name}, &action))
	assert.Equal(t, "1", action.Labels[porterv1.LabelResourceGeneration], "The wrong action is set on the status")

	// Mark the action as scheduled
	action.Status.Phase = porterv1.PhasePending
	action.Status.Conditions = []metav1.Condition{{Type: string(porterv1.ConditionScheduled), Status: metav1.ConditionTrue}}
	require.NoError(t, controller.Status().Update(ctx, &action))

	triggerReconcile()

	// Verify the credential set status was synced with the action
	assert.Equal(t, porterv1.PhasePending, cs.Status.Phase, "incorrect Phase")
	assert.True(t, apimeta.IsStatusConditionTrue(cs.Status.Conditions, string(porterv1.ConditionScheduled)))

	// Mark the action as started
	action.Status.Phase = porterv1.PhaseRunning
	action.Status.Conditions = []metav1.Condition{{Type: string(porterv1.ConditionStarted), Status: metav1.ConditionTrue}}
	require.NoError(t, controller.Status().Update(ctx, &action))

	triggerReconcile()

	// Verify the credential set status was synced with the action
	assert.Equal(t, porterv1.PhaseRunning, cs.Status.Phase, "incorrect Phase")
	assert.True(t, apimeta.IsStatusConditionTrue(cs.Status.Conditions, string(porterv1.ConditionStarted)))

	// Complete the action
	action.Status.Phase = porterv1.PhaseSucceeded
	action.Status.Conditions = []metav1.Condition{{Type: string(porterv1.ConditionComplete), Status: metav1.ConditionTrue}}
	require.NoError(t, controller.Status().Update(ctx, &action))

	triggerReconcile()

	// Verify the credential set status was synced with the action
	assert.NotNil(t, cs.Status.Action, "expected Action to still be set")
	assert.Equal(t, porterv1.PhaseSucceeded, cs.Status.Phase, "incorrect Phase")
	assert.True(t, apimeta.IsStatusConditionTrue(cs.Status.Conditions, string(string(porterv1.ConditionComplete))))

	// Fail the action
	action.Status.Phase = porterv1.PhaseFailed
	action.Status.Conditions = []metav1.Condition{{Type: string(porterv1.ConditionFailed), Status: metav1.ConditionTrue}}
	require.NoError(t, controller.Status().Update(ctx, &action))

	triggerReconcile()

	// Verify that the credential set status shows the action is failed
	require.NotNil(t, cs.Status.Action, "expected Action to still be set")
	assert.Equal(t, porterv1.PhaseFailed, cs.Status.Phase, "incorrect Phase")
	assert.True(t, apimeta.IsStatusConditionTrue((cs.Status.Conditions), string(porterv1.ConditionFailed)))

	// Edit the generation spec
	cs.Generation = 2
	require.NoError(t, controller.Update(ctx, &cs))

	triggerReconcile()

	// Verify that the credential set status was re-initialized
	assert.Equal(t, int64(2), cs.Status.ObservedGeneration)
	assert.Equal(t, porterv1.PhaseUnknown, cs.Status.Phase, "New resources should be initialized to Phase: Unknown")
	assert.Empty(t, cs.Status.Conditions, "Conditions should have been reset")

	// Retry the last action
	lastAction := cs.Status.Action.Name
	cs.Annotations = map[string]string{porterv1.AnnotationRetry: "retry-1"}
	require.NoError(t, controller.Update(ctx, &cs))

	triggerReconcile()

	// Verify that action has retry set on it now
	require.NotNil(t, cs.Status.Action, "Expected the action to still be set")
	assert.Equal(t, lastAction, cs.Status.Action.Name, "Expected the action to be the same")
	// get the latest version of the action
	require.NoError(t, controller.Get(ctx, client.ObjectKey{Namespace: cs.Namespace, Name: cs.Status.Action.Name}, &action))
	assert.NotEmpty(t, action.Annotations[porterv1.AnnotationRetry], "Expected the action to have its retry annotation set")

	assert.Equal(t, int64(2), cs.Status.ObservedGeneration)
	assert.NotEmpty(t, cs.Status.Action, "Expected the action to still be set")
	assert.Equal(t, porterv1.PhaseUnknown, cs.Status.Phase, "New resources should be initialized to Phase Unknown")
	assert.Empty(t, cs.Status.Conditions, "Conditions should have been reset")

	// Delete the credential set (setting the delete timestamp directly instead of client.Delete because otherwise the fake client just removes it immediately)
	// The fake client doesn't really follow finalizer logic
	now := metav1.NewTime(time.Now())
	cs.Generation = 3
	cs.DeletionTimestamp = &now
	require.NoError(t, controller.Update(ctx, &cs))

	triggerReconcile()

	// Verify that an action was created to delete it
	require.NotNil(t, cs.Status.Action, "expected Action to be set")
	require.NoError(t, controller.Get(ctx, client.ObjectKey{Namespace: cs.Namespace, Name: cs.Status.Action.Name}, &action))
	assert.Equal(t, "3", action.Labels[porterv1.LabelResourceGeneration], "The wrong action is set on the status")

	// Complete the delete action
	action.Status.Phase = porterv1.PhaseSucceeded
	action.Status.Conditions = []metav1.Condition{{Type: string(porterv1.ConditionComplete), Status: metav1.ConditionTrue}}
	require.NoError(t, controller.Status().Update(ctx, &action))

	triggerReconcile()

	// Verify that the credential set was removed
	err := controller.Get(ctx, client.ObjectKeyFromObject(&cs), &cs)
	require.True(t, apierrors.IsNotFound(err), "expected the credential set was deleted")

	// Verify that the reconcile doesn't error out after its deleted
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
