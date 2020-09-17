package kubernetes

import (
	"github.com/pkg/errors"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/resource"
)

// A Client for a Kubernetes cluster.
type Client struct {
	resource.ClientApplicator
	cache.Informers
	kubernetes.Interface

	Config *rest.Config
}

// NewClient returns a client for a Kubernetes cluster.
func NewClient(cc ClientConfig) (Client, error) {
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: cc.KubeConfigPath},
		&clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return Client{}, errors.Wrap(err, "cannot load kubeconfig file")

	}
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		return Client{}, errors.Wrap(err, "cannot register core API types with Kubernetes client")
	}

	ca, err := cache.New(cfg, cache.Options{Scheme: s, Resync: &cc.ResyncInterval})
	if err != nil {
		return Client{}, errors.Wrap(err, "cannot create cache for Kubernetes client")
	}

	// TODO(negz): Use a stop channel that we could actually close?
	stop := make(<-chan struct{})
	go func() {
		err := ca.Start(stop)
		if err != nil {
			log.L.Error("cannot start cache", err)
		}
	}()
	_ = ca.WaitForCacheSync(stop)

	cl, err := client.New(cfg, client.Options{Scheme: s})
	if err != nil {
		return Client{}, errors.Wrap(err, "cannot create Kubernetes client")
	}

	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return Client{}, errors.Wrap(err, "cannot create Kubernetes clientset")
	}

	return Client{
		ClientApplicator: resource.ClientApplicator{
			Client: &client.DelegatingClient{
				Reader:       &client.DelegatingReader{CacheReader: ca, ClientReader: cl},
				Writer:       cl,
				StatusClient: cl,
			},
			Applicator: resource.NewAPIUpdatingApplicator(cl),
		},
		Informers: ca,
		Interface: cs,
		Config:    cfg,
	}, nil
}
