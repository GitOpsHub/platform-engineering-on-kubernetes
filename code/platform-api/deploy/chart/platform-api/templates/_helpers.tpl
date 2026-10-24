{{- define "platform-api.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "platform-api.fullname" -}}
{{- if contains .Chart.Name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "platform-api.selectorLabels" -}}
app.kubernetes.io/name: {{ include "platform-api.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "platform-api.labels" -}}
{{ include "platform-api.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: platform
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end -}}

{{- define "platform-api.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "platform-api.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/*
Groups platform-api may impersonate, exactly as listed in impersonation.groups.
Rendered as a JSON array so the RBAC resourceNames and the --impersonate-groups
flag come from one list. This matches platform-core's platformApi.impersonation.groups.
*/}}
{{- define "platform-api.impersonateGroupList" -}}
{{- .Values.impersonation.groups | toJson -}}
{{- end -}}
