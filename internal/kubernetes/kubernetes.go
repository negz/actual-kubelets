package kubernetes

import (
	"context"
	"io"
	"net/http"

	"github.com/pkg/errors"
	"github.com/virtual-kubelet/node-cli/provider"
	"github.com/virtual-kubelet/virtual-kubelet/errdefs"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	"github.com/virtual-kubelet/virtual-kubelet/node/api"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/deprecated/scheme"
	kcache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/negz/actual-kubelets/internal/pointer"
	"github.com/negz/actual-kubelets/internal/remote"
)

type Provider struct {
	dependencies DependencyFetcher
	remote       Client
	nodeName     string
	cfg          Config
}

func NewProvider(ic provider.InitConfig) (provider.Provider, error) {
	if ic.ConfigPath == "" {
		return nil, errors.New("provider config file is required")
	}

	cfg, err := ParseConfigFile(ic.ConfigPath)
	if err != nil {
		return nil, errors.Wrap(err, "cannot parse provider config")
	}

	local, err := NewClient(cfg.Local)
	if err != nil {
		return nil, errors.Wrap(err, "cannot create client for local (kubelet) API server")
	}

	remote, err := NewClient(cfg.Remote)
	if err != nil {
		return nil, errors.Wrap(err, "cannot create client for remote (backing) API server")
	}

	p := &Provider{
		dependencies: NewAPIDependencyFetcher(local),
		remote:       remote,
		nodeName:     ic.NodeName,
		cfg: Config{
			InitConfig: ic,
			ConfigFile: cfg,
		},
	}

	return p, nil
}

func (p *Provider) ApplyPodDependencies(ctx context.Context, lcl *corev1.Pod) error {
	deps, err := p.dependencies.Fetch(ctx, lcl)
	if err != nil {
		return errors.Wrap(err, "cannot fetch local pod dependencies")
	}

	ns := remote.Namespace(p.nodeName, lcl.GetNamespace())
	if err := p.remote.Apply(ctx, ns); err != nil {
		return errors.Wrap(err, "cannot apply remote pod namespace")
	}

	// NOTE(negz): Multiple pods might share the same dependency within a
	// namespace; i.e. several pods might mount the same ConfigMap. We apply
	// them all for every pod, so applying pod A might also apply dependencies
	// of pod B.
	for _, d := range deps {
		remote.PrepareObject(p.nodeName, d)
		if err := p.remote.Apply(ctx, d); err != nil {
			return errors.Wrap(err, "cannot apply remote pod dependency")
		}
	}

	return nil
}

func (p *Provider) CreatePod(ctx context.Context, lcl *corev1.Pod) error {
	if err := p.ApplyPodDependencies(ctx, lcl); err != nil {
		return errors.Wrap(err, "cannot apply remote pod dependencies")
	}

	rmt := lcl.DeepCopy()
	remote.PreparePod(p.nodeName, rmt, remote.WithEnvVars(p.cfg.Pods.Env...))
	err := p.remote.Create(ctx, rmt)
	return errors.Wrap(err, "cannot apply remote pod")
}

func (p *Provider) UpdatePod(ctx context.Context, lcl *corev1.Pod) error {
	if err := p.ApplyPodDependencies(ctx, lcl); err != nil {
		return errors.Wrap(err, "cannot apply remote pod dependencies")
	}

	rmt := &corev1.Pod{}
	nn := types.NamespacedName{Namespace: remote.NamespaceName(p.nodeName, lcl.GetNamespace()), Name: lcl.GetName()}
	if err := p.remote.Get(ctx, nn, rmt); err != nil {
		return errors.Wrap(err, "cannot get remote pod")
	}

	remote.PreparePodUpdate(p.nodeName, lcl, rmt)
	err := p.remote.Update(ctx, rmt)
	return errors.Wrap(err, "cannot update remote pod")
}

// DeletePod takes a Kubernetes Pod and deletes it from the provider. Once a pod is deleted, the provider is
// expected to call the NotifyPods callback with a terminal pod status where all the containers are in a terminal
// state, as well as the pod. DeletePod may be called multiple times for the same pod.
func (p *Provider) DeletePod(ctx context.Context, lcl *corev1.Pod) error {
	// TODO(negz): Garbage collect empty namespaces and orphaned dependencies?
	// This could potentially be better left to a garbage collection controller
	// in the remote cluster.
	rmt := lcl.DeepCopy()
	// TODO(negz): Figure out why we're seeing the below error.
	// error while updating pod status in kubernetes: Pod \"negztest\" is
	// invalid: metadata.deletionGracePeriodSeconds: Invalid value: 0: field is
	// immutable"
	remote.PreparePod(p.nodeName, rmt)
	err := p.remote.Delete(ctx, rmt)
	if kerrors.IsNotFound(err) {
		return errdefs.AsNotFound(err)
	}
	return errors.Wrap(err, "cannot delete pod")
}

