package kubernetes

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestIsTokenVolume(t *testing.T) {
	dm := int32(0420)

	v := corev1.Volume{
		Name: "test-token-sdk2d",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName:  "test-token-sdk2d",
				DefaultMode: &dm,
			},
		},
	}
	want := true
	got := IsTokenVolume(v)
	if got != want {
		t.Errorf("IsTokenVolume: want %t, got %t", want, got)
	}
}
