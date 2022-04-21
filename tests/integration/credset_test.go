//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/carolynvs/magex/shx"
	. "github.com/onsi/ginkgo"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cl "k8s.io/client-go/kubernetes"
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

				csSecret := NewTestSecret(name, testSecret)
				csSecret.ObjectMeta.Namespace = ns
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
				// patchCredSet := func(cs *porterv1.CredentialSet) {
				// 	controllers.PatchObjectWithRetry(ctx, logr.Discard(), k8sClient, k8sClient.Patch, cs, func() client.Object {
				// 		return &porterv1.CredentialSet{}
				// 	})
				// }
				Log("install porter-test-me bundle with new credset")
				inst := NewTestInstallation("cs-with-secret")
				inst.ObjectMeta.Namespace = ns
				inst.Spec.Namespace = ns
				inst.Spec.CredentialSets = append(inst.Spec.CredentialSets, name)
				inst.Spec.SchemaVersion = "1.0.0"
				Expect(k8sClient.Create(ctx, inst)).Should(Succeed())
				Expect(waitForPorter(ctx, inst, "waiting for porter-test-me to install")).Should(Succeed())
				validateInstallationConditions(inst)
				action := &porterv1.AgentAction{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Name:      "agentaction-show-outputs",
					},
					Spec: porterv1.AgentActionSpec{
						//Args: []string{"installations", "list", "--all-namespaces", "-o", "json"},
						Args: []string{"installation", "outputs", "list", "-n", ns, "-i", inst.Spec.Name, "-o", "json"},
					},
				}
				Expect(k8sClient.Create(ctx, action)).Should(Succeed())
				Expect(waitForAgentAction(ctx, action, "waiting for agent action to run")).Should(Succeed())
				getAgentActionJobOutput(ctx, action.Name, ns)
				// newCred := porterv1.Credential{
				// 	Name: "newValue",
				// 	Source: porterv1.CredentialSource{
				// 		Secret: name,
				// 	},
				// }
				// cs.Spec.Credentials = []porterv1.Credential{newCred}
				// patchCredSet(cs)
				// Expect(waitForPorterCS(ctx, cs, "waiting for credential set to update"))

				// Log("install porter-test-me bundle with new credset")
				// newInst := NewTestInstallation("updated-cs")
				// newInst.ObjectMeta.Namespace = ns
				// newInst.Spec.Namespace = ns
				// newInst.Spec.CredentialSets = append(newInst.Spec.CredentialSets, name)
				// newInst.Spec.SchemaVersion = "1.0.0"
				// Expect(k8sClient.Create(ctx, newInst)).Should(Succeed())
				// Expect(waitForPorter(ctx, newInst, "waiting for porter-test-me to install")).Should(Succeed())
				// validateInstallationConditions(newInst)
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

var _ = Describe("CredSet delete", func() {
	Context("when an existing CredentialSet is delete", func() {
		It("should run porter credentials delete", func() {
			By("creating an agent action", func() {
				ctx := context.Background()
				ns := createTestNamespace(ctx)
				name := "test-cs-" + ns
				testSecret := "secret-value"

				Log(fmt.Sprintf("create k8s secret '%s' for credset", name))
				csSecret := NewTestSecret(name, testSecret)
				csSecret.ObjectMeta.Namespace = ns
				Expect(k8sClient.Create(ctx, csSecret)).Should(Succeed())

				Log(fmt.Sprintf("create credential set '%s' for credset", name))
				cs := NewTestCredSet(name)
				cs.ObjectMeta.Namespace = ns
				cred := porterv1.Credential{
					Name: "test-credential",
					Source: porterv1.CredentialSource{
						Secret: name,
					},
				}
				cs.Spec.Credentials = append(cs.Spec.Credentials, cred)
				cs.Spec.Namespace = ns

				Expect(k8sClient.Create(ctx, cs)).Should(Succeed())
				Expect(waitForPorterCS(ctx, cs, "waiting for credential set to apply")).Should(Succeed())
				validateCredSetConditions(cs)
				createCheck := &porterv1.AgentAction{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Name:      "create-check-credentials-list",
					},
					Spec: porterv1.AgentActionSpec{
						Args: []string{"credentials", "list", "-n", ns, "-o", "json"},
					},
				}
				Expect(k8sClient.Create(ctx, createCheck)).Should(Succeed())
				Expect(waitForAgentAction(ctx, createCheck, "waiting for credentials list agent action to run")).Should(Succeed())
				aaOut, err := getAgentActionJobOutput(ctx, createCheck.Name, ns)
				Expect(err).Error().ShouldNot(HaveOccurred())

				jsonOut := getAgentActionCmdOut(createCheck, aaOut)
				firstName := gjson.Get(jsonOut, "0.name").String()
				numCreds := gjson.Get(jsonOut, "#").Int()
				firstCredName := gjson.Get(jsonOut, "0.credentials.0.name").String()
				Expect(int64(1)).To(Equal(numCreds))
				Expect(name).To(Equal(firstName))
				Expect("test-credential").To(Equal(firstCredName))

				Log("delete a credential set")
				Expect(k8sClient.Delete(ctx, cs)).Should(Succeed())
				Expect(waitForPorterCS(ctx, cs, "waiting for credential set to delete")).Should(Succeed())

				Log("verify it's gone")
				delCheck := &porterv1.AgentAction{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ns,
						Name:      "delete-check-credentials-list",
					},
					Spec: porterv1.AgentActionSpec{
						Args: []string{"credentials", "list", "-n", ns, "-o", "json"},
					},
				}
				Expect(k8sClient.Create(ctx, delCheck)).Should(Succeed())
				Expect(waitForAgentAction(ctx, delCheck, "waiting for credentials list agent action to run")).Should(Succeed())
				daOut, err := getAgentActionJobOutput(ctx, delCheck.Name, ns)
				Expect(err).Error().ShouldNot(HaveOccurred())
				delJsonOut := getAgentActionCmdOut(createCheck, daOut)
				delNumCreds := gjson.Get(delJsonOut, "#").Int()
				Expect(int64(0)).To(Equal(delNumCreds))

			})
		})
	})
})

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

