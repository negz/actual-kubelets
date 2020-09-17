package kubernetes

import (
	"io/ioutil"
	"os"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
)

type ClientConfig struct {
	KubeConfigPath string        `toml:"kubeconfig_path"`
	ResyncInterval time.Duration `toml:"resync_interval"`
}

type PodConfig struct {
	Env []corev1.EnvVar `toml:"env"`
}

type Config struct {
	Local  ClientConfig `toml:"local"`
	Remote ClientConfig `toml:"remote"`
	Pods   PodConfig    `toml:"pods"`
}

func ParseConfig(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, errors.Wrap(err, "cannot open config file")
	}
	defer f.Close()

	b, err := ioutil.ReadAll(f)
	if err != nil {
		return Config{}, errors.Wrap(err, "cannot read config file")
	}

	cfg := &Config{}
	if err := toml.Unmarshal(b, cfg); err != nil {
		return Config{}, errors.Wrap(err, "cannot unmarshal config file")
	}

	return *cfg, err
}
