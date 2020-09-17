package kubernetes

import (
	"io/ioutil"
	"os"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
	"github.com/virtual-kubelet/node-cli/provider"
	corev1 "k8s.io/api/core/v1"
)

type Config struct {
	provider.InitConfig
	ConfigFile
}

type ClientConfig struct {
	KubeConfigPath string        `toml:"kubeconfig_path"`
	ResyncInterval time.Duration `toml:"resync_interval"`
}

type PodConfig struct {
	Env []corev1.EnvVar `toml:"env"`
}

type ConfigFile struct {
	Local  ClientConfig `toml:"local"`
	Remote ClientConfig `toml:"remote"`
	Pods   PodConfig    `toml:"pods"`
}

func ParseConfigFile(path string) (ConfigFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return ConfigFile{}, errors.Wrap(err, "cannot open config file")
	}
	defer f.Close()

	b, err := ioutil.ReadAll(f)
	if err != nil {
		return ConfigFile{}, errors.Wrap(err, "cannot read config file")
	}

	cfg := &ConfigFile{}
	if err := toml.Unmarshal(b, cfg); err != nil {
		return ConfigFile{}, errors.Wrap(err, "cannot unmarshal config file")
	}

	return *cfg, err
}