func NewTestSecret(name, value string) *corev1.Secret {
	csSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Type:       corev1.SecretTypeOpaque,
		StringData: map[string]string{"value": value},
	}
	return csSecret
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

func waitForAgentAction(ctx context.Context, aa *porterv1.AgentAction, msg string) error {
	Log("%s: %s/%s", msg, aa.Namespace, aa.Name)
	key := client.ObjectKey{Namespace: aa.Namespace, Name: aa.Name}
	ctx, cancel := context.WithTimeout(ctx, getWaitTimeout())
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			Fail(errors.Wrapf(ctx.Err(), "timeout %s", msg).Error())
		default:
			err := k8sClient.Get(ctx, key, aa)
			if err != nil {
				// There is lag between creating and being able to retrieve, I don't understand why
				if apierrors.IsNotFound(err) {
					time.Sleep(time.Second)
					continue
				}
				return err
			}

			// Check if the latest change has been processed
			if aa.Generation == aa.Status.ObservedGeneration {
				if apimeta.IsStatusConditionTrue(aa.Status.Conditions, string(porterv1.ConditionComplete)) {
					return nil
				}

				if apimeta.IsStatusConditionTrue(aa.Status.Conditions, string(porterv1.ConditionFailed)) {
					// Grab some extra info to help with debugging
					//debugFailedCSCreate(ctx, aa)
					return errors.New("porter did not run successfully")
				}
			}

			time.Sleep(time.Second)
			continue
		}
	}

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

func getAgentActionJobOutput(ctx context.Context, agentActionName string, namespace string) (string, error) {
	actionKey := client.ObjectKey{Name: agentActionName, Namespace: namespace}
	action := &porterv1.AgentAction{}
	if err := k8sClient.Get(ctx, actionKey, action); err != nil {
		Log(errors.Wrap(err, "could not retrieve the CredentialSet's AgentAction to troubleshoot").Error())
		return "", err
	}
	jobKey := client.ObjectKey{Name: action.Status.Job.Name, Namespace: action.Namespace}
	job := &batchv1.Job{}
	if err := k8sClient.Get(ctx, jobKey, job); err != nil {
		Log(errors.Wrap(err, "could not retrieve the Job to troubleshoot").Error())
		return "", err
	}
	c, err := cl.NewForConfig(testEnv.Config)
	if err != nil {
		Log(err.Error())
		return "", err
	}
	selector, err := metav1.LabelSelectorAsSelector(job.Spec.Selector)
	if err != nil {
		Log(errors.Wrap(err, "could not retrieve label selector for job").Error())
		return "", err
	}
	pods, err := c.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		Log(errors.Wrap(err, "could not retrive pod list for job").Error())
		return "", err
	}
	if len(pods.Items) != 1 {
		Log(fmt.Sprintf("too many pods associated to agent action job. Expected 1 found %v", len(pods.Items)))
		return "", err
	}
	podLogOpts := corev1.PodLogOptions{}
	req := c.CoreV1().Pods(namespace).GetLogs(pods.Items[0].Name, &podLogOpts)
	podLogs, err := req.Stream(ctx)
	if err != nil {
		Log(errors.Wrap(err, "could not stream pod logs").Error())
		return "", err
	}
	defer podLogs.Close()
	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		Log(errors.Wrap(err, "could not copy pod logs to byte buffer").Error())
		return "", err
	}
	outputLog := buf.String()
	return outputLog, nil
}

func getAgentActionCmdOut(action *porterv1.AgentAction, aaOut string) string {
	return strings.SplitAfterN(strings.Replace(aaOut, "\n", "", -1), strings.Join(action.Spec.Args, " "), 2)[1]
}
