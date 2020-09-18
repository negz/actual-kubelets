package remote

import (
	"fmt"
	"hash/fnv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/negz/actual-kubelets/internal/pointer"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
)

const (
	// LabelKeyNodeName represents the node-name of the Virtual Kubelet that
	// created an object in a remote cluster. Each Virtual Kubelet should have a
	// uninque node-name within its remote cluster.
	LabelKeyNodeName = "actual.vk/node-name"

	// LabelKeyNamespace represents the 'local' namespace of an object created
	// in a remote cluster.
	LabelKeyNamespace = "actual.vk/namespace"

	// AnnotationKeyServiceAccountName is added to replicated service account
	// token secrets to indicate the service account they are associated with.
	AnnotationKeyServiceAccountName = "actual.vk/replicated-service-account.name"

	// SecretTypeReplicatedServiceAccountToken indicates that a secret is a
	// service account token replicated by the Virtual Kubelet so that a remote
	// pod may connect to the local API.
	SecretTypeReplicatedServiceAccountToken corev1.SecretType = "actual.vk/replicated-service-account-token"
)

// PrepareObject prepares the supplied object for submission to a remote
// cluster by running PrepareObjectMeta on it, if possible.
func PrepareObject(nodeName string, o runtime.Object) {
	om, ok := o.(metav1.Object)
	if !ok {
		return
	}
	PrepareObjectMeta(nodeName, om)
}

// PrepareObjectMeta prepares the supplied object for submission to a remote
// cluster by adding labels that relate it back to its identity on the local
// cluster, and removing any metadata (UIDs, etc) that would conflict with the
// remote cluster.
func PrepareObjectMeta(nodeName string, o metav1.Object) {
	// Provide a hint relating the remote resource back to the local resource.
	meta.AddLabels(o, map[string]string{
		LabelKeyNodeName:  nodeName,
		LabelKeyNamespace: o.GetNamespace(),
	})

	// Clear out metadata that should not propagate to the remote cluster.
	o.SetUID(types.UID(""))
	o.SetResourceVersion("")
	o.SetSelfLink("")
	o.SetOwnerReferences(nil)

	// Use a deterministic remote namespace that is scoped to the local
	// namespace, and likely to be RFC-1123 compatible.
	o.SetNamespace(NamespaceName(nodeName, o.GetNamespace()))
}

// RecoverObjectMeta recovers a remote object for representation in the local
// cluster by recovering data from labels that relate it back to its identity on
// the local cluster, stripping those labels, and removing any metadata (UIDs,
// etc) that would conflict with the local cluster.
func RecoverObjectMeta(o metav1.Object) {
	// Clear out our hint labels, and restore our local namespace.
	l := map[string]string{}
	for k, v := range o.GetLabels() {
		if k == LabelKeyNodeName {
			continue
		}
		if k == LabelKeyNamespace {
			o.SetNamespace(v)
			continue
		}
		l[k] = v
	}
	o.SetLabels(l)

	// Clear out metadata that should not propagate to the local cluster.
	o.SetUID(types.UID(""))
	o.SetResourceVersion("")
	o.SetSelfLink("")
	o.SetOwnerReferences(nil)
}

type ppo struct {
	env []corev1.EnvVar
}

// A PreparePodOption influences how a pod is prepared for the remote cluster.
type PreparePodOption func(*ppo)

// WithEnvVars injects the supplied environment variables into all containers of
// the pod. Any existing environment variables of the same name are replaced.
func WithEnvVars(v ...corev1.EnvVar) PreparePodOption {
	return func(o *ppo) {
		o.env = v
	}
}

// PreparePod prepares the supplied pod for submission to a remote cluster by
// running PrepareObjectMeta on it, and removing any scheduling constraints that
// might influence the remote cluster.
func PreparePod(nodeName string, pod *corev1.Pod, o ...PreparePodOption) {
	ppo := &ppo{}
	for _, fn := range o {
		fn(ppo)
	}

	PrepareObjectMeta(nodeName, pod)

	// Disable service account. We replicate and mount any service account token
	// that was created on the local cluster. We don't want the remote cluster's
	// service account controller to override this.
	pod.Spec.AutomountServiceAccountToken = pointer.Bool(false)
	if n := pod.Spec.DeprecatedServiceAccount; n != "" {
		pod.Spec.DeprecatedServiceAccount = ""
		meta.AddAnnotations(pod, map[string]string{AnnotationKeyServiceAccountName: n})
	}
	if n := pod.Spec.ServiceAccountName; n != "" {
		pod.Spec.ServiceAccountName = ""
		meta.AddAnnotations(pod, map[string]string{AnnotationKeyServiceAccountName: n})
	}

	setEnvVars(pod.Spec.InitContainers, ppo.env...)
	setEnvVars(pod.Spec.Containers, ppo.env...)

	// Remove spec fields that could influence scheduling on the remote cluster.
	pod.Spec.NodeName = ""
	pod.Spec.NodeSelector = nil
	pod.Spec.Affinity = nil
	pod.Spec.TopologySpreadConstraints = nil
}

