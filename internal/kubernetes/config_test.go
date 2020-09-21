package kubernetes

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/crossplane/crossplane-runtime/pkg/test"
)

func TestValidateConfigFile(t *testing.T) {
	rt := "coolness"
	_, err := resource.ParseQuantity("wat")

	cases := map[string]struct {
		reason string
		cfg    ConfigFile
		want   error
	}{
		"MissingRemoteKubecfgPath": {
			reason: "A remote kubeconfig path is required",
			cfg:    ConfigFile{},
			want:   errors.New("remote kubeconfig path is required"),
		},
		"InvalidResourceValue": {
			reason: "Resource values must be parseable",
			cfg: ConfigFile{
				Remote: ClientConfig{KubeConfigPath: "/kcfg"},
				Node: NodeConfig{
					Resources: NodeResourcesConfig{
						Allocatable: map[string]string{rt: "wat"},
					},
				},
			},
			want: errors.Wrapf(err, "cannot parse %q resource quantity", rt),
		},
		"ValidConfigFile": {
			reason: "A valid config file should return no error",
			cfg: ConfigFile{
				Remote: ClientConfig{KubeConfigPath: "/kcfg"},
				Node: NodeConfig{
					Resources: NodeResourcesConfig{
						Allocatable: map[string]string{rt: "1000m"},
					},
				},
			},
			want: nil,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := ValidateConfigFile(tc.cfg)
			if diff := cmp.Diff(tc.want, got, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nValidateConfig(...): -want, +got: \n%s\n", tc.reason, diff)
			}
		})

	}
}