// GetPod retrieves a pod by name from the provider (can be cached).
// The Pod returned is expected to be immutable, and may be accessed
// concurrently outside of the calling goroutine. Therefore it is recommended
// to return a version after DeepCopy.
func (p *Provider) GetPod(ctx context.Context, namespace, name string) (*corev1.Pod, error) {
	rmt := &corev1.Pod{}
	nn := types.NamespacedName{Namespace: remote.NamespaceName(p.nodeName, namespace), Name: name}
	err := p.remote.Get(ctx, nn, rmt)
	if kerrors.IsNotFound(err) {
		return nil, errdefs.AsNotFound(err)
	}
	remote.RecoverPod(rmt)
	return rmt.DeepCopy(), errors.Wrap(err, "cannot get pod")
}

// GetPodStatus retrieves the status of a pod by name from the provider.
// The PodStatus returned is expected to be immutable, and may be accessed
// concurrently outside of the calling goroutine. Therefore it is recommended
// to return a version after DeepCopy.
func (p *Provider) GetPodStatus(ctx context.Context, namespace, name string) (*corev1.PodStatus, error) {
	rmt := &corev1.Pod{}
	nn := types.NamespacedName{Namespace: remote.NamespaceName(p.nodeName, namespace), Name: name}
	err := p.remote.Get(ctx, nn, rmt)
	if kerrors.IsNotFound(err) {
		return nil, errdefs.AsNotFound(err)
	}
	remote.RecoverPod(rmt)
	return rmt.Status.DeepCopy(), errors.Wrap(err, "cannot get pod")
}

// GetPods retrieves a list of all pods running on the provider (can be cached).
// The Pods returned are expected to be immutable, and may be accessed
// concurrently outside of the calling goroutine. Therefore it is recommended
// to return a version after DeepCopy.
func (p *Provider) GetPods(ctx context.Context) ([]*corev1.Pod, error) {
	l := &corev1.PodList{}
	if err := p.remote.List(ctx, l, client.HasLabels([]string{remote.LabelKeyNodeName})); err != nil {
		return nil, errors.Wrap(err, "cannot list pods")
	}

	log.G(ctx).Debug("Listed remote pods", l)

	pods := make([]*corev1.Pod, len(l.Items))
	for i := range l.Items {
		pod := &l.Items[i]
		remote.RecoverPod(pod)
		pods[i] = pod
	}

	return pods, nil
}

// NotifyPods instructs the notifier to call the passed in function when
// the pod status changes. It should be called when a pod's status changes.
//
// The provided pointer to a Pod is guaranteed to be used in a read-only
// fashion. The provided pod's PodStatus should be up to date when
// this function is called.
//
// NotifyPods must not block the caller since it is only used to register the callback.
// The callback passed into `NotifyPods` may block when called.
func (p *Provider) NotifyPods(ctx context.Context, changed func(*corev1.Pod)) {
	i, err := p.remote.GetInformer(ctx, &corev1.Pod{})
	if err != nil {
		log.G(ctx).Error("cannot get informer", err)
		return
	}
	i.AddEventHandler(kcache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if rmt, ok := obj.(*corev1.Pod); ok {
				lcl := rmt.DeepCopy()
				remote.RecoverPod(lcl)
				changed(lcl)
			}
		},
		UpdateFunc: func(_, obj interface{}) {
			if rmt, ok := obj.(*corev1.Pod); ok {
				lcl := rmt.DeepCopy()
				remote.RecoverPod(lcl)
				changed(lcl)
			}
		},
		DeleteFunc: func(obj interface{}) {
			if rmt, ok := obj.(*corev1.Pod); ok {
				lcl := rmt.DeepCopy()
				remote.RecoverPod(lcl)
				changed(lcl)
			}
		},
	})
}

// GetContainerLogs retrieves the logs of a container by name from the provider.
func (p *Provider) GetContainerLogs(ctx context.Context, namespace, podName, containerName string, opts api.ContainerLogOpts) (io.ReadCloser, error) {
	o := &corev1.PodLogOptions{
		Container:    containerName,
		Timestamps:   opts.Timestamps,
		Previous:     opts.Previous,
		Follow:       opts.Follow,
		TailLines:    pointer.Int64OrNil(opts.Tail),
		LimitBytes:   pointer.Int64OrNil(opts.LimitBytes),
		SinceSeconds: pointer.Int64OrNil(opts.SinceSeconds),
		SinceTime: func() *metav1.Time {
			if opts.SinceTime.IsZero() {
				return nil
			}
			return &metav1.Time{Time: opts.SinceTime}
		}(),
	}

	logs := p.remote.CoreV1().Pods(remote.NamespaceName(p.nodeName, namespace)).GetLogs(podName, o)
	r, err := logs.Stream(ctx)
	return r, errors.Wrap(err, "cannot stream container logs")
}

