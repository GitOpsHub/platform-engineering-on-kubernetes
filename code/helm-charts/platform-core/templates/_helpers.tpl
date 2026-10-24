{{/*
Common helpers for platform-core. Names of cluster-scoped objects are fixed (no release prefix),
so install this chart once per cluster.
*/}}

{{- define "platform-core.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* Labels applied to every object. */}}
{{- define "platform-core.labels" -}}
helm.sh/chart: {{ include "platform-core.chart" . }}
app.kubernetes.io/part-of: platform
app.kubernetes.io/managed-by: {{ .Release.Service }}
platform.mycompany.io/part-of: platform
platform.mycompany.io/managed-by: helm
{{- end -}}

{{/* Selector labels for one component. Pass a dict with "name" and "root". */}}
{{- define "platform-core.selectorLabels" -}}
app.kubernetes.io/name: {{ .name }}
app.kubernetes.io/instance: {{ .root.Release.Name }}
{{- end -}}

{{/*
Image reference. Pass a dict with "image" (repository, tag) and "root".
An empty tag falls back to .Chart.AppVersion.
*/}}
{{- define "platform-core.image" -}}
{{- $tag := .image.tag | default .root.Chart.AppVersion -}}
{{- printf "%s:%s" .image.repository (toString $tag) -}}
{{- end -}}

{{/* Restricted pod security context, required by the namespace's PSA label. */}}
{{- define "platform-core.podSecurityContext" -}}
runAsNonRoot: true
seccompProfile:
  type: RuntimeDefault
{{- end -}}

{{- define "platform-core.containerSecurityContext" -}}
allowPrivilegeEscalation: false
readOnlyRootFilesystem: true
capabilities:
  drop: ["ALL"]
{{- end -}}
