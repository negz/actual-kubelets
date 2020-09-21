package kubernetes

import (
	"io/ioutil"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
	"github.com/virtual-kubelet/node-cli/provider"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// A Config contains the configuration that a provider needs - both that
// provided by the InitConfig and that read from the config file therein.
type Config struct {
	provider.InitConfig
	ConfigFile
}

// A ClientConfig is used to configure a Kubernetes client.
type ClientConfig struct {
	// KubeConfigPath is an optional path to a kubeconfig file that will be used
	// to configure a client. Clients attempt in-cluster config if no kubeconfig
	// is provided.
	KubeConfigPath string `toml:"kubeconfig_path"`

	// ResyncInterval specifies how frequently the client's cache resync its
	// contents with the API server. The cache watches the API server; the
	// resync guards against missed updates.
	ResyncInterval time.Duration `toml:"resync_interval"`
}

// The PodsConfig is used to influence how pods are prepared for submission to
// the remote API server.
type PodsConfig struct {
	// Env vars that should be added to (or overridden in) all pod containers.
	Env []corev1.EnvVar `toml:"env"`
}

// The NodeConfig is used to configure how the Node presented to the local API
// server.
type NodeConfig struct {
	// Resources the Node should indicate it has.
	Resources NodeResourcesConfig `toml:"resources"`
}

// The NodeResourcesConfig is used to configure the resources the Node will
// present to the local API server.
type NodeResourcesConfig struct {
	// Allocatable resources the Node should indicate it has.
	Allocatable map[string]string `toml:"allocatable"`
}

// A ConfigFile is used to configure AK.
type ConfigFile struct {
	// Local client configuration - i.e. how AK should connect to the API
	// server to which it registers as a node.
	Local ClientConfig `toml:"local"`

	// Remote client configuration - i.e. the API server in which AK runs pods.
	Remote ClientConfig `toml:"remote"`

	// Pods configuration - influences how pods are prepared for submission to
	// the remote API server.
	Pods PodsConfig `toml:"pods"`

	// Node configuration - configures how the Node is presented to the local
	// API server.
	Node NodeConfig `toml:"node"`
}

// ParseConfigFile parses the TOML config file at the supplied path.
func ParseConfigFile(path string) (ConfigFile, error) {
	b, err := ioutil.ReadFile(filepath.Clean(path))
	if err != nil {
		return ConfigFile{}, errors.Wrap(err, "cannot read config file")
	}

	cfg := &ConfigFile{}
	if err := toml.Unmarshal(b, cfg); err != nil {
		return ConfigFile{}, errors.Wrap(err, "cannot unmarshal config file")
	}

	if err := ValidateConfigFile(*cfg); err != nil {
		return ConfigFile{}, errors.Wrap(err, "invalid config file")
	}

	return *cfg, nil
}

// ValidateConfigFile returns an error if the supplied config is invalid.
func ValidateConfigFile(cfg ConfigFile) error {
	if cfg.Remote.KubeConfigPath == "" && cfg.Local.KubeConfigPath == "" {
		return errors.New("at least one of local or remote kubeconfig path is required")
	}

	for k, v := range cfg.Node.Resources.Allocatable {
		if _, err := resource.ParseQuantity(v); err != nil {
			return errors.Wrapf(err, "cannot parse %q resource quantity", k)
		}
	}

	return nil
}
