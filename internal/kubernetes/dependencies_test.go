package kubernetes

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/test"

	"github.com/negz/actual-kubelets/internal/remote"
)

func TestFindPodDependencies(t *testing.T) {
	imagePullSecret := "ips"
	volumeSecret := "vs"
	containerSecret := "cs"
	initContainerSecret := "ics"

	cases := map[string]struct {
		reason string
		pod    *corev1.Pod
		want   []Dependency
	}{
		"Pod": {
			reason: "Should find all image pull, volume, and container env var secrets",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					ImagePullSecrets: []corev1.LocalObjectReference{{
						Name: imagePullSecret,
					}},
					Volumes: []corev1.Volume{{
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: volumeSecret,
							},
						},
					}},
					Containers: []corev1.Container{{
						Env: []corev1.EnvVar{{
							ValueFrom: &corev1.EnvVarSource{
								SecretKeyRef: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: containerSecret,
									},
								},
							},
						}},
					}},
					InitContainers: []corev1.Container{{
						Env: []corev1.EnvVar{{
							ValueFrom: &corev1.EnvVarSource{
								SecretKeyRef: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: initContainerSecret,
									},
								},
							},
						}},
					}},
				},
			},
			want: []Dependency{
				{
					Kind:     DependencyKindSecret,
					Name:     imagePullSecret,
					Optional: true,
				},
				{
					Kind: DependencyKindSecret,
					Name: volumeSecret,
				},
				{
					Kind: DependencyKindSecret,
					Name: containerSecret,
				},
				{
					Kind: DependencyKindSecret,
					Name: initContainerSecret,
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := FindPodDependencies(tc.pod)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("\n%s\nFindPodDependencies(...): -want, +got: \n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestFindVolumeDependencies(t *testing.T) {
	requiredSecret := "rs"
	optionalSecret := "os"
	tokenSecret := "secret-token-secret"
	requiredConfigMap := "rcm"
	optionalConfigMap := "ocm"

	cases := map[string]struct {
		reason string
		v      corev1.Volume
		want   []Dependency
	}{
		"RequiredSecret": {
			reason: "Should find a required volume secret",
			v: corev1.Volume{
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: requiredSecret,
					},
				},
			},
			want: []Dependency{
				{
					Kind: DependencyKindSecret,
					Name: requiredSecret,
				},
			},
		},
		"OptionalSecret": {
			reason: "Should find an optional volume secret",
			v: corev1.Volume{
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: optionalSecret,
						Optional: func() *bool {
							t := true
							return &t
						}(),
					},
				},
			},
			want: []Dependency{
				{
					Kind:     DependencyKindSecret,
					Name:     optionalSecret,
					Optional: true,
				},
			},
		},
		"TokenSecret": {
			reason: "Should find a token volume secret",
			v: corev1.Volume{
				Name: tokenSecret,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: tokenSecret,
					},
				},
			},
			want: []Dependency{
				{
					Kind: DependencyKindServiceAccountTokenSecret,
					Name: tokenSecret,
				},
			},
		},
		"RequiredConfigMap": {
			reason: "Should find a required volume config map",
			v: corev1.Volume{
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: requiredConfigMap,
						},
					},
				},
			},
			want: []Dependency{
				{
					Kind: DependencyKindConfigMap,
					Name: requiredConfigMap,
				},
			},
		},
		"OptionalConfigMap": {
			reason: "Should find an optional volume config map",
			v: corev1.Volume{
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: optionalConfigMap,
						},
						Optional: func() *bool {
							t := true
							return &t
						}(),
					},
				},
			},
			want: []Dependency{
				{
					Kind:     DependencyKindConfigMap,
					Name:     optionalConfigMap,
					Optional: true,
				},
			},
		},
		"NotASecretOrConfigMap": {
			reason: "Volumes that aren't backed by a config map or a secret should return no dependencies",
			v:      corev1.Volume{},
			want:   nil,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := FindVolumeDependencies(tc.v)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("\n%s\nFindVolumeDependencies(...): -want, +got: \n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestFindContainerDependencies(t *testing.T) {
	requiredSecret := "rs"
	optionalSecret := "os"
	requiredConfigMap := "rcm"
	optionalConfigMap := "ocm"

	cases := map[string]struct {
		reason string
		c      corev1.Container
		want   []Dependency
	}{
		"EnvFromRequiredSecret": {
			reason: "Should find a required EnvFrom secret",
			c: corev1.Container{
				EnvFrom: []corev1.EnvFromSource{{
					SecretRef: &corev1.SecretEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: requiredSecret,
						},
					},
				}},
			},
			want: []Dependency{
				{
					Kind: DependencyKindSecret,
					Name: requiredSecret,
				},
			},
		},
		"EnvFromOptionalSecret": {
			reason: "Should find an optional EnvFrom secret",
			c: corev1.Container{
				EnvFrom: []corev1.EnvFromSource{{
					SecretRef: &corev1.SecretEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: optionalSecret,
						},
						Optional: func() *bool {
							t := true
							return &t
						}(),
					},
				}},
			},
			want: []Dependency{
				{
					Kind:     DependencyKindSecret,
					Name:     optionalSecret,
					Optional: true,
				},
			},
		},
		"EnvFromRequiredConfigMap": {
			reason: "Should find a required EnvFrom config map",
			c: corev1.Container{
				EnvFrom: []corev1.EnvFromSource{{
					ConfigMapRef: &corev1.ConfigMapEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: requiredConfigMap,
						},
					},
				}},
			},
			want: []Dependency{
				{
					Kind: DependencyKindConfigMap,
					Name: requiredConfigMap,
				},
			},
		},
		"EnvFromOptionalConfigMap": {
			reason: "Should find an optional EnvFrom config map",
			c: corev1.Container{
				EnvFrom: []corev1.EnvFromSource{{
					ConfigMapRef: &corev1.ConfigMapEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: optionalConfigMap,
						},
						Optional: func() *bool {
							t := true
							return &t
						}(),
					},
				}},
			},
			want: []Dependency{
				{
					Kind:     DependencyKindConfigMap,
					Name:     optionalConfigMap,
					Optional: true,
				},
			},
		},
		"EnvRequiredSecret": {
			reason: "Should find a required EnvFrom secret",
			c: corev1.Container{
				Env: []corev1.EnvVar{{
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: requiredSecret,
							},
						},
					},
				}},
			},
			want: []Dependency{
				{
					Kind: DependencyKindSecret,
					Name: requiredSecret,
				},
			},
		},
		"EnvOptionalSecret": {
			reason: "Should find an optional EnvFrom secret",
			c: corev1.Container{
				Env: []corev1.EnvVar{{
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: optionalSecret,
							},
							Optional: func() *bool {
								t := true
								return &t
							}(),
						},
					},
				}},
			},
			want: []Dependency{
				{
					Kind:     DependencyKindSecret,
					Name:     optionalSecret,
					Optional: true,
				},
			},
		},
		"EnvRequiredConfigMap": {
			reason: "Should find a required EnvFrom config map",
			c: corev1.Container{
				Env: []corev1.EnvVar{{
					ValueFrom: &corev1.EnvVarSource{
						ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: requiredConfigMap,
							},
						},
					},
				}},
			},
			want: []Dependency{
				{
					Kind: DependencyKindConfigMap,
					Name: requiredConfigMap,
				},
			},
		},
		"EnvOptionalConfigMap": {
			reason: "Should find an optional EnvFrom config map",
			c: corev1.Container{
				Env: []corev1.EnvVar{{
					ValueFrom: &corev1.EnvVarSource{
						ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: optionalConfigMap,
							},
							Optional: func() *bool {
								t := true
								return &t
							}(),
						},
					},
				}},
			},
			want: []Dependency{
				{
					Kind:     DependencyKindConfigMap,
					Name:     optionalConfigMap,
					Optional: true,
				},
			},
		},
		"NotASecretOrConfigMap": {
			reason: "Env vars that aren't backed by a config map or a secret should return no dependencies",
			c:      corev1.Container{Env: []corev1.EnvVar{{}}},
			want:   []Dependency{},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := FindContainerDependencies(tc.c)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("\n%s\nFindContainerDependencies(...): -want, +got: \n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestAPIDependencyFetcher(t *testing.T) {
	errBoom := errors.New("boom")
	errNotFound := kerrors.NewNotFound(schema.GroupResource{}, "")
	ns := "coolns"
	name := "coolname"

	type args struct {
		ctx context.Context
		pod *corev1.Pod
	}
	type want struct {
		o   []runtime.Object
		err error
	}
	cases := map[string]struct {
		reason string
		c      client.Reader
		o      []APIDependencyFetcherOption
		args   args
		want   want
	}{
		"RequiredDependencyNotFound": {
			reason: "Errors because a required dependency was not found should be returned",
			c: &test.MockClient{
				MockGet: test.NewMockGetFn(errNotFound),
			},
			o: []APIDependencyFetcherOption{
				WithDependencyFinder(DependencyFinderFn(func(*corev1.Pod) []Dependency {
					return []Dependency{{Kind: DependencyKindConfigMap}}
				})),
			},
			args: args{
				pod: &corev1.Pod{},
			},
			want: want{
				err: errors.Wrap(errNotFound, "cannot fetch dependency"),
			},
		},
		"OptionalDependencyNotFound": {
			reason: "Errors because a required dependency was not found should be ignored",
			c: &test.MockClient{
				MockGet: test.NewMockGetFn(errNotFound),
			},
			o: []APIDependencyFetcherOption{
				WithDependencyFinder(DependencyFinderFn(func(*corev1.Pod) []Dependency {
					return []Dependency{{
						Kind:     DependencyKindSecret,
						Optional: true,
					}}
				})),
			},
			args: args{
				pod: &corev1.Pod{},
			},
			want: want{
				o: []runtime.Object{},
			},
		},
		"GetDependencyError": {
			reason: "Unknown errors getting a dependency should be returned",
			c: &test.MockClient{
				MockGet: test.NewMockGetFn(errBoom),
			},
			o: []APIDependencyFetcherOption{
				WithDependencyFinder(DependencyFinderFn(func(*corev1.Pod) []Dependency {
					return []Dependency{{Kind: DependencyKindConfigMap}}
				})),
			},
			args: args{
				pod: &corev1.Pod{},
			},
			want: want{
				err: errors.Wrap(errBoom, "cannot fetch dependency"),
			},
		},
		"GetDependencySuccess": {
			reason: "Fetched dependencies should be returned, and prepared if they're a service account secret",
			c: &test.MockClient{
				MockGet: test.NewMockGetFn(nil, func(obj runtime.Object) error {
					s := obj.(*corev1.Secret)
					s.SetNamespace(ns)
					s.SetName(name)
					return nil
				}),
			},
			o: []APIDependencyFetcherOption{
				WithDependencyFinder(DependencyFinderFn(func(*corev1.Pod) []Dependency {
					return []Dependency{{Kind: DependencyKindServiceAccountTokenSecret}}
				})),
			},
			args: args{
				pod: &corev1.Pod{},
			},
			want: want{
				o: []runtime.Object{&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   ns,
						Name:        name,
						Annotations: map[string]string{},
					},
					Type: remote.SecretTypeReplicatedServiceAccountToken,
				}},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := NewAPIDependencyFetcher(tc.c, tc.o...)
			got, err := f.Fetch(tc.args.ctx, tc.args.pod)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nNewAPIDependencyFetcher(...): -want error, +got error: \n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\nNewAPIDependencyFetcher(...): -want, +got: \n%s\n", tc.reason, diff)
			}
		})
	}
}