// RunInContainer executes a command in a container in the pod, copying data
// between in/out/err and the container's stdin/stdout/stderr.
func (p *Provider) RunInContainer(ctx context.Context, namespace, podName, containerName string, cmd []string, attach api.AttachIO) error {
	defer func() {
		if attach.Stdout() != nil {
			_ = attach.Stdout().Close()
		}
		if attach.Stderr() != nil {
			_ = attach.Stderr().Close()
		}
	}()

	peo := &corev1.PodExecOptions{
		Container: containerName,
		Command:   cmd,
		Stdin:     attach.Stdin() != nil,
		Stdout:    attach.Stdout() != nil,
		Stderr:    attach.Stderr() != nil,
		TTY:       attach.TTY(),
	}

	req := p.remote.CoreV1().RESTClient().
		Post().
		Namespace(remote.NamespaceName(p.nodeName, namespace)).
		Resource(corev1.ResourcePods.String()).
		Name(podName).
		SubResource("exec").
		Timeout(0).
		VersionedParams(peo, scheme.ParameterCodec)

	e, err := remotecommand.NewSPDYExecutor(p.remote.Config, http.MethodPost, req.URL())
	if err != nil {
		return errors.Wrap(err, "cannot create remote command executor")
	}

	so := remotecommand.StreamOptions{
		Stdin:             attach.Stdin(),
		Stdout:            attach.Stdout(),
		Stderr:            attach.Stderr(),
		Tty:               attach.TTY(),
		TerminalSizeQueue: &tsq{attach},
	}
	attach.Resize()
	return errors.Wrap(e.Stream(so), "cannot create remote command stream")
}

type tsq struct {
	api.AttachIO
}

func (t *tsq) Next() *remotecommand.TerminalSize {
	r := remotecommand.TerminalSize(<-t.Resize())
	return &r
}

// ConfigureNode enables a provider to configure the node object that
// will be used for Kubernetes.
func (p *Provider) ConfigureNode(_ context.Context, n *corev1.Node) {
	n.Status.NodeInfo.OperatingSystem = p.cfg.OperatingSystem

	n.Status.Addresses = []corev1.NodeAddress{
		{Type: corev1.NodeInternalIP, Address: p.cfg.InternalIP},
	}

	n.Status.DaemonEndpoints = corev1.NodeDaemonEndpoints{
		KubeletEndpoint: corev1.DaemonEndpoint{Port: p.cfg.DaemonPort},
	}

	// TODO(negz): Dynamically infer these from the resources the remote cluster
	// has available? This could be difficult to measure given that the remote
	// cluster may autoscale. These should probably just be configurable.
	n.Status.Allocatable = corev1.ResourceList{
		corev1.ResourceCPU:     resource.MustParse("100"),
		corev1.ResourceMemory:  resource.MustParse("1024G"),
		corev1.ResourceStorage: resource.MustParse("100000G"),
		corev1.ResourcePods:    resource.MustParse("1000"),
	}
	// TODO(negz): Would leaving these out impact anything?
	// TODO(negz): Update these messages to indicate that they're fake?
	n.Status.Conditions = []corev1.NodeCondition{
		{
			Type:               corev1.NodeReady,
			Status:             corev1.ConditionTrue,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "KubeletReady",
			Message:            "kubelet is ready.",
		},
		{
			Type:               corev1.NodeMemoryPressure,
			Status:             corev1.ConditionFalse,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "KubeletHasSufficientMemory",
			Message:            "kubelet has sufficient memory available",
		},
		{
			Type:               corev1.NodeDiskPressure,
			Status:             corev1.ConditionFalse,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "KubeletHasNoDiskPressure",
			Message:            "kubelet has no disk pressure",
		},

		{
			Type:               corev1.NodePIDPressure,
			Status:             corev1.ConditionFalse,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "KubeletHasSufficientPID",
			Message:            "kubelet has sufficient PID available",
		},
		{
			Type:               corev1.NodeNetworkUnavailable,
			Status:             corev1.ConditionFalse,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "RouteCreated",
			Message:            "RouteController created a route",
		},
	}

}
