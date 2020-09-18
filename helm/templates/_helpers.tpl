{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "vk.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 24 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
*/}}
{{- define "vk.fullname" -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- printf "%s-%s" $name .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Standard labels for helm resources
*/}}
{{- define "vk.labels" -}}
labels:
  heritage: "{{ .Release.Service }}"
  release: "{{ .Release.Name }}"
  revision: "{{ .Release.Revision }}"
  chart: "{{ .Chart.Name }}"
  chartVersion: "{{ .Chart.Version }}"
  app: {{ template "vk.name" . }}
{{- end -}}

{{/*
Create a secret to pull private image.
*/}}
{{- define "imagePullSecret" }}
{{- printf "{\"auths\": {\"%s\": {\"auth\": \"%s\"}}}" .Values.image.repository (printf "%s:%s" .Values.image.repositoryReadOnlyPrincipalId .Values.image.repositoryReadOnlyPrincipalSecret | b64enc) | b64enc }}
{{- end }}


{{- define "config" }}
[local]
# The cluster the Virtual Kubelet will join as a node. Falls back to
# in-cluster config if not set.
resync_period = "10m"

[remote]
kubeconfig_path = "/etc/vk-config/remote.yaml"
resync_period = "10m"

[pods]
env = [
    # Inject this environment variable into all remote pods. In this case we're
    # tricking the in-cluster Kubernetes client configuration logic into
    # connecting to our 'local' API server, not the 'remote' on where they are
    # actually running.
    { name = "KUBERNETES_SERVICE_HOST", value = "{{ required "A local API-server host is required" .Values.local.apiserverHost }}"}
]

[node.resources.allocatable]
cpu = "100"
storage = "1024G"
memory = "100000G"
pods = "1000"
{{- end }}

{{- define "remotekubeconfig" }}
apiVersion: v1
clusters:
- cluster:
    {{- $caData := required "Remote API-server CA certificate data is required." .Values.remote.caData }}
    certificate-authority-data: {{ b64enc $caData }}
    server: https://{{ required "A remote API-server host is required. " .Values.remote.apiserverHost }}
  name: remote
contexts:
- context:
    cluster: remote
    user: remote
  name: remote
current-context: remote
kind: Config
preferences: {}
users:
- name: remote
  user:
    token: {{ required "A remote API-server token is required." .Values.remote.token }}
{{- end }}