{{/* Labels for objects this chart renders itself (the test resources). */}}
{{- define "platform-addons.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/part-of: platform
app.kubernetes.io/managed-by: {{ .Release.Service }}
platform.mycompany.io/part-of: platform
platform.mycompany.io/managed-by: helm
{{- end -}}

{{/* Names of the CRDs a given add-on installs, used by the test. */}}
{{- define "platform-addons.enabledCRDs" -}}
{{- $crds := list -}}
{{- if index .Values "cert-manager" "enabled" -}}
{{- $crds = append $crds "certificates.cert-manager.io" -}}
{{- end -}}
{{- if index .Values "external-secrets" "enabled" -}}
{{- $crds = append $crds "externalsecrets.external-secrets.io" -}}
{{- end -}}
{{- if index .Values "kube-prometheus-stack" "enabled" -}}
{{- $crds = append $crds "prometheuses.monitoring.coreos.com" -}}
{{- end -}}
{{- toJson $crds -}}
{{- end -}}