func setEnvVars(cs []corev1.Container, v ...corev1.EnvVar) {
	if len(cs) == 0 || len(v) == 0 {
		return
	}

	set := map[string]corev1.EnvVar{}
	for _, vr := range v {
		set[strings.ToUpper(vr.Name)] = vr
	}

	for i := range cs {
		ev := make([]corev1.EnvVar, 0, len(cs[i].Env))

		// Filter out existing environment variables that we intend to set.
		for j := range cs[i].Env {
			if _, ok := set[strings.ToUpper(cs[i].Env[j].Name)]; ok {
				continue
			}
			ev = append(ev, cs[i].Env[j])
		}

		// Set the supplied env vars.
		ev = append(ev, v...)
		cs[i].Env = ev
	}
}

// PreparePodUpdate prepares the supplied remote pod to be updated in accordance
// with the supplied local pod.
func PreparePodUpdate(nodeName string, local, remote *corev1.Pod) {
	// TODO(negz): Allow updating container images.
	PrepareObjectMeta(nodeName, local)
	remote.SetLabels(local.GetLabels())
	remote.SetAnnotations(local.GetAnnotations())
}

// RecoverPod recovers the supplied pod for representation in the local cluster
// by running RecoverObjectMeta on it, and removing any scheduling constraints
// that might influence the local cluster.
func RecoverPod(pod *corev1.Pod) {
	RecoverObjectMeta(pod)

	pod.Spec.NodeName = ""
	pod.Spec.NodeSelector = nil
	pod.Spec.Affinity = nil
	pod.Spec.TopologySpreadConstraints = nil
}

// Namespace returns a remote namespace corresponding to the supplied local
// namespace. It assumes a many-to-one local-to-remote relationship, allowing
// many (local) virtual kubelets to create pods (and their dependencies) in one
// remote cluster. Each remote namespace corresponds to a single local namespace
// as long as all Kubelet node names are unique within the remote cluster.
func Namespace(nodeName, localNamespace string) *corev1.Namespace {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: NamespaceName(nodeName, localNamespace),
			Labels: map[string]string{
				LabelKeyNodeName:  nodeName,
				LabelKeyNamespace: localNamespace,
			},
		},
	}

	return ns
}

// NamespaceName returns a remote namespace name. Remote namespaces are named
// such that each remote namespace corresponds to a single local namespace as
// long as all Kubelet node names are unique within the remote cluster.
func NamespaceName(nodeName, localNamespace string) string {
	h := fnv.New64()
	_, _ = h.Write([]byte(localNamespace)) // Writing to a hash never errors.
	return fmt.Sprintf("%s-%x", nodeName, h.Sum64())
}

// IsTokenVolume returns true if the supplied volume is (very likely to be) a
// service account token volume.
func IsTokenVolume(v corev1.Volume) bool {
	if v.Secret == nil {
		return false
	}
	if !strings.Contains(v.Name, "-token-") {
		return false
	}
	return strings.Contains(v.Secret.SecretName, "-token-")
}

// PrepareServiceAccountTokenSecret updates the type and annotations of a
// service account secret. This ensures the remote cluster's service account
// controller does not attempt to garbage collect or otherwise interfere with
// the secret.
func PrepareServiceAccountTokenSecret(s *corev1.Secret) {
	a := map[string]string{}
	for k, v := range s.GetAnnotations() {
		if k == corev1.ServiceAccountUIDKey {
			continue
		}
		if k == corev1.ServiceAccountNameKey {
			// Use our own.
			a[AnnotationKeyServiceAccountName] = v
			continue
		}
		a[k] = v
	}
	s.SetAnnotations(a)
	s.Type = SecretTypeReplicatedServiceAccountToken
}
