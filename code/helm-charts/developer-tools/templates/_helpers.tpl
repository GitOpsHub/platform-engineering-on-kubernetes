{{- define "developer-tools.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/part-of: platform
app.kubernetes.io/managed-by: {{ .Release.Service }}
platform.mycompany.io/part-of: platform
platform.mycompany.io/managed-by: helm
platform.mycompany.io/component: developer-tools
{{- end -}}
