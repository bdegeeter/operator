//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"time"

	"github.com/carolynvs/magex/shx"
	. "github.com/onsi/ginkgo"
	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	porterv1 "get.porter.sh/operator/api/v1"
	. "github.com/onsi/gomega"
)

var _ = Describe("CredSet create", func() {
	Context("when a new CredentialSet resource is created with secret source", func() {
		It("should run porter", func() {
			By("creating an agent action", func() {
				ctx := context.Background()
				ns := createTestNamespace(ctx)
				name := "test-cs-" + ns
				testSecret := "foo"

				Log(fmt.Sprintf("create k8s secret '%s' for credset", name))

				csSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: ns,
					},
					Type:       corev1.SecretTypeOpaque,
					StringData: map[string]string{"value": testSecret},
				}
				Expect(k8sClient.Create(ctx, csSecret)).Should(Succeed())

				Log(fmt.Sprintf("create credential set '%s' for credset", name))
				cs := NewTestCredSet(name)
				cs.ObjectMeta.Namespace = ns
				cred := porterv1.Credential{
					Name: "insecureValue",
					Source: porterv1.CredentialSource{
						Secret: name,
					},
				}
				cs.Spec.Credentials = append(cs.Spec.Credentials, cred)
				cs.Spec.Namespace = ns

				Expect(k8sClient.Create(ctx, cs)).Should(Succeed())
				Expect(waitForPorterCS(ctx, cs, "waiting for credential set to apply")).Should(Succeed())
				validateCredSetConditions(cs)

				Log("install porter-test-me bundle with new credset")
				inst := NewTestInstallation("cs-with-secret")
				inst.ObjectMeta.Namespace = ns
				inst.Spec.Namespace = ns
				inst.Spec.CredentialSets = append(inst.Spec.CredentialSets, name)
				inst.Spec.SchemaVersion = "1.0.0"
				Expect(k8sClient.Create(ctx, inst)).Should(Succeed())
				Expect(waitForPorter(ctx, inst, "waiting for porter-test-me to install")).Should(Succeed())
				validateInstallationConditions(inst)

			})
		})
	})
})
var _ = PDescribe("CredSet update", func() {
	Context("when a new CredentialSet resource is updated", func() {
		It("should run porter apply", func() {
			By("creating an agent action", func() {
				Log("update a credential set")
			})
		})
	})
})

var _ = PDescribe("CredSet delete", func() {})

var _ = PDescribe("New Installation with CredentialSet", func() {})

//NewTestCredSet minimal CredentialSet CRD for tests
func NewTestCredSet(csName string) *porterv1.CredentialSet {
	cs := &porterv1.CredentialSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "porter.sh/v1",
			Kind:       "CredentialSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "porter-test-me-",
		},
		Spec: porterv1.CredentialSetSpec{
			//TODO: get schema version from porter version?
			SchemaVersion: schemaVersion,
			Name:          csName,
		},
	}
	return cs
}

func NewTestInstallation(iName string) *porterv1.Installation {
	inst := &porterv1.Installation{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "porter.sh/v1",
			Kind:       "Installation",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "porter-test-me-",
		},
		Spec: porterv1.InstallationSpec{
			SchemaVersion: schemaVersion,
			Name:          iName,
			Bundle: porterv1.OCIReferenceParts{
				Repository: "ghcr.io/bdegeeter/porter-test-me",
				Version:    "0.2.0",
			},
		},
	}
	return inst
}

func waitForPorterCS(ctx context.Context, cs *porterv1.CredentialSet, msg string) error {
	Log("%s: %s/%s", msg, cs.Namespace, cs.Name)
	key := client.ObjectKey{Namespace: cs.Namespace, Name: cs.Name}
	ctx, cancel := context.WithTimeout(ctx, getWaitTimeout())
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			Fail(errors.Wrapf(ctx.Err(), "timeout %s", msg).Error())
		default:
			err := k8sClient.Get(ctx, key, cs)
			if err != nil {
				// There is lag between creating and being able to retrieve, I don't understand why
				if apierrors.IsNotFound(err) {
					time.Sleep(time.Second)
					continue
				}
				return err
			}

			// Check if the latest change has been processed
			if cs.Generation == cs.Status.ObservedGeneration {
				if apimeta.IsStatusConditionTrue(cs.Status.Conditions, string(porterv1.ConditionComplete)) {
					return nil
				}

				if apimeta.IsStatusConditionTrue(cs.Status.Conditions, string(porterv1.ConditionFailed)) {
					// Grab some extra info to help with debugging
					debugFailedCSCreate(ctx, cs)
					return errors.New("porter did not run successfully")
				}
			}

			time.Sleep(time.Second)
			continue
		}
	}
}

func debugFailedCSCreate(ctx context.Context, cs *porterv1.CredentialSet) {
	Log("DEBUG: ----------------------------------------------------")
	actionKey := client.ObjectKey{Name: cs.Status.Action.Name, Namespace: cs.Namespace}
	action := &porterv1.AgentAction{}
	if err := k8sClient.Get(ctx, actionKey, action); err != nil {
		Log(errors.Wrap(err, "could not retrieve the CredentialSet's AgentAction to troubleshoot").Error())
		return
	}

	jobKey := client.ObjectKey{Name: action.Status.Job.Name, Namespace: action.Namespace}
	job := &batchv1.Job{}
	if err := k8sClient.Get(ctx, jobKey, job); err != nil {
		Log(errors.Wrap(err, "could not retrieve the CredentialSet's Job to troubleshoot").Error())
		return
	}

	shx.Command("kubectl", "logs", "-n="+job.Namespace, "job/"+job.Name).
		Env("KUBECONFIG=" + "../../kind.config").RunV()
	Log("DEBUG: ----------------------------------------------------")
}

func validateCredSetConditions(cs *porterv1.CredentialSet) {
	// Checks that all expected conditions are set
	Expect(apimeta.IsStatusConditionTrue(cs.Status.Conditions, string(porterv1.ConditionScheduled)))
	Expect(apimeta.IsStatusConditionTrue(cs.Status.Conditions, string(porterv1.ConditionStarted)))
	Expect(apimeta.IsStatusConditionTrue(cs.Status.Conditions, string(porterv1.ConditionComplete)))
}
