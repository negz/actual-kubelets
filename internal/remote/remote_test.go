package remote

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

const (
	nodeName   = "coolnode"
	nsName     = "coolns"
	nsNameHash = "-ae69504377748847"
	name       = "coolpod"
)

func TestPrepareObject(t *testing.T) {

	type args struct {
		nodeName string
		o        runtime.Object
	}
	cases := map[string]struct {
		reason string
		args   args
		want   runtime.Object
	}{
		"Nil": {
			reason: "Should be a no-op if the runtime.Object contains a type that does not satisfy metav1.Object",
			args: args{
				nodeName: nodeName,
				o:        nil,
			},
			want: nil,
		},
		"Pod": {
			reason: "PrepareObjectMeta should be called if the runtime.Object satisfies metav1.Object",
			args: args{
				nodeName: nodeName,
				o: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: nsName,
						Name:      name,
					},
				},
			},
			want: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: nodeName + nsNameHash,
					Name:      name,
					Labels: map[string]string{
						LabelKeyNamespace: nsName,
						LabelKeyNodeName:  nodeName,
					},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			PrepareObject(tc.args.nodeName, tc.args.o)
			if diff := cmp.Diff(tc.want, tc.args.o); diff != "" {
				t.Errorf("\n%s\nPrepareObject(...): -want, +got: \n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestPrepareObjectMeta(t *testing.T) {
	type args struct {
		nodeName string
		o        metav1.Object
	}
	cases := map[string]struct {
		reason string
		args   args
		want   metav1.Object
	}{
		"Pod": {
			reason: "Labels should be added, and identifying data should be cleared or (in the case of the namespace) mutated",
			args: args{
				nodeName: nodeName,
				o: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       nsName,
						Name:            name,
						UID:             types.UID("no-you-id"),
						SelfLink:        "https://example.org/api/coolns/coolpod",
						ResourceVersion: "42",
						Labels:          map[string]string{"cool": "very"},
					},
				},
			},
			want: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: nodeName + nsNameHash,
					Name:      name,
					Labels: map[string]string{
						"cool":            "very",
						LabelKeyNamespace: nsName,
						LabelKeyNodeName:  nodeName,
					},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			PrepareObjectMeta(tc.args.nodeName, tc.args.o)
			if diff := cmp.Diff(tc.want, tc.args.o); diff != "" {
				t.Errorf("\n%s\nPrepareObjectMeta(...): -want, +got: \n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestRecoverObjectMeta(t *testing.T) {
	cases := map[string]struct {
		reason string
		o      metav1.Object
		want   metav1.Object
	}{
		"Pod": {
			reason: "Identifying data should be recovered from labels",
			o: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:       nodeName + nsNameHash,
					Name:            name,
					UID:             types.UID("no-you-id"),
					SelfLink:        "https://example.org/api/coolns/coolpod",
					ResourceVersion: "42",
					Labels: map[string]string{
						"cool":            "very",
						LabelKeyNamespace: nsName,
						LabelKeyNodeName:  nodeName,
					},
				},
			},
			want: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: nsName,
					Name:      name,
					Labels:    map[string]string{"cool": "very"},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			RecoverObjectMeta(tc.o)
			if diff := cmp.Diff(tc.want, tc.o); diff != "" {
				t.Errorf("\n%s\nRecoverObjectMeta(...): -want, +got: \n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestPreparePod(t *testing.T) {
	svcAcctName := "acct"
	envVar := "var"
	envVal := "val"

	type args struct {
		nodeName string
		pod      *corev1.Pod
		o        []PreparePodOption
	}
	cases := map[string]struct {
		reason string
		args   args
		want   *corev1.Pod
	}{
		"Pod": {
			reason: "ObjectMeta should be prepared, service accounts should be cleared, and env vars should be set",
			args: args{
				nodeName: nodeName,
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: nsName,
						Name:      name,
					},
					Spec: corev1.PodSpec{
						ServiceAccountName:       svcAcctName,
						DeprecatedServiceAccount: svcAcctName,
						AutomountServiceAccountToken: func() *bool {
							t := true
							return &t
						}(),
						Containers: []corev1.Container{{
							Env: []corev1.EnvVar{
								{
									Name:  envVar,
									Value: "wat",
								},
								{
									Name:  "other",
									Value: envVal,
								},
							},
						}},
					},
				},
				o: []PreparePodOption{WithEnvVars(corev1.EnvVar{
					Name:  envVar,
					Value: envVal,
				})},
			},
			want: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: nodeName + nsNameHash,
					Name:      name,
					Labels: map[string]string{
						LabelKeyNamespace: nsName,
						LabelKeyNodeName:  nodeName,
					},
					Annotations: map[string]string{
						AnnotationKeyServiceAccountName: svcAcctName,
					},
				},
				Spec: corev1.PodSpec{
					AutomountServiceAccountToken: func() *bool {
						f := false
						return &f
					}(),
					Containers: []corev1.Container{{
						Env: []corev1.EnvVar{
							{
								Name:  "other",
								Value: envVal,
							},
							{
								Name:  envVar,
								Value: envVal,
							},
						},
					}},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			PreparePod(tc.args.nodeName, tc.args.pod, tc.args.o...)
			if diff := cmp.Diff(tc.want, tc.args.pod); diff != "" {
				t.Errorf("\n%s\nPreparePod(...): -want, +got: \n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestPreparePodUpdate(t *testing.T) {
	labels := map[string]string{"l": "t"}
	annos := map[string]string{"a": "t"}

	type args struct {
		nodeName string
		local    *corev1.Pod
		remote   *corev1.Pod
	}
	cases := map[string]struct {
		reason string
		args   args
		want   *corev1.Pod // The remote pod.
	}{
		"Pod": {
			reason: "The remote pod's labels and annotations should be updated to match the local pod's",
			args: args{
				nodeName: nodeName,
				local: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   nsName,
						Name:        name,
						Labels:      labels,
						Annotations: annos,
					},
				},
				remote: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   nodeName + nsNameHash,
						Name:        name,
						Labels:      labels,
						Annotations: annos,
					},
				},
			},
			want: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: nodeName + nsNameHash,
					Name:      name,
					Labels: func() map[string]string {
						l := map[string]string{
							LabelKeyNamespace: nsName,
							LabelKeyNodeName:  nodeName,
						}
						for k, v := range labels {
							l[k] = v
						}
						return l
					}(),
					Annotations: annos,
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			PreparePodUpdate(tc.args.nodeName, tc.args.local, tc.args.remote)
			if diff := cmp.Diff(tc.want, tc.args.remote); diff != "" {
				t.Errorf("\n%s\nPreparePod(...): -want, +got: \n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestRecoverPod(t *testing.T) {
	cases := map[string]struct {
		reason string
		pod    *corev1.Pod
		want   *corev1.Pod
	}{
		"Pod": {
			reason: "Identifying data should be recovered from labels",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: nodeName + nsNameHash,
					Name:      name,
					Labels: map[string]string{
						"cool":            "very",
						LabelKeyNamespace: nsName,
						LabelKeyNodeName:  nodeName,
					},
				},
				Spec: corev1.PodSpec{
					NodeName:                  "bob",
					NodeSelector:              map[string]string{"cool": "extremely"},
					Affinity:                  &corev1.Affinity{},
					TopologySpreadConstraints: []corev1.TopologySpreadConstraint{{}},
				},
			},
			want: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: nsName,
					Name:      name,
					Labels:    map[string]string{"cool": "very"},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			RecoverPod(tc.pod)
			if diff := cmp.Diff(tc.want, tc.pod); diff != "" {
				t.Errorf("\n%s\nRecoverPod(...): -want, +got: \n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestNamespace(t *testing.T) {
	nodeName := "coolnode"
	localName := "coolns"
	localNameHash := "-ae69504377748847"

	type args struct {
		nodeName       string
		localNamespace string
	}
	cases := map[string]struct {
		reason string
		args   args
		want   *corev1.Namespace
	}{
		"Namespace": {
			reason: "The local namespace name should be transformed, but persisted as a label",
			args: args{
				nodeName:       nodeName,
				localNamespace: localName,
			},
			want: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName + localNameHash,
					Labels: map[string]string{
						LabelKeyNodeName:  nodeName,
						LabelKeyNamespace: localName,
					},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := Namespace(tc.args.nodeName, tc.args.localNamespace)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("\n%s\nNamespace(...): -want, +got: \n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestIsTokenVolume(t *testing.T) {
	cases := map[string]struct {
		reason string
		v      corev1.Volume
		want   bool
	}{
		"NotASecretVolume": {
			reason: "Token volumes are always backed by a secret.",
			v:      corev1.Volume{},
			want:   false,
		},
		"VolumeNameDoesNotContainToken": {
			reason: "Token volumes always have the substring '-token-' in their name.",
			v: corev1.Volume{
				VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{}},
			},
			want: false,
		},
		"SecretNameDoesNotContainToken": {
			reason: "Token volumes always have the substring '-token-' in their secret name.",
			v: corev1.Volume{
				Name:         "cool-token-randm",
				VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{}},
			},
			want: false,
		},
		"ProbablyATokenVolume": {
			reason: "A volume backed by a secret with '-token-' in its name and token name is very likely to be a token volume.",
			v: corev1.Volume{
				Name: "cool-token-randm",
				VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{
					SecretName: "cool-token-randm",
				}},
			},
			want: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := IsTokenVolume(tc.v)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("\n%s\nIsTokenVolume(...): -want, +got: \n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestPrepareServiceAccountTokenSecret(t *testing.T) {
	name := "acct"
	uid := "no-you-id"

	cases := map[string]struct {
		reason string
		s      *corev1.Secret
		want   *corev1.Secret
	}{
		"Secret": {
			reason: "A service account secret's type should be updated and its annotations stripped",
			s: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						corev1.ServiceAccountNameKey: name,
						corev1.ServiceAccountUIDKey:  uid,
						"cool":                       "true",
					},
				},
				Type: corev1.SecretTypeServiceAccountToken,
			},
			want: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationKeyServiceAccountName: name,
						"cool":                          "true",
					},
				},
				Type: SecretTypeReplicatedServiceAccountToken,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			PrepareServiceAccountTokenSecret(tc.s)
			if diff := cmp.Diff(tc.want, tc.s); diff != "" {
				t.Errorf("\n%s\nPrepareServiceAccountTokenSecret(...): -want, +got: \n%s\n", tc.reason, diff)
			}
		})
	}
}
