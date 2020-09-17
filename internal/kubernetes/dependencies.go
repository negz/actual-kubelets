package kubernetes

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/negz/actual-kubelets/internal/pointer"
	"github.com/negz/actual-kubelets/internal/remote"
)

func IsTokenVolume(v corev1.Volume) bool {
	if v.Secret == nil {
		return false
	}
	if !strings.Contains(v.Name, "-token-") {
		return false
	}
	return strings.Contains(v.Secret.SecretName, "-token-")
}

type DependencyKind int

const (
	DependencyKindConfigMap DependencyKind = iota
	DependencyKindSecret
	DependencyKindServiceAccountTokenSecret
)

type Dependency struct {
	Kind     DependencyKind
	Name     string
	Optional bool
}

func FindPodDependencies(pod *corev1.Pod) []Dependency {
	deps := make([]Dependency, 0, len(pod.Spec.ImagePullSecrets))

	for _, ref := range pod.Spec.ImagePullSecrets {
		deps = append(deps, Dependency{
			Kind: DependencyKindSecret,
			Name: ref.Name,
			// Image pull secrets are optional; a pod will start without one as
			// long as it can pull its image.
			Optional: true,
		})
	}

	for _, v := range pod.Spec.Volumes {
		deps = append(deps, FindVolumeDependencies(v)...)
	}

	cs := make([]corev1.Container, 0, len(pod.Spec.Containers)+len(pod.Spec.InitContainers))
	cs = append(cs, pod.Spec.Containers...)
	cs = append(cs, pod.Spec.InitContainers...)

	for _, c := range cs {
		deps = append(deps, FindContainerDependencies(c)...)
	}

	return deps
}

func FindVolumeDependencies(v corev1.Volume) []Dependency {
	switch {
	case v.VolumeSource.ConfigMap != nil:
		return []Dependency{{
			Kind:     DependencyKindConfigMap,
			Name:     v.VolumeSource.ConfigMap.Name,
			Optional: pointer.DerefBoolOr(v.VolumeSource.ConfigMap.Optional, false),
		}}
	case v.VolumeSource.Secret != nil:
		k := DependencyKindSecret
		// Service account token secrets get special handling.
		if IsTokenVolume(v) {
			k = DependencyKindServiceAccountTokenSecret
		}

		return []Dependency{{
			Kind:     k,
			Name:     v.VolumeSource.Secret.SecretName,
			Optional: pointer.DerefBoolOr(v.VolumeSource.Secret.Optional, false),
		}}
	}

	return nil
}

func FindContainerDependencies(c corev1.Container) []Dependency {
	deps := make([]Dependency, 0)

	for _, v := range c.EnvFrom {
		switch {
		case v.ConfigMapRef != nil:
			deps = append(deps, Dependency{
				Kind:     DependencyKindConfigMap,
				Name:     v.ConfigMapRef.Name,
				Optional: pointer.DerefBoolOr(v.ConfigMapRef.Optional, false),
			})
		case v.SecretRef != nil:
			deps = append(deps, Dependency{
				Kind:     DependencyKindSecret,
				Name:     v.SecretRef.Name,
				Optional: pointer.DerefBoolOr(v.SecretRef.Optional, false),
			})
		}
		for _, v := range c.Env {
			switch {
			case v.ValueFrom == nil:
				continue
			case v.ValueFrom.ConfigMapKeyRef != nil:
				deps = append(deps, Dependency{
					Kind:     DependencyKindConfigMap,
					Name:     v.ValueFrom.ConfigMapKeyRef.Name,
					Optional: pointer.DerefBoolOr(v.ValueFrom.ConfigMapKeyRef.Optional, false),
				})
			case v.ValueFrom.SecretKeyRef != nil:
				deps = append(deps, Dependency{
					Kind:     DependencyKindSecret,
					Name:     v.ValueFrom.SecretKeyRef.Name,
					Optional: pointer.DerefBoolOr(v.ValueFrom.SecretKeyRef.Optional, false),
				})
			}
		}
	}

	return deps
}

type DependencyFetcher interface {
	Fetch(ctx context.Context, pod *corev1.Pod) ([]runtime.Object, error)
}

type APIDependencyFetcher struct {
	client client.Reader
	find   func(*corev1.Pod) []Dependency
}

func NewAPIDependencyFetcher(c client.Client) *APIDependencyFetcher {
	return &APIDependencyFetcher{client: c, find: FindPodDependencies}
}

func (f *APIDependencyFetcher) Fetch(ctx context.Context, pod *corev1.Pod) ([]runtime.Object, error) {
	d := f.find(pod)

	fetched := make([]runtime.Object, 0, len(d))

	for _, dp := range d {
		var obj runtime.Object

		nn := types.NamespacedName{Namespace: pod.GetNamespace(), Name: dp.Name}
		switch dp.Kind {
		case DependencyKindSecret, DependencyKindServiceAccountTokenSecret:
			obj = &corev1.Secret{}
		case DependencyKindConfigMap:
			obj = &corev1.ConfigMap{}
		}

		if err := f.client.Get(ctx, nn, obj); err != nil {
			if kerrors.IsNotFound(err) && dp.Optional {
				continue
			}
			return nil, errors.Wrap(err, "cannot fetch dependency")
		}

		if dp.Kind == DependencyKindServiceAccountTokenSecret {
			remote.PrepareServiceAccountTokenSecret(obj.(*corev1.Secret))
		}

		fetched = append(fetched, obj)
	}

	return fetched, nil
}
